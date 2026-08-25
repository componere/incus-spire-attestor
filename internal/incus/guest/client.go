package guest

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/componere/incus-spire-attestor/internal/agent"
	"github.com/componere/incus-spire-attestor/internal/attest"
)

// defaultSocketPath is the production guest API socket.
const defaultSocketPath = "/dev/incus/sock"

// defaultDMIPath is the production SMBIOS product UUID file.
const defaultDMIPath = "/sys/class/dmi/id/product_uuid"

// maxGuestBody is the maximum accepted guest response size in bytes.
const maxGuestBody = 65536

// guestHTTPBase is the dummy URL origin used with the Unix transport.
const guestHTTPBase = "http://incus"

// Client reads guest claims and challenged config values.
type Client struct {
	// project is the optional configured project hint.
	project attest.ProjectName
	// socketPath is the guest Unix socket used by DialContext.
	socketPath string
	// dmiPath is the product UUID file path.
	dmiPath string
	// http is the Unix-socket HTTP client.
	http *http.Client
}

var _ agent.GuestEvidence = (*Client)(nil)

// New constructs a Client for the production guest socket and DMI path.
func New(project attest.ProjectName) *Client {
	return newClient(project, defaultSocketPath, defaultDMIPath, nil)
}

// newClient constructs a Client with constructor-only socket, DMI, and transport overrides.
func newClient(project attest.ProjectName, socketPath, dmiPath string, httpClient *http.Client) *Client {
	c := &Client{
		project:    project,
		socketPath: socketPath,
		dmiPath:    dmiPath,
		http:       httpClient,
	}
	if c.http == nil {
		c.http = c.unixHTTPClient()
	}
	return c
}

// unixHTTPClient returns an HTTP client that dials c.socketPath.
func (c *Client) unixHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", c.socketPath)
			},
		},
	}
}

// get issues a context-aware GET and returns the bounded response body.
func (c *Client) get(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, guestHTTPBase+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build guest request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, transportError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGuestBody+1))
	if err != nil {
		return nil, 0, transportError(err)
	}
	if len(body) > maxGuestBody {
		return nil, resp.StatusCode, fmt.Errorf("guest response exceeds %d bytes", maxGuestBody)
	}
	return body, resp.StatusCode, nil
}
