package guest

import (
	"context"
	"net/http"
	"net/url"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// configPrefix is the guest config-value collection path.
const configPrefix = "/1.0/config/"

// ReadConfig reads one challenged guest config value.
//
// A 404 is reported as found=false with a nil error. Returned errors omit
// the config key and value.
func (c *Client) ReadConfig(ctx context.Context, key attest.ConfigKey) (string, bool, error) {
	body, status, err := c.get(ctx, configPath(key))
	if err != nil {
		if isContextError(err) {
			return "", false, wrapContext("read guest config", err)
		}
		return "", false, err
	}
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if status != http.StatusOK {
		return "", false, statusError(status)
	}
	return string(body), true, nil
}

// configPath appends the escaped key as exactly one /1.0/config/ segment.
func configPath(key attest.ConfigKey) string {
	return configPrefix + url.PathEscape(string(key))
}
