package attest

import (
	"fmt"
	"slices"
	"strings"
)

// maxSelectors is the maximum number of final selector strings.
const maxSelectors = 100

// maxSelectorValueBytes is the maximum UTF-8 byte budget for selector values.
const maxSelectorValueBytes = 32768

// BuildAttributes derives the agent ID and selectors from instance only.
func BuildAttributes(trustDomain string, instance Instance, userSelectors []string) (Attributes, error) {
	if err := requireVirtualMachine(instance.Type, "api"); err != nil {
		return Attributes{}, err
	}
	id, err := NewInstanceUUID(string(instance.UUID))
	if err != nil {
		return Attributes{}, fmt.Errorf("%w: %v", ErrDenied, err)
	}

	selectors, err := buildSelectors(instance, id, userSelectors)
	if err != nil {
		return Attributes{}, err
	}

	return Attributes{
		AgentID:     "spiffe://" + trustDomain + "/spire/agent/incus/" + string(id),
		CanReattest: true,
		Selectors:   selectors,
	}, nil
}

// buildSelectors assembles, sorts, and bounds the API-derived selector values.
func buildSelectors(instance Instance, id InstanceUUID, userSelectors []string) ([]string, error) {
	selectors := make([]string, 0, 4+len(instance.Profiles)+len(userSelectors))
	selectors = append(selectors,
		"project:"+string(instance.Project),
		"name:"+string(instance.Name),
		"location:"+instance.Location,
		"uuid:"+string(id),
	)
	for _, profile := range instance.Profiles {
		selectors = append(selectors, "profile:"+profile)
	}
	for _, key := range userSelectors {
		if strings.HasPrefix(key, configKeyPrefix) {
			return nil, fmt.Errorf("%w: reserved selector key", ErrDenied)
		}
		value, ok := instance.ExpandedConfig[key]
		if !ok {
			continue
		}
		selectors = append(selectors, key+":"+value)
	}

	slices.Sort(selectors)
	selectors = slices.Clip(slices.Compact(selectors))

	if len(selectors) > maxSelectors {
		return nil, fmt.Errorf("%w: selector count %d exceeds %d", ErrDenied, len(selectors), maxSelectors)
	}

	valueBytes := 0
	for _, selector := range selectors {
		valueBytes += selectorValueLen(selector)
		if valueBytes > maxSelectorValueBytes {
			return nil, fmt.Errorf("%w: selector value bytes exceed %d", ErrDenied, maxSelectorValueBytes)
		}
	}
	return selectors, nil
}

// selectorValueLen returns the UTF-8 byte length after the first colon.
func selectorValueLen(selector string) int {
	index := strings.IndexByte(selector, ':')
	if index < 0 {
		return 0
	}
	return len(selector) - index - 1
}
