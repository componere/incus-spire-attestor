package host

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"

	"github.com/componere/incus-spire-attestor/internal/attest"
	"github.com/componere/incus-spire-attestor/internal/server"
)

// Client is the host-side Incus adapter.
type Client struct {
	// server is the connected Incus API client.
	server incus.InstanceServer
	// wait waits between retries; tests may replace it.
	wait waitFunc
	// standaloneLocation is the cached non-clustered server name from /1.0.
	// It is empty on a clustered server.
	standaloneLocation string
}

// contextual reaches the concrete client's request-context clone.
//
// Pinned v7.3 WithContext is absent from incus.InstanceServer and mutates
// its receiver, so callers must UseProject first and then clone that result.
type contextual interface {
	// WithContext returns a client that attaches ctx to subsequent API calls.
	WithContext(ctx context.Context) incus.InstanceServer
}

// New connects to endpoint over HTTPS using ConnectIncusWithContext.
//
// ca is the PKI CA certificate mapped to ConnectionArgs.TLSCA. cert and key
// are the client certificate and key mapped to TLSClientCert and TLSClientKey.
// Credentials are already-loaded PEM bytes; New does not read files.
// New reads Incus /1.0 once and caches the non-clustered server name used
// when instance location is the standalone sentinel "none".
func New(ctx context.Context, endpoint string, ca, cert, key []byte) (*Client, error) {
	server, err := incus.ConnectIncusWithContext(ctx, endpoint, &incus.ConnectionArgs{
		TLSCA:         string(ca),
		TLSClientCert: string(cert),
		TLSClientKey:  string(key),
	})
	if err != nil {
		return nil, fmt.Errorf("connect incus: %w", err)
	}
	client, err := newClient(server)
	if err != nil {
		(&Client{server: server}).CloseIdleConnections()
		return nil, err
	}
	return client, nil
}

// newClient constructs a Client around an already-connected server.
func newClient(server incus.InstanceServer) (*Client, error) {
	client := &Client{server: server, wait: waitDuration}
	if err := client.loadStandaloneLocation(); err != nil {
		return nil, err
	}
	return client, nil
}

// loadStandaloneLocation caches Environment.ServerName from Incus /1.0 when
// the server is not clustered.
func (c *Client) loadStandaloneLocation() error {
	info, _, err := c.server.GetServer()
	if err != nil {
		return fmt.Errorf("read incus server: %w", err)
	}
	if info == nil {
		return errors.New("incus server metadata is unavailable")
	}
	if info.Environment.ServerClustered {
		return nil
	}
	if info.Environment.ServerName == "" {
		return errors.New("incus server name is required")
	}
	c.standaloneLocation = info.Environment.ServerName
	return nil
}

// CloseIdleConnections closes idle HTTP connections on the underlying client.
//
// It never calls Disconnect, so in-flight requests on a superseded runtime
// keep their connections.
func (c *Client) CloseIdleConnections() {
	httpClient, err := c.server.GetHTTPClient()
	if err != nil || httpClient == nil {
		return
	}
	httpClient.CloseIdleConnections()
}

// Lookup returns the named instance in project when it exists.
func (c *Client) Lookup(
	ctx context.Context,
	project attest.ProjectName,
	name attest.InstanceName,
) (attest.Instance, bool, error) {
	server, err := c.scoped(ctx, project)
	if err != nil {
		return attest.Instance{}, false, fmt.Errorf("lookup instance: %w", err)
	}
	inst, _, err := server.GetInstance(string(name))
	if err != nil {
		if api.StatusErrorCheck(err, http.StatusNotFound) {
			return attest.Instance{}, false, nil
		}
		return attest.Instance{}, false, fmt.Errorf("lookup instance: %w", err)
	}
	mapped, err := mapInstance(project, name, inst, c.standaloneLocation)
	if err != nil {
		return attest.Instance{}, false, fmt.Errorf("lookup instance: %w", err)
	}
	return mapped, true, nil
}

// scoped returns a project-qualified, request-scoped Incus client.
func (c *Client) scoped(ctx context.Context, project attest.ProjectName) (incus.InstanceServer, error) {
	return withContext(ctx, c.server.UseProject(string(project)))
}

// withContext clones server after UseProject so WithContext cannot mutate the shared client.
func withContext(ctx context.Context, server incus.InstanceServer) (incus.InstanceServer, error) {
	clone, ok := server.(contextual)
	if !ok {
		return nil, errMissingContext
	}
	return clone.WithContext(ctx), nil
}

// errMissingContext is returned when an Incus client cannot attach request context.
var errMissingContext = errors.New("incus client does not support request context")

var _ server.Incus = (*Client)(nil)
