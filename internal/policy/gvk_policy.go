package policy

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const defaultDeniedClusterRoleBinding = "rbac.authorization.k8s.io/v1/ClusterRoleBinding"

type Policy struct {
	allow map[string]struct{}
	deny  map[string]struct{}
}

type DeniedError struct {
	GVK    schema.GroupVersionKind
	Reason string
}

func (e *DeniedError) Error() string {
	return fmt.Sprintf("GVK %s is denied: %s", Format(e.GVK), e.Reason)
}

func DefaultDenyGVKsCSV() string {
	return defaultDeniedClusterRoleBinding
}

func FromCSV(allowCSV, denyCSV string) (Policy, error) {
	allow, err := parseCSV(allowCSV)
	if err != nil {
		return Policy{}, fmt.Errorf("parse allow GVKs: %w", err)
	}
	deny, err := parseCSV(denyCSV)
	if err != nil {
		return Policy{}, fmt.Errorf("parse deny GVKs: %w", err)
	}
	return Policy{allow: allow, deny: deny}, nil
}

func (p Policy) Validate(gvk schema.GroupVersionKind) error {
	key := Format(gvk)
	if _, found := p.deny[key]; found {
		return &DeniedError{GVK: gvk, Reason: "matched denylist"}
	}
	if len(p.allow) > 0 {
		if _, found := p.allow[key]; !found {
			return &DeniedError{GVK: gvk, Reason: "not present in allowlist"}
		}
	}
	return nil
}

func Format(gvk schema.GroupVersionKind) string {
	if gvk.Group == "" {
		return fmt.Sprintf("%s/%s", gvk.Version, gvk.Kind)
	}
	return fmt.Sprintf("%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind)
}

func parseCSV(value string) (map[string]struct{}, error) {
	gvks := map[string]struct{}{}
	if strings.TrimSpace(value) == "" {
		return gvks, nil
	}

	for _, raw := range strings.Split(value, ",") {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		gvk, err := ParseGVK(item)
		if err != nil {
			return nil, err
		}
		gvks[Format(gvk)] = struct{}{}
	}
	return gvks, nil
}

func ParseGVK(value string) (schema.GroupVersionKind, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) == 2 {
		if parts[0] == "" || parts[1] == "" {
			return schema.GroupVersionKind{}, fmt.Errorf("invalid GVK %q, expected apiVersion/kind", value)
		}
		return schema.GroupVersionKind{Version: parts[0], Kind: parts[1]}, nil
	}
	if len(parts) == 3 {
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return schema.GroupVersionKind{}, fmt.Errorf("invalid GVK %q, expected group/version/kind", value)
		}
		return schema.GroupVersionKind{Group: parts[0], Version: parts[1], Kind: parts[2]}, nil
	}
	return schema.GroupVersionKind{}, fmt.Errorf("invalid GVK %q, expected apiVersion/kind or group/version/kind", value)
}
