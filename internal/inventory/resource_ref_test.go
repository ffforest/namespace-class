package inventory

import "testing"

func TestResourceRefKeyIncludesScope(t *testing.T) {
	namespaced := ResourceRef{
		APIVersion: "v1",
		Kind:       "ServiceAccount",
		Namespace:  "web-portal",
		Name:       "app",
	}
	clusterScoped := ResourceRef{
		APIVersion: "v1",
		Kind:       "ServiceAccount",
		Name:       "app",
	}

	namespacedKey, err := namespaced.Key()
	if err != nil {
		t.Fatalf("namespaced key: %v", err)
	}
	clusterKey, err := clusterScoped.Key()
	if err != nil {
		t.Fatalf("cluster key: %v", err)
	}
	if namespacedKey == clusterKey {
		t.Fatalf("expected namespace to participate in resource identity")
	}
}

func TestResourceRefKeyRejectsMissingIdentity(t *testing.T) {
	ref := ResourceRef{APIVersion: "v1", Kind: "ConfigMap"}

	if _, err := ref.Key(); err == nil {
		t.Fatal("expected missing name to fail")
	}
}
