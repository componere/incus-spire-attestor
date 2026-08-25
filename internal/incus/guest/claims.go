package guest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// guestInfo is the raw /1.0 JSON object.
type guestInfo struct {
	// APIVersion is the guest API version.
	APIVersion string `json:"api_version"`
	// Location is the cluster member.
	Location string `json:"location"`
	// InstanceType is the guest instance type.
	InstanceType string `json:"instance_type"`
	// State is the guest instance state.
	State string `json:"state"`
}

// guestMetadata is the first generated cloud-init identity pair.
type guestMetadata struct {
	// instanceID is the first instance-id value.
	instanceID string
	// hostname is the first local-hostname value.
	hostname string
}

// Claims returns the validated guest instance locators.
func (c *Client) Claims(ctx context.Context) (attest.Claims, error) {
	info, err := c.readInfo(ctx)
	if err != nil {
		return attest.Claims{}, err
	}
	meta, err := c.readMetadata(ctx)
	if err != nil {
		return attest.Claims{}, err
	}
	id, err := c.readProductUUID()
	if err != nil {
		return attest.Claims{}, err
	}

	claims := attest.Claims{
		Project:     c.project,
		Name:        attest.InstanceName(meta.hostname),
		UUID:        id,
		Type:        attest.InstanceType(info.InstanceType),
		Location:    info.Location,
		CloudInitID: meta.instanceID,
	}
	if err := attest.ValidateClaims(claims); err != nil {
		return attest.Claims{}, err
	}
	return claims, nil
}

// readInfo loads type and location from /1.0.
func (c *Client) readInfo(ctx context.Context) (guestInfo, error) {
	body, status, err := c.get(ctx, "/1.0")
	if err != nil {
		if isContextError(err) {
			return guestInfo{}, wrapContext("read guest info", err)
		}
		return guestInfo{}, err
	}
	if status != http.StatusOK {
		return guestInfo{}, statusError(status)
	}
	info, err := decodeInfo(body)
	if err != nil {
		return guestInfo{}, err
	}
	return info, nil
}

// decodeInfo decodes one bounded /1.0 JSON object and ignores unknown fields.
func decodeInfo(raw []byte) (guestInfo, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var info guestInfo
	if err := dec.Decode(&info); err != nil {
		return guestInfo{}, fmt.Errorf("decode guest info: %w", err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return guestInfo{}, errors.New("decode guest info: trailing JSON data")
	}
	return info, nil
}

// readMetadata loads cloud-init identity from /1.0/meta-data.
func (c *Client) readMetadata(ctx context.Context) (guestMetadata, error) {
	body, status, err := c.get(ctx, "/1.0/meta-data")
	if err != nil {
		if isContextError(err) {
			return guestMetadata{}, wrapContext("read guest metadata", err)
		}
		return guestMetadata{}, err
	}
	if status != http.StatusOK {
		return guestMetadata{}, statusError(status)
	}
	return parseMetadata(body), nil
}

// parseMetadata takes the first instance-id and local-hostname lines.
func parseMetadata(raw []byte) guestMetadata {
	var (
		meta    guestMetadata
		sawID   bool
		sawHost bool
	)
	for line := range strings.SplitSeq(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "instance-id":
			if !sawID {
				meta.instanceID = strings.TrimSpace(value)
				sawID = true
			}
		case "local-hostname":
			if !sawHost {
				meta.hostname = strings.TrimSpace(value)
				sawHost = true
			}
		}
	}
	return meta
}

// readProductUUID canonicalizes the DMI product UUID.
func (c *Client) readProductUUID() (attest.InstanceUUID, error) {
	raw, err := os.ReadFile(c.dmiPath)
	if err != nil {
		return "", fmt.Errorf("read product uuid: %w", err)
	}
	id, err := attest.NewInstanceUUID(string(bytes.TrimSpace(raw)))
	if err != nil {
		return "", fmt.Errorf("parse product uuid: %w", err)
	}
	return id, nil
}
