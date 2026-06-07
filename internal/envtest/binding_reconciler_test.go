//go:build envtest

package envtest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	ncmanager "github.com/forest/namespace-class/internal/manager"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestNamespaceClassBindingIsCreatedForLabeledNamespace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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

	mgr, err := ncmanager.New(restConfig, ncmanager.Options{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: freeLocalAddress(t),
		LeaderElection:         false,
	})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(managerCtx)
	}()
	defer func() {
		stopManager()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("manager returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("manager did not stop")
		}
	}()

	kubeClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{}, "spec", "resources"); err != nil {
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

	binding := waitForBinding(t, ctx, kubeClient, "web-portal")
	if got := nestedString(t, binding, "spec", "namespaceName"); got != "web-portal" {
		t.Fatalf("unexpected namespaceName: %q", got)
	}
	if got := nestedString(t, binding, "spec", "className"); got != "public-network" {
		t.Fatalf("unexpected className: %q", got)
	}
	if got := nestedString(t, binding, "status", "observedNamespaceUID"); got == "" {
		t.Fatal("expected observedNamespaceUID to be recorded")
	}
	if !hasCondition(t, binding, "Ready", "True", "BindingRecorded") {
		t.Fatalf("expected Ready=True BindingRecorded condition, got %#v", binding.Object["status"])
	}
}

func waitForBinding(t *testing.T, ctx context.Context, kubeClient client.Client, name string) *unstructured.Unstructured {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		binding := newUnstructured(namespaceClassBindingGVK, "", name)
		err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, binding)
		if err == nil && nestedString(t, binding, "status", "observedNamespaceUID") != "" && hasCondition(t, binding, "Ready", "True", "BindingRecorded") {
			return binding
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("binding %q was not created: %v", name, lastErr)
	return nil
}

func nestedString(t *testing.T, object *unstructured.Unstructured, fields ...string) string {
	t.Helper()

	value, found, err := unstructured.NestedString(object.Object, fields...)
	if err != nil {
		t.Fatalf("read %v: %v", fields, err)
	}
	if !found {
		return ""
	}
	return value
}

func hasCondition(t *testing.T, object *unstructured.Unstructured, conditionType, status, reason string) bool {
	t.Helper()

	conditions, found, err := unstructured.NestedSlice(object.Object, "status", "conditions")
	if err != nil {
		t.Fatalf("read status.conditions: %v", err)
	}
	if !found {
		return false
	}

	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["status"] == status && condition["reason"] == reason {
			return true
		}
	}
	return false
}
