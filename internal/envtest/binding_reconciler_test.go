//go:build envtest

package envtest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	ncmanager "github.com/forest/namespace-class/internal/manager"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var serviceAccountGVK = schema.GroupVersionKind{
	Version: "v1",
	Kind:    "ServiceAccount",
}

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

func TestNamespaceClassResourcesAreAppliedAndRecordedInInventory(t *testing.T) {
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

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")

	binding := waitForBinding(t, ctx, kubeClient, "web-portal")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "public-app") {
		t.Fatalf("expected ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}
}

func TestNamespaceClassResourcesAbsentFromDesiredSetAreDeleted(t *testing.T) {
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
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "old-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set initial resources: %v", err)
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

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "old-app")
	binding := waitForBinding(t, ctx, kubeClient, "web-portal")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "old-app") {
		t.Fatalf("expected old ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}

	namespaceClass = newUnstructured(namespaceClassGVK, "", "public-network")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "public-network"}, namespaceClass); err != nil {
		t.Fatalf("get namespaceclass for update: %v", err)
	}
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "new-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set updated resources: %v", err)
	}
	if err := kubeClient.Update(ctx, namespaceClass); err != nil {
		t.Fatalf("update namespaceclass: %v", err)
	}

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace for reconcile trigger: %v", err)
	}
	annotations := namespace.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["test.namespaceclass.akuity.io/reconcile"] = "updated-class"
	namespace.SetAnnotations(annotations)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("trigger namespace reconcile: %v", err)
	}

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "new-app")
	waitForObjectDeleted(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "old-app")

	binding = waitForBinding(t, ctx, kubeClient, "web-portal")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "new-app") {
		t.Fatalf("expected new ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}
	if hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "old-app") {
		t.Fatalf("expected old ServiceAccount inventory entry to be removed, got %#v", binding.Object["status"])
	}
}

func TestNamespaceClassUpdatesReconcileBoundNamespaces(t *testing.T) {
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
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "old-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set initial resources: %v", err)
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

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "old-app")
	binding := waitForBinding(t, ctx, kubeClient, "web-portal")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "old-app") {
		t.Fatalf("expected old ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}

	namespaceClass = newUnstructured(namespaceClassGVK, "", "public-network")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "public-network"}, namespaceClass); err != nil {
		t.Fatalf("get namespaceclass for update: %v", err)
	}
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "new-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set updated resources: %v", err)
	}
	if err := kubeClient.Update(ctx, namespaceClass); err != nil {
		t.Fatalf("update namespaceclass: %v", err)
	}

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "new-app")
	waitForObjectDeleted(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "old-app")

	binding = waitForBinding(t, ctx, kubeClient, "web-portal")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "new-app") {
		t.Fatalf("expected new ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}
	if hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "old-app") {
		t.Fatalf("expected old ServiceAccount inventory entry to be removed, got %#v", binding.Object["status"])
	}
}

func TestNamespaceClassLabelSwitchCreatesNewResourcesAndDeletesOldResources(t *testing.T) {
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

	publicClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(publicClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "public-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set public resources: %v", err)
	}
	if err := kubeClient.Create(ctx, publicClass); err != nil {
		t.Fatalf("create public namespaceclass: %v", err)
	}

	internalClass := newUnstructured(namespaceClassGVK, "", "internal-network")
	if err := unstructured.SetNestedSlice(internalClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "internal-app",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set internal resources: %v", err)
	}
	if err := kubeClient.Create(ctx, internalClass); err != nil {
		t.Fatalf("create internal namespaceclass: %v", err)
	}

	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	binding := waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "public-app") {
		t.Fatalf("expected public ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace for class switch: %v", err)
	}
	labels := namespace.GetLabels()
	labels["namespaceclass.akuity.io/name"] = "internal-network"
	namespace.SetLabels(labels)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("switch namespace class: %v", err)
	}

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "internal-app")
	waitForObjectDeleted(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")

	binding = waitForBindingClass(t, ctx, kubeClient, "web-portal", "internal-network")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "internal-app") {
		t.Fatalf("expected internal ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}
	if hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "public-app") {
		t.Fatalf("expected public ServiceAccount inventory entry to be removed, got %#v", binding.Object["status"])
	}
}

func TestNamespaceClassLabelRemovalDeletesResourcesAndBinding(t *testing.T) {
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

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	binding := waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "public-app") {
		t.Fatalf("expected ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace for label removal: %v", err)
	}
	labels := namespace.GetLabels()
	delete(labels, "namespaceclass.akuity.io/name")
	namespace.SetLabels(labels)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("remove namespace class label: %v", err)
	}

	waitForObjectDeleted(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	waitForBindingDeleted(t, ctx, kubeClient, "web-portal")
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

func waitForBindingClass(t *testing.T, ctx context.Context, kubeClient client.Client, name, className string) *unstructured.Unstructured {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		binding := newUnstructured(namespaceClassBindingGVK, "", name)
		err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, binding)
		if err == nil &&
			nestedString(t, binding, "spec", "className") == className &&
			nestedString(t, binding, "status", "observedNamespaceUID") != "" &&
			hasCondition(t, binding, "Ready", "True", "BindingRecorded") {
			return binding
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("binding %q did not observe class %q: %v", name, className, lastErr)
	return nil
}

func waitForBindingDeleted(t *testing.T, ctx context.Context, kubeClient client.Client, name string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		binding := newUnstructured(namespaceClassBindingGVK, "", name)
		err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, binding)
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("binding %q was not deleted: %v", name, lastErr)
	}
	t.Fatalf("binding %q was not deleted", name)
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

func waitForObject(t *testing.T, ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		object := newUnstructured(gvk, namespace, name)
		err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, object)
		if err == nil {
			return object
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("%s %s/%s was not created: %v", gvk.Kind, namespace, name, lastErr)
	return nil
}

func waitForObjectDeleted(t *testing.T, ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, namespace, name string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		object := newUnstructured(gvk, namespace, name)
		err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, object)
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("%s %s/%s was not deleted: %v", gvk.Kind, namespace, name, lastErr)
	}
	t.Fatalf("%s %s/%s was not deleted", gvk.Kind, namespace, name)
}

func hasInventoryEntry(t *testing.T, object *unstructured.Unstructured, apiVersion, kind, namespace, name string) bool {
	t.Helper()

	inventory, found, err := unstructured.NestedSlice(object.Object, "status", "inventory")
	if err != nil {
		t.Fatalf("read status.inventory: %v", err)
	}
	if !found {
		return false
	}
	for _, item := range inventory {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if entry["apiVersion"] == apiVersion && entry["kind"] == kind && entry["namespace"] == namespace && entry["name"] == name {
			return true
		}
	}
	return false
}
