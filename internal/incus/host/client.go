package host

import (
	"context"
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
func New(ctx context.Context, endpoint string, ca, cert, key []byte) (*Client, error) {
	server, err := incus.ConnectIncusWithContext(ctx, endpoint, &incus.ConnectionArgs{
		TLSCA:         string(ca),
		TLSClientCert: string(cert),
		TLSClientKey:  string(key),
	})
	if err != nil {
		return nil, fmt.Errorf("connect incus: %w", err)
	}
	return newClient(server), nil
}

// newClient constructs a Client around an already-connected server.
func newClient(server incus.InstanceServer) *Client {
	return &Client{server: server, wait: waitDuration}
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
	mapped, err := mapInstance(project, name, inst)
	if err != nil {
		return attest.Instance{}, false, fmt.Errorf("lookup instance: %w", err)
	}
	return mapped, true, nil
}

// scoped returns a project-qualified, request-scoped Incus client.
func (c *Client) scoped(ctx context.Context, project attest.ProjectName) (incus.InstanceServer, error) {
	return withContext(c.server.UseProject(string(project)), ctx)
}

// withContext clones server after UseProject so WithContext cannot mutate the shared client.
func withContext(server incus.InstanceServer, ctx context.Context) (incus.InstanceServer, error) {
	clone, ok := server.(contextual)
	if !ok {
		return nil, errMissingContext
	}
	return clone.WithContext(ctx), nil
}

// errMissingContext is returned when an Incus client cannot attach request context.
var errMissingContext = fmt.Errorf("incus client does not support request context")

var _ server.Incus = (*Client)(nil)
