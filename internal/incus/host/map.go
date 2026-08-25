package host

import (
	"fmt"

	"github.com/lxc/incus/v7/shared/api"

	"github.com/componere/incus-spire-attestor/internal/attest"
)

// volatileUUIDKey is the Incus instance UUID configuration key.
const volatileUUIDKey = "volatile.uuid"

// volatileCloudInitIDKey is the Incus cloud-init instance-id configuration key.
const volatileCloudInitIDKey = "volatile.cloud-init.instance-id"

// mapInstance copies a detached domain snapshot from an Incus API instance.
func mapInstance(project attest.ProjectName, name attest.InstanceName, inst *api.Instance) (attest.Instance, error) {
	if inst == nil {
		return attest.Instance{}, fmt.Errorf("%w: instance is required", attest.ErrDenied)
	}
	mappedProject := project
	if inst.Project != "" {
		mappedProject = attest.ProjectName(inst.Project)
	}
	mappedName := name
	if inst.Name != "" {
		mappedName = attest.InstanceName(inst.Name)
	}
	uuid, err := attest.NewInstanceUUID(configValue(inst.ExpandedConfig, volatileUUIDKey))
	if err != nil {
		return attest.Instance{}, fmt.Errorf("%w: invalid instance uuid", attest.ErrDenied)
	}
	return attest.Instance{
		Project:        mappedProject,
		Name:           mappedName,
		UUID:           uuid,
		Type:           attest.InstanceType(inst.Type),
		Location:       inst.Location,
		CloudInitID:    configValue(inst.ExpandedConfig, volatileCloudInitIDKey),
		Profiles:       copyStrings(inst.Profiles),
		ExpandedConfig: copyConfig(inst.ExpandedConfig),
	}, nil
}

// configValue returns key from cfg, or empty when cfg is nil.
func configValue(cfg api.ConfigMap, key string) string {
	if cfg == nil {
		return ""
	}
	return cfg[key]
}

// copyStrings returns a detached copy of values.
func copyStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

// copyConfig returns a detached writable copy of cfg.
func copyConfig(cfg api.ConfigMap) map[string]string {
	if cfg == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(cfg))
	for key, value := range cfg {
		out[key] = value
	}
	return out
}
