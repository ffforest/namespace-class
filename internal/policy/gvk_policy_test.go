package policy

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDefaultPolicyDeniesClusterRoleBinding(t *testing.T) {
	policy, err := FromCSV("", DefaultDenyGVKsCSV())
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}

	err = policy.Validate(schema.GroupVersionKind{
		Group:   "rbac.authorization.k8s.io",
		Version: "v1",
		Kind:    "ClusterRoleBinding",
	})
	if err == nil {
		t.Fatal("expected ClusterRoleBinding to be denied")
	}
	if !strings.Contains(err.Error(), "rbac.authorization.k8s.io/v1/ClusterRoleBinding") {
		t.Fatalf("expected error to include denied GVK, got %q", err.Error())
	}
}

func TestAllowListRestrictsResourcesWhenConfigured(t *testing.T) {
	policy, err := FromCSV("v1/ServiceAccount", "")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}

	if err := policy.Validate(schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"}); err != nil {
		t.Fatalf("expected ServiceAccount to be allowed: %v", err)
	}
	if err := policy.Validate(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}); err == nil {
		t.Fatal("expected ConfigMap to be denied by allowlist")
	}
}

func TestDenyListTakesPrecedenceOverAllowList(t *testing.T) {
	policy, err := FromCSV("rbac.authorization.k8s.io/v1/ClusterRoleBinding", "rbac.authorization.k8s.io/v1/ClusterRoleBinding")
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}

	err = policy.Validate(schema.GroupVersionKind{
		Group:   "rbac.authorization.k8s.io",
		Version: "v1",
		Kind:    "ClusterRoleBinding",
	})
	if err == nil {
		t.Fatal("expected denylist to override allowlist")
	}
}

func TestRejectsMalformedPolicyEntries(t *testing.T) {
	if _, err := FromCSV("ServiceAccount", ""); err == nil {
		t.Fatal("expected malformed allowlist entry to fail")
	}
}
