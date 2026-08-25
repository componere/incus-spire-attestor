package guest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

const (
	canonicalUUID     = "550e8400-e29b-41d4-a716-446655440000"
	testInstanceName  = "vm-01"
	testProject       = "default"
	testLocation      = "member-01"
	testCloudInitID   = "i-0123456789abcdef"
	testConfigKey     = "user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"
	testConfigValue   = "3q2-7wARIjNEVWZ3iJmquw"
	escapedConfigPath = "/1.0/config/user.spire.attestor.nonce.0123456789abcdef0123456789abcdef"
)

// timeoutError is the consumer predicate used by agent.poll.
type timeoutError interface {
	Timeout() bool
}

// temporaryError is the consumer predicate used by agent.poll.
type temporaryError interface {
	Temporary() bool
}

// isRetryable reports whether err should be polled, matching agent.poll.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var timeout timeoutError
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var temporary temporaryError
	if errors.As(err, &temporary) && temporary.Temporary() {
		return true
	}
	return false
}

type guestServer struct {
	info     []byte
	meta     []byte
	config   []byte
	statuses map[string]int
	delay    time.Duration
	mu       sync.Mutex
	paths    []string
}

type testContext struct {
	client *Client
	server *guestServer
	socket string
	dmi    string
}

func golden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "testdata %s must be readable", name)
	return raw
}

func validClaims() attest.Claims {
	return attest.Claims{
		Project:     attest.ProjectName(testProject),
		Name:        attest.InstanceName(testInstanceName),
		UUID:        attest.InstanceUUID(canonicalUUID),
		Type:        attest.InstanceTypeVirtualMachine,
		Location:    testLocation,
		CloudInitID: testCloudInitID,
	}
}

func validKey() attest.ConfigKey {
	key, err := attest.NewConfigKey(testConfigKey)
	if err != nil {
		panic(err)
	}
	return key
}

func writeDMI(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "product_uuid")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func newGuestServer() *guestServer {
	return &guestServer{
		info:     append([]byte(nil), goldenBytes("1.0.json")...),
		meta:     append([]byte(nil), goldenBytes("meta-data")...),
		config:   append([]byte(nil), goldenBytes("config")...),
		statuses: map[string]int{},
	}
}

func goldenBytes(name string) []byte {
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		panic(err)
	}
	return raw
}

func (s *guestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.paths = append(s.paths, r.URL.RequestURI())
	status := s.statuses[r.URL.Path]
	delay := s.delay
	s.mu.Unlock()

	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
	}

	if status != 0 && status != http.StatusOK {
		w.WriteHeader(status)
		return
	}

	switch r.URL.Path {
	case "/1.0":
		_, _ = w.Write(s.info)
	case "/1.0/meta-data":
		_, _ = w.Write(s.meta)
	default:
		if r.URL.Path == escapedConfigPath {
			_, _ = w.Write(s.config)
			return
		}
		http.NotFound(w, r)
	}
}

func (s *guestServer) recordedPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.paths))
	copy(out, s.paths)
	return out
}

func startUnixServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "incus.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
	})
	return socket
}

func newTestContext(t *testing.T, project attest.ProjectName) *testContext {
	t.Helper()
	server := newGuestServer()
	dir := t.TempDir()
	dmi := writeDMI(t, dir, string(golden(t, "product_uuid")))
	socket := startUnixServer(t, server)
	return &testContext{
		client: newClient(project, socket, dmi, nil),
		server: server,
		socket: socket,
		dmi:    dmi,
	}
}

func TestClaimsMapsCommittedFixtures(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	got, err := tc.client.Claims(context.Background())
	require.NoError(t, err)
	assert.Equal(t, validClaims(), got)
}

func TestClaimsOmitsUnconfiguredProject(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, "")
	got, err := tc.client.Claims(context.Background())
	require.NoError(t, err)
	want := validClaims()
	want.Project = ""
	assert.Equal(t, want, got)
}

func TestClaimsIncludesConfiguredProject(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	got, err := tc.client.Claims(context.Background())
	require.NoError(t, err)
	assert.Equal(t, attest.ProjectName(testProject), got.Project)
}

func TestClaimsUsesFirstGeneratedMetadata(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.meta = []byte("#cloud-config\ninstance-id: i-0123456789abcdef\nlocal-hostname: vm-01\n#cloud-config\ninstance-id: operator-id\nlocal-hostname: operator-name\n")
	got, err := tc.client.Claims(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testCloudInitID, got.CloudInitID)
	assert.Equal(t, attest.InstanceName(testInstanceName), got.Name)
}

func TestClaimsRejectsContainer(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.info = []byte(`{"api_version":"1.0","location":"member-01","instance_type":"container","state":"Started"}`)
	_, err := tc.client.Claims(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, attest.ErrDenied)
	assert.Contains(t, err.Error(), "container")
}

func TestClaimsIgnoresUnknownInfoFields(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.info = []byte(`{"api_version":"1.0","location":"member-01","instance_type":"virtual-machine","state":"Started","extra":"ok"}`)
	got, err := tc.client.Claims(context.Background())
	require.NoError(t, err)
	assert.Equal(t, validClaims(), got)
}

func TestClaimsRejectsTrailingInfoJSON(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.info = []byte(`{"api_version":"1.0","location":"member-01","instance_type":"virtual-machine","state":"Started"}{"x":1}`)
	_, err := tc.client.Claims(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing JSON")
}

func TestClaimsRejectsMissingMetadata(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.meta = []byte("#cloud-config\n")
	_, err := tc.client.Claims(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, attest.ErrDenied)
}

func TestClaimsRejectsMalformedDMI(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	require.NoError(t, os.WriteFile(tc.dmi, []byte("not-a-uuid"), 0o600))
	_, err := tc.client.Claims(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, attest.ErrDenied)
	assert.Contains(t, err.Error(), "product uuid")
}

func TestClaimsRejectsMissingDMI(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	require.NoError(t, os.Remove(tc.dmi))
	_, err := tc.client.Claims(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "product uuid")
}

func TestReadConfigReturnsFoundValue(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	value, found, err := tc.client.ReadConfig(context.Background(), validKey())
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, testConfigValue, value)
}

func TestReadConfigMapsNotFound(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.statuses[escapedConfigPath] = http.StatusNotFound
	value, found, err := tc.client.ReadConfig(context.Background(), validKey())
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestReadConfigRecordsExactEscapedPath(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	_, found, err := tc.client.ReadConfig(context.Background(), validKey())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{escapedConfigPath}, tc.server.recordedPaths())
	for _, path := range tc.server.recordedPaths() {
		assert.NotContains(t, path, "?")
		assert.NotContains(t, path, "%2F")
		assert.NotContains(t, path, "//")
	}
}

func TestReadConfigClassifiesTransientStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		timeout bool
	}{
		{name: "request timeout", status: http.StatusRequestTimeout, timeout: true},
		{name: "too many requests", status: http.StatusTooManyRequests},
		{name: "internal error", status: http.StatusInternalServerError},
		{name: "bad gateway", status: http.StatusBadGateway},
		{name: "service unavailable", status: http.StatusServiceUnavailable},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, timeout: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tc := newTestContext(t, testProject)
			tc.server.statuses[escapedConfigPath] = tt.status
			value, found, err := tc.client.ReadConfig(context.Background(), validKey())
			require.Error(t, err)
			assert.False(t, found)
			assert.Empty(t, value)
			assert.True(t, isRetryable(err), "transient status must match agent poll predicate")
			assert.NotContains(t, err.Error(), testConfigKey)
			assert.NotContains(t, err.Error(), testConfigValue)
			if tt.timeout {
				var timeout timeoutError
				require.True(t, errors.As(err, &timeout))
				assert.True(t, timeout.Timeout())
			}
		})
	}
}

func TestReadConfigClassifiesPermanentStatus(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.statuses[escapedConfigPath] = http.StatusForbidden
	value, found, err := tc.client.ReadConfig(context.Background(), validKey())
	require.Error(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
	assert.False(t, isRetryable(err), "permanent status must not be retried")
	assert.NotContains(t, err.Error(), testConfigKey)
	assert.NotContains(t, err.Error(), testConfigValue)
}

func TestReadConfigClassifiesTransportFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dmi := writeDMI(t, dir, string(golden(t, "product_uuid")))
	client := newClient(testProject, filepath.Join(dir, "missing.sock"), dmi, nil)
	value, found, err := client.ReadConfig(context.Background(), validKey())
	require.Error(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
	assert.True(t, isRetryable(err), "transport failure must expose Temporary through errors.As")
	assert.NotContains(t, err.Error(), testConfigKey)
}

func TestReadConfigPreservesCancellation(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.delay = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	value, found, err := tc.client.ReadConfig(ctx, validKey())
	require.Error(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, isRetryable(err), "cancellation must not be classified as retryable")
	assert.NotContains(t, err.Error(), testConfigKey)
}

func TestReadConfigPreservesDeadline(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.delay = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	value, found, err := tc.client.ReadConfig(ctx, validKey())
	require.Error(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, err.Error(), testConfigKey)
}

func TestClaimsPreservesCancellation(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	tc.server.delay = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tc.client.Claims(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewUsesProductionPaths(t *testing.T) {
	t.Parallel()

	client := New(testProject)
	assert.Equal(t, attest.ProjectName(testProject), client.project)
	assert.Equal(t, defaultDMIPath, client.dmiPath)
	assert.NotNil(t, client.http)
}

func TestReadConfigDoesNotAcceptRawChallenge(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, testProject)
	_, _, err := tc.client.ReadConfig(context.Background(), validKey())
	require.NoError(t, err)
	assert.Equal(t, escapedConfigPath, tc.server.recordedPaths()[0])
	assert.NotEqual(t, "/1.0/config/"+testConfigKey+"?raw=1", tc.server.recordedPaths()[0])
}
