package host

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

const (
	testProject     = "default"
	otherProject    = "other"
	testName        = "vm-01"
	testUUID        = "550e8400-e29b-41d4-a716-446655440000"
	replacedUUID    = "11111111-2222-4333-8444-555555555555"
	testLocation    = "member-01"
	testCloudInitID = "i-abc"
	testNonce       = "secret-nonce-value"
	operatorKey     = "user.environment"
	operatorValue   = "prod"
	otherAttemptKey = "user.spire.attestor.nonce.aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func testKey(t *testing.T) attest.ConfigKey {
	t.Helper()
	key, err := attest.NewConfigKey("user.spire.attestor.nonce.0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	return key
}

func testTarget() attest.Instance {
	return attest.Instance{
		Project: testProject,
		Name:    testName,
		UUID:    testUUID,
		Type:    attest.InstanceTypeVirtualMachine,
	}
}

func instantWait(ctx context.Context, _ time.Duration) error {
	return ctx.Err()
}

type instanceRecord struct {
	// inst is the current API instance.
	inst api.Instance
	// etag is the current ETag.
	etag string
}

type fakeStore struct {
	// mu guards all store fields.
	mu sync.Mutex
	// instances is the project-qualified instance map.
	instances map[instanceKey]*instanceRecord
	// getErrs are consumed by successive GetInstance calls.
	getErrs []error
	// updateErrs are consumed by successive UpdateInstance calls.
	updateErrs []error
	// afterUpdates run after the corresponding UpdateInstance is recorded.
	afterUpdates []func()
	// waitErrs are consumed by successive WaitContext calls.
	waitErrs []error
	// projects records UseProject arguments.
	projects []string
	// gets records project-qualified lookups.
	gets []instanceKey
	// updates records applied mutations.
	updates []recordedUpdate
	// waits is the number of operation waits.
	waits int
	// rootWithContext counts WithContext on the shared root.
	rootWithContext int
	// cloneWithContext counts WithContext on a UseProject clone.
	cloneWithContext int
	// disconnects counts Disconnect calls.
	disconnects int
	// httpClient is returned by GetHTTPClient.
	httpClient *http.Client
	// httpErr is returned by GetHTTPClient when set.
	httpErr error
}

type instanceKey struct {
	// project is the Incus project.
	project string
	// name is the instance name.
	name string
}

type recordedUpdate struct {
	// key is the mutated instance.
	key instanceKey
	// etag is the If-Match value.
	etag string
	// config is the submitted writable config.
	config map[string]string
}

type fakeServer struct {
	incus.InstanceServer

	// store is the shared fake backend.
	store *fakeStore
	// project is the UseProject clone's project.
	project string
	// ctx is the request context attached by WithContext.
	ctx context.Context
	// root reports whether this value is the shared stored client.
	root bool
}

type fakeOp struct {
	incus.Operation

	// store consumes queued wait errors.
	store *fakeStore
}

func newFake() *fakeServer {
	return &fakeServer{
		store:   &fakeStore{instances: map[instanceKey]*instanceRecord{}},
		project: testProject,
		root:    true,
	}
}

func (s *fakeServer) UseProject(name string) incus.InstanceServer {
	s.store.mu.Lock()
	s.store.projects = append(s.store.projects, name)
	s.store.mu.Unlock()
	return &fakeServer{store: s.store, project: name}
}

func (s *fakeServer) WithContext(ctx context.Context) incus.InstanceServer {
	s.store.mu.Lock()
	if s.root {
		s.store.rootWithContext++
	} else {
		s.store.cloneWithContext++
	}
	s.store.mu.Unlock()
	return &fakeServer{store: s.store, project: s.project, ctx: ctx}
}

func (s *fakeServer) GetInstance(name string) (*api.Instance, string, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.gets = append(s.store.gets, instanceKey{project: s.project, name: name})
	if s.ctx != nil {
		if err := s.ctx.Err(); err != nil {
			return nil, "", err
		}
	}
	if err := popErr(&s.store.getErrs); err != nil {
		return nil, "", err
	}
	rec, ok := s.store.instances[instanceKey{project: s.project, name: name}]
	if !ok {
		return nil, "", api.StatusErrorf(http.StatusNotFound, "not found")
	}
	cloned := cloneInstance(rec.inst)
	return &cloned, rec.etag, nil
}

func (s *fakeServer) UpdateInstance(name string, instance api.InstancePut, etag string) (incus.Operation, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if s.ctx != nil {
		if err := s.ctx.Err(); err != nil {
			return nil, err
		}
	}
	key := instanceKey{project: s.project, name: name}
	s.store.updates = append(s.store.updates, recordedUpdate{
		key:    key,
		etag:   etag,
		config: copyConfig(instance.Config),
	})
	if hook := popHook(&s.store.afterUpdates); hook != nil {
		hook()
	}
	if err := popErr(&s.store.updateErrs); err != nil {
		return nil, err
	}
	rec, ok := s.store.instances[key]
	if !ok {
		return nil, api.StatusErrorf(http.StatusNotFound, "not found")
	}
	if etag != rec.etag {
		return nil, api.StatusErrorf(http.StatusPreconditionFailed, "etag mismatch")
	}
	rec.inst.Config = copyConfig(instance.Config)
	rec.inst.Profiles = copyStrings(instance.Profiles)
	rec.etag += "-next"
	return &fakeOp{store: s.store}, nil
}

func (s *fakeServer) GetHTTPClient() (*http.Client, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if s.store.httpErr != nil {
		return nil, s.store.httpErr
	}
	return s.store.httpClient, nil
}

func (s *fakeServer) Disconnect() {
	s.store.mu.Lock()
	s.store.disconnects++
	s.store.mu.Unlock()
}

func (o *fakeOp) WaitContext(ctx context.Context) error {
	o.store.mu.Lock()
	defer o.store.mu.Unlock()
	o.store.waits++
	if err := ctx.Err(); err != nil {
		return err
	}
	return popErr(&o.store.waitErrs)
}

func popHook(queue *[]func()) func() {
	if len(*queue) == 0 {
		return nil
	}
	hook := (*queue)[0]
	*queue = (*queue)[1:]
	return hook
}

func popErr(queue *[]error) error {
	if len(*queue) == 0 {
		return nil
	}
	err := (*queue)[0]
	*queue = (*queue)[1:]
	return err
}

func cloneInstance(in api.Instance) api.Instance {
	out := in
	out.Config = copyConfig(in.Config)
	out.ExpandedConfig = copyConfig(in.ExpandedConfig)
	out.Profiles = copyStrings(in.Profiles)
	return out
}

func (s *fakeServer) putInstance(project, name string, inst api.Instance, etag string) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.putInstanceLocked(project, name, inst, etag)
}

func (s *fakeServer) putInstanceLocked(project, name string, inst api.Instance, etag string) {
	cloned := cloneInstance(inst)
	s.store.instances[instanceKey{project: project, name: name}] = &instanceRecord{inst: cloned, etag: etag}
}

func (s *fakeServer) instance() (api.Instance, bool) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	rec, ok := s.store.instances[instanceKey{project: s.project, name: testName}]
	if !ok {
		return api.Instance{}, false
	}
	return cloneInstance(rec.inst), true
}

func vmInstance() api.Instance {
	return api.Instance{
		Project:  testProject,
		Name:     testName,
		Type:     string(attest.InstanceTypeVirtualMachine),
		Location: testLocation,
		InstancePut: api.InstancePut{
			Profiles: []string{"default", "vm"},
			Config: api.ConfigMap{
				volatileUUIDKey:        testUUID,
				volatileCloudInitIDKey: testCloudInitID,
				operatorKey:            operatorValue,
			},
		},
		ExpandedConfig: api.ConfigMap{
			volatileUUIDKey:        testUUID,
			volatileCloudInitIDKey: testCloudInitID,
			operatorKey:            operatorValue,
		},
	}
}

func newTestClient(t *testing.T, server incus.InstanceServer) *Client {
	t.Helper()
	client := newClient(server)
	client.wait = instantWait
	return client
}

func TestLookupUsesProjectThenContext(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	fake.putInstance(otherProject, testName, vmInstance(), "etag-other")
	client := newTestClient(t, fake)

	got, found, err := client.Lookup(context.Background(), testProject, testName)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, attest.ProjectName(testProject), got.Project)
	assert.Equal(t, attest.InstanceName(testName), got.Name)
	assert.Equal(t, attest.InstanceUUID(testUUID), got.UUID)
	assert.Equal(t, attest.InstanceTypeVirtualMachine, got.Type)
	assert.Equal(t, testLocation, got.Location)
	assert.Equal(t, testCloudInitID, got.CloudInitID)
	assert.Equal(t, []string{"default", "vm"}, got.Profiles)
	assert.Equal(t, operatorValue, got.ExpandedConfig[operatorKey])

	fake.store.mu.Lock()
	defer fake.store.mu.Unlock()
	assert.Equal(t, []string{testProject}, fake.store.projects)
	assert.Equal(t, []instanceKey{{project: testProject, name: testName}}, fake.store.gets)
	assert.Equal(t, 0, fake.store.rootWithContext)
	assert.Equal(t, 1, fake.store.cloneWithContext)
}

func TestLookupMapsDetachedCopy(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	client := newTestClient(t, fake)

	got, found, err := client.Lookup(context.Background(), testProject, testName)
	require.NoError(t, err)
	require.True(t, found)

	got.ExpandedConfig[operatorKey] = "mutated"
	got.Profiles[0] = "mutated"
	stored, ok := fake.instance()
	require.True(t, ok)
	assert.Equal(t, operatorValue, stored.ExpandedConfig[operatorKey])
	assert.Equal(t, "default", stored.Profiles[0])
}

func TestMapInstanceDetachesConfigAndProfiles(t *testing.T) {
	t.Parallel()

	source := vmInstance()
	got, err := mapInstance(testProject, testName, &source)
	require.NoError(t, err)

	got.ExpandedConfig[operatorKey] = "mutated"
	got.Profiles[0] = "mutated"
	assert.Equal(t, operatorValue, source.ExpandedConfig[operatorKey])
	assert.Equal(t, "default", source.Profiles[0])
}

func TestLookupMaps404OnlyToAbsence(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		client := newTestClient(t, newFake())
		got, found, err := client.Lookup(context.Background(), testProject, testName)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Zero(t, got)
	})

	t.Run("forbidden is operational", func(t *testing.T) {
		t.Parallel()
		fake := newFake()
		fake.store.getErrs = []error{api.StatusErrorf(http.StatusForbidden, "denied")}
		client := newTestClient(t, fake)
		got, found, err := client.Lookup(context.Background(), testProject, testName)
		require.Error(t, err)
		assert.False(t, found)
		assert.Zero(t, got)
		assert.True(t, api.StatusErrorCheck(err, http.StatusForbidden))
		require.NotErrorIs(t, err, attest.ErrDenied)
	})

	t.Run("malformed uuid is not absence", func(t *testing.T) {
		t.Parallel()
		inst := vmInstance()
		inst.ExpandedConfig[volatileUUIDKey] = "not-a-uuid"
		fake := newFake()
		fake.putInstance(testProject, testName, inst, "etag-1")
		client := newTestClient(t, fake)
		got, found, err := client.Lookup(context.Background(), testProject, testName)
		require.Error(t, err)
		assert.False(t, found)
		assert.Zero(t, got)
		require.ErrorIs(t, err, attest.ErrDenied)
	})

	t.Run("missing api name is not absence", func(t *testing.T) {
		t.Parallel()
		inst := vmInstance()
		inst.Name = ""
		fake := newFake()
		fake.putInstance(testProject, testName, inst, "etag-1")
		client := newTestClient(t, fake)
		got, found, err := client.Lookup(context.Background(), testProject, testName)
		require.Error(t, err)
		assert.False(t, found)
		assert.Zero(t, got)
		require.ErrorIs(t, err, attest.ErrDenied)
	})

	t.Run("mismatched api name is not absence", func(t *testing.T) {
		t.Parallel()
		inst := vmInstance()
		inst.Name = "vm-other"
		fake := newFake()
		fake.putInstance(testProject, testName, inst, "etag-1")
		client := newTestClient(t, fake)
		got, found, err := client.Lookup(context.Background(), testProject, testName)
		require.Error(t, err)
		assert.False(t, found)
		assert.Zero(t, got)
		require.ErrorIs(t, err, attest.ErrDenied)
	})

	t.Run("mismatched api project is not absence", func(t *testing.T) {
		t.Parallel()
		inst := vmInstance()
		inst.Project = otherProject
		fake := newFake()
		fake.putInstance(testProject, testName, inst, "etag-1")
		client := newTestClient(t, fake)
		got, found, err := client.Lookup(context.Background(), testProject, testName)
		require.Error(t, err)
		assert.False(t, found)
		assert.Zero(t, got)
		require.ErrorIs(t, err, attest.ErrDenied)
	})
}

func TestLookupContextClonesAreConcurrentSafe(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	other := vmInstance()
	other.Project = otherProject
	other.Name = "vm-02"
	fake.putInstance(otherProject, "vm-02", other, "etag-2")
	client := newTestClient(t, fake)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := client.Lookup(context.Background(), testProject, testName)
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, _, err := client.Lookup(context.Background(), otherProject, "vm-02")
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	fake.store.mu.Lock()
	defer fake.store.mu.Unlock()
	assert.Equal(t, 0, fake.store.rootWithContext)
	assert.Equal(t, 2, fake.store.cloneWithContext)
	assert.ElementsMatch(t, []string{testProject, otherProject}, fake.store.projects)
}

func TestSetNonceWritesExactKeyAndPreservesOthers(t *testing.T) {
	t.Parallel()

	fake := newFake()
	inst := vmInstance()
	inst.Config[otherAttemptKey] = "other-attempt"
	fake.putInstance(testProject, testName, inst, "etag-1")
	client := newTestClient(t, fake)
	key := testKey(t)

	require.NoError(t, client.SetNonce(context.Background(), testTarget(), key, testNonce))

	stored, ok := fake.instance()
	require.True(t, ok)
	assert.Equal(t, testNonce, stored.Config[string(key)])
	assert.Equal(t, operatorValue, stored.Config[operatorKey])
	assert.Equal(t, "other-attempt", stored.Config[otherAttemptKey])
	assert.Equal(t, 1, fake.store.waits)
	require.Len(t, fake.store.updates, 1)
	assert.Equal(t, "etag-1", fake.store.updates[0].etag)
	assert.Equal(t, testProject, fake.store.updates[0].key.project)
}

func TestUnsetNonceRemovesExactKey(t *testing.T) {
	t.Parallel()

	fake := newFake()
	key := testKey(t)
	inst := vmInstance()
	inst.Config[string(key)] = testNonce
	fake.putInstance(testProject, testName, inst, "etag-1")
	client := newTestClient(t, fake)

	require.NoError(t, client.UnsetNonce(context.Background(), testTarget(), key))

	stored, ok := fake.instance()
	require.True(t, ok)
	_, found := stored.Config[string(key)]
	assert.False(t, found)
	assert.Equal(t, operatorValue, stored.Config[operatorKey])
	assert.Equal(t, 1, fake.store.waits)
}

func TestUnsetNonceSucceedsWithoutMutation(t *testing.T) {
	t.Parallel()

	key := testKey(t)
	tests := []struct {
		name  string
		setup func(*fakeServer)
	}{
		{
			name: "absent key",
			setup: func(fake *fakeServer) {
				fake.putInstance(testProject, testName, vmInstance(), "etag-1")
			},
		},
		{
			name:  "absent instance",
			setup: func(*fakeServer) {},
		},
		{
			name: "replaced uuid",
			setup: func(fake *fakeServer) {
				inst := vmInstance()
				inst.ExpandedConfig[volatileUUIDKey] = replacedUUID
				inst.Config[volatileUUIDKey] = replacedUUID
				fake.putInstance(testProject, testName, inst, "etag-1")
			},
		},
		{
			name: "replaced vm type",
			setup: func(fake *fakeServer) {
				inst := vmInstance()
				inst.Type = string(api.InstanceTypeContainer)
				fake.putInstance(testProject, testName, inst, "etag-1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFake()
			tt.setup(fake)
			client := newTestClient(t, fake)
			require.NoError(t, client.UnsetNonce(context.Background(), testTarget(), key))
			assert.Empty(t, fake.store.updates)
			assert.Zero(t, fake.store.waits)
		})
	}
}

func TestSetNonceRetriesTransientStatusErrors(t *testing.T) {
	t.Parallel()

	statuses := []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			fake := newFake()
			fake.putInstance(testProject, testName, vmInstance(), "etag-1")
			fake.store.getErrs = []error{api.StatusErrorf(status, "transient %d", status)}
			client := newTestClient(t, fake)
			key := testKey(t)
			require.NoError(t, client.SetNonce(context.Background(), testTarget(), key, testNonce))
			stored, ok := fake.instance()
			require.True(t, ok)
			assert.Equal(t, testNonce, stored.Config[string(key)])
		})
	}
}

func TestSetNonceRetriesTransportFailure(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	fake.store.getErrs = []error{&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}}
	client := newTestClient(t, fake)
	require.NoError(t, client.SetNonce(context.Background(), testTarget(), testKey(t), testNonce))
}

func TestSetNonceRetriesETagConflictAfterRefetch(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	fake.store.updateErrs = []error{api.StatusErrorf(http.StatusPreconditionFailed, "etag mismatch")}
	client := newTestClient(t, fake)
	key := testKey(t)

	require.NoError(t, client.SetNonce(context.Background(), testTarget(), key, testNonce))

	stored, ok := fake.instance()
	require.True(t, ok)
	assert.Equal(t, testNonce, stored.Config[string(key)])
	assert.Equal(t, operatorValue, stored.Config[operatorKey])
	require.Len(t, fake.store.updates, 2)
	assert.Equal(t, 1, fake.store.waits)
}

func TestSetNonceDoesNotRetryReplacementDuringConflict(t *testing.T) {
	t.Parallel()

	t.Run("uuid", func(t *testing.T) {
		t.Parallel()
		fake := newFake()
		fake.putInstance(testProject, testName, vmInstance(), "etag-1")
		replaced := false
		fake.store.afterUpdates = []func(){func() {
			inst := vmInstance()
			inst.ExpandedConfig[volatileUUIDKey] = replacedUUID
			inst.Config[volatileUUIDKey] = replacedUUID
			fake.putInstanceLocked(testProject, testName, inst, "etag-2")
			replaced = true
		}}
		fake.store.updateErrs = []error{api.StatusErrorf(http.StatusPreconditionFailed, "etag mismatch")}
		err := newTestClient(t, fake).SetNonce(context.Background(), testTarget(), testKey(t), testNonce)
		require.Error(t, err)
		require.ErrorIs(t, err, errReplacedTarget)
		assert.True(t, replaced)
		assert.Len(t, fake.store.updates, 1)
	})

	t.Run("vm type", func(t *testing.T) {
		t.Parallel()
		fake := newFake()
		fake.putInstance(testProject, testName, vmInstance(), "etag-1")
		fake.store.afterUpdates = []func(){func() {
			inst := vmInstance()
			inst.Type = string(api.InstanceTypeContainer)
			fake.putInstanceLocked(testProject, testName, inst, "etag-2")
		}}
		fake.store.updateErrs = []error{api.StatusErrorf(http.StatusPreconditionFailed, "etag mismatch")}
		err := newTestClient(t, fake).SetNonce(context.Background(), testTarget(), testKey(t), testNonce)
		require.Error(t, err)
		require.ErrorIs(t, err, errReplacedTarget)
		assert.Len(t, fake.store.updates, 1)
	})
}

func TestSetNonceUnknownOutcomeOnFailedWait(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	fake.store.waitErrs = []error{api.StatusErrorf(http.StatusForbidden, "cannot watch %s", testNonce)}
	err := newTestClient(t, fake).SetNonce(context.Background(), testTarget(), testKey(t), testNonce)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("%s: %d", setAction, http.StatusForbidden), err.Error())
	assert.NotContains(t, err.Error(), testNonce)
	assert.True(t, api.StatusErrorCheck(err, http.StatusForbidden))
	assert.Equal(t, 1, fake.store.waits)
}

func TestSetNonceOmitsNonceFromErrorText(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	injected := fmt.Errorf("echo %s: %w", testNonce, context.DeadlineExceeded)
	fake.store.updateErrs = []error{injected}
	err := newTestClient(t, fake).SetNonce(context.Background(), testTarget(), testKey(t), testNonce)
	require.Error(t, err)
	assert.Equal(t, setAction+": "+context.DeadlineExceeded.Error(), err.Error())
	assert.NotContains(t, err.Error(), testNonce)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, injected)
}

func TestSetNoncePreservesContextDeadline(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	err := newTestClient(t, fake).SetNonce(ctx, testTarget(), testKey(t), testNonce)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, setAction+": "+context.DeadlineExceeded.Error(), err.Error())
	assert.NotContains(t, err.Error(), testNonce)
	assert.Empty(t, fake.store.updates)
}

func TestSetNonceDoesNotRetryPermanentStatus(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.putInstance(testProject, testName, vmInstance(), "etag-1")
	fake.store.getErrs = []error{api.StatusErrorf(http.StatusForbidden, "denied")}
	err := newTestClient(t, fake).SetNonce(context.Background(), testTarget(), testKey(t), testNonce)
	require.Error(t, err)
	assert.True(t, api.StatusErrorCheck(err, http.StatusForbidden))
	assert.Len(t, fake.store.gets, 1)
}

func TestCloseIdleConnectionsDoesNotDisconnect(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.store.httpClient = &http.Client{Transport: &http.Transport{}}
	client := newTestClient(t, fake)
	client.CloseIdleConnections()
	assert.Zero(t, fake.store.disconnects)

	fake.store.httpErr = errors.New("missing client")
	client.CloseIdleConnections()
	assert.Zero(t, fake.store.disconnects)
}

func TestMatchResolvedTargetRejectsEmptyName(t *testing.T) {
	t.Parallel()

	current := vmInstance()
	current.Name = ""
	err := matchResolvedTarget(testTarget(), &current)
	require.ErrorIs(t, err, errReplacedTarget)

	current = vmInstance()
	current.Project = ""
	require.NoError(t, matchResolvedTarget(testTarget(), &current))
}

func TestWrapMutationSanitizesDiagnostics(t *testing.T) {
	t.Parallel()

	status := api.StatusErrorf(http.StatusConflict, "echo %s", testNonce)
	err := wrapMutation(setAction, status)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("%s: %d", setAction, http.StatusConflict), err.Error())
	assert.NotContains(t, err.Error(), testNonce)
	assert.True(t, api.StatusErrorCheck(err, http.StatusConflict))

	err = wrapMutation(setAction, fmt.Errorf("echo %s: %w", testNonce, errReplacedTarget))
	require.ErrorIs(t, err, errReplacedTarget)
	assert.Equal(t, setAction+": "+errReplacedTarget.Error(), err.Error())
	assert.NotContains(t, err.Error(), testNonce)

	err = wrapMutation(unsetAction, fmt.Errorf("echo %s: %w", testNonce, context.Canceled))
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, unsetAction+": "+context.Canceled.Error(), err.Error())
	assert.NotContains(t, err.Error(), testNonce)
}

func TestWaitDurationReturnsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitDuration(ctx, time.Hour)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNextRetryDelaySequenceAndCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{name: "first doubling", in: initialRetryDelay, want: 50 * time.Millisecond},
		{name: "second doubling", in: 50 * time.Millisecond, want: 100 * time.Millisecond},
		{name: "third doubling", in: 100 * time.Millisecond, want: 200 * time.Millisecond},
		{name: "caps at max", in: 200 * time.Millisecond, want: maxRetryDelay},
		{name: "stays capped", in: maxRetryDelay, want: maxRetryDelay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, nextRetryDelay(tt.in))
		})
	}
}
