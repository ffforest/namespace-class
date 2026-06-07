//go:build envtest

package envtest_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	namespaceClassGVK = schema.GroupVersionKind{
		Group:   "namespaceclass.akuity.io",
		Version: "v1alpha1",
		Kind:    "NamespaceClass",
	}
	namespaceClassBindingGVK = schema.GroupVersionKind{
		Group:   "namespaceclass.akuity.io",
		Version: "v1alpha1",
		Kind:    "NamespaceClassBinding",
	}
	namespaceGVK = schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Namespace",
	}
)

func TestEnvtestRegistersCRDsAndStatusSubresource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join(repoRoot(t), "config", "crd", "bases")},
	}

	restConfig, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("stop envtest: %v", err)
		}
	}()

	kubeClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "public-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set resources: %v", err)
	}
	if err := kubeClient.Create(ctx, namespaceClass); err != nil {
		t.Fatalf("create namespaceclass: %v", err)
	}

	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	binding := newUnstructured(namespaceClassBindingGVK, "", "web-portal")
	if err := unstructured.SetNestedField(binding.Object, "web-portal", "spec", "namespaceName"); err != nil {
		t.Fatalf("set namespaceName: %v", err)
	}
	if err := unstructured.SetNestedField(binding.Object, "public-network", "spec", "className"); err != nil {
		t.Fatalf("set className: %v", err)
	}
	if err := kubeClient.Create(ctx, binding); err != nil {
		t.Fatalf("create binding: %v", err)
	}

	bindingStatus := binding.DeepCopy()
	if err := unstructured.SetNestedSlice(bindingStatus.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"namespace":  "web-portal",
			"name":       "public-app",
		},
	}, "status", "inventory"); err != nil {
		t.Fatalf("set inventory status: %v", err)
	}
	if err := unstructured.SetNestedSlice(bindingStatus.Object, []interface{}{
		map[string]interface{}{
			"type":               "Ready",
			"status":             "True",
			"reason":             "ReconcileSucceeded",
			"lastTransitionTime": metav1.Now().Format(time.RFC3339),
		},
	}, "status", "conditions"); err != nil {
		t.Fatalf("set conditions status: %v", err)
	}
	if err := kubeClient.Status().Update(ctx, bindingStatus); err != nil {
		t.Fatalf("update binding status: %v", err)
	}

	got := newUnstructured(namespaceClassBindingGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, got); err != nil {
		t.Fatalf("get binding: %v", err)
	}
	inventory, found, err := unstructured.NestedSlice(got.Object, "status", "inventory")
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if !found || len(inventory) != 1 {
		t.Fatalf("expected one inventory entry, got found=%v len=%d", found, len(inventory))
	}
}

func newUnstructured(gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
