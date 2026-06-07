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

var clusterRoleGVK = schema.GroupVersionKind{
	Group:   "rbac.authorization.k8s.io",
	Version: "v1",
	Kind:    "ClusterRole",
}

var clusterRoleBindingGVK = schema.GroupVersionKind{
	Group:   "rbac.authorization.k8s.io",
	Version: "v1",
	Kind:    "ClusterRoleBinding",
}

var configMapGVK = schema.GroupVersionKind{
	Version: "v1",
	Kind:    "ConfigMap",
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

func TestNamespaceClassDeletionDeletesResourcesAndBinding(t *testing.T) {
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

	namespaceClass = newUnstructured(namespaceClassGVK, "", "public-network")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "public-network"}, namespaceClass); err != nil {
		t.Fatalf("get namespaceclass for deletion: %v", err)
	}
	if err := kubeClient.Delete(ctx, namespaceClass); err != nil {
		t.Fatalf("delete namespaceclass: %v", err)
	}

	waitForObjectDeleted(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	waitForBindingDeleted(t, ctx, kubeClient, "web-portal")
}

func TestNamespaceDeletionCleansClusterScopedResourcesAndBinding(t *testing.T) {
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
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "web-portal-public-reader",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{""},
					"resources": []interface{}{"pods"},
					"verbs":     []interface{}{"get"},
				},
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

	waitForObject(t, ctx, kubeClient, clusterRoleGVK, "", "web-portal-public-reader")
	binding := waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
	if !hasInventoryEntry(t, binding, "rbac.authorization.k8s.io/v1", "ClusterRole", "", "web-portal-public-reader") {
		t.Fatalf("expected ClusterRole inventory entry, got %#v", binding.Object["status"])
	}
	waitForNamespaceFinalizer(t, ctx, kubeClient, "web-portal", "namespaceclass.akuity.io/finalizer")

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace for deletion: %v", err)
	}
	if err := kubeClient.Delete(ctx, namespace); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}

	waitForObjectDeleted(t, ctx, kubeClient, clusterRoleGVK, "", "web-portal-public-reader")
	waitForBindingDeleted(t, ctx, kubeClient, "web-portal")
	waitForNamespaceFinalizerRemoved(t, ctx, kubeClient, "web-portal", "namespaceclass.akuity.io/finalizer")
}

func TestDefaultGVKPolicyDeniesClusterRoleBinding(t *testing.T) {
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
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "web-portal-cluster-admin",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "cluster-admin",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "default",
					"namespace": "web-portal",
				},
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

	binding := waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "GVKDenied")
	if got := nestedString(t, binding, "status", "observedNamespaceUID"); got == "" {
		t.Fatal("expected observedNamespaceUID to be recorded")
	}
	waitForObjectDeleted(t, ctx, kubeClient, clusterRoleBindingGVK, "", "web-portal-cluster-admin")
}

func TestUnmanagedExistingResourceConflictSetsApplyConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	unmanagedServiceAccount := newUnstructured(serviceAccountGVK, "web-portal", "public-app")
	if err := kubeClient.Create(ctx, unmanagedServiceAccount); err != nil {
		t.Fatalf("create unmanaged serviceaccount: %v", err)
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

	labelNamespaceClass(t, ctx, kubeClient, "web-portal", "public-network")
	binding := waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "ApplyConflict")
	if hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "public-app") {
		t.Fatalf("expected unmanaged conflicting resource not to be recorded in inventory, got %#v", binding.Object["status"])
	}
}

func TestServerSideApplyConflictSetsApplyConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	createManagedConfigMapWithFieldOwner(t, ctx, kubeClient, namespace, "public-config", "other-manager", map[string]interface{}{
		"mode": "other",
	})

	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "public-config",
			},
			"data": map[string]interface{}{
				"mode": "controller",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set resources: %v", err)
	}
	if err := kubeClient.Create(ctx, namespaceClass); err != nil {
		t.Fatalf("create namespaceclass: %v", err)
	}

	labelNamespaceClass(t, ctx, kubeClient, "web-portal", "public-network")
	waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "ApplyConflict")

	configMap := waitForObject(t, ctx, kubeClient, configMapGVK, "web-portal", "public-config")
	if got := nestedString(t, configMap, "data", "mode"); got != "other" {
		t.Fatalf("expected other field manager value to remain, got %q", got)
	}
}

func TestPartialApplyFailureRecordsSuccessfulInventory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	createManagedConfigMapWithFieldOwner(t, ctx, kubeClient, namespace, "conflicting-config", "other-manager", map[string]interface{}{
		"mode": "other",
	})

	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "first-config",
			},
			"data": map[string]interface{}{
				"mode": "created",
			},
		},
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "conflicting-config",
			},
			"data": map[string]interface{}{
				"mode": "controller",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set resources: %v", err)
	}
	if err := kubeClient.Create(ctx, namespaceClass); err != nil {
		t.Fatalf("create namespaceclass: %v", err)
	}

	labelNamespaceClass(t, ctx, kubeClient, "web-portal", "public-network")
	binding := waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "ApplyConflict")
	waitForObject(t, ctx, kubeClient, configMapGVK, "web-portal", "first-config")
	if !hasInventoryEntry(t, binding, "v1", "ConfigMap", "web-portal", "first-config") {
		t.Fatalf("expected successfully applied resource to be recorded in inventory, got %#v", binding.Object["status"])
	}
}

func TestUnknownGVKSetsApplyFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "MissingKind",
			"metadata": map[string]interface{}{
				"name": "missing",
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set resources: %v", err)
	}
	if err := kubeClient.Create(ctx, namespaceClass); err != nil {
		t.Fatalf("create namespaceclass: %v", err)
	}

	waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "ApplyFailed")
}

func TestDuplicateDesiredIdentitySetsDuplicateResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "duplicate-app",
			},
		},
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "duplicate-app",
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

	waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "DuplicateResource")
}

func TestLabelRemovalDeleteFailureSetsDeleteFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	createNamespaceClassWithServiceAccount(t, ctx, kubeClient, "public-network", "public-app")
	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	serviceAccount := waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")

	labels := serviceAccount.GetLabels()
	delete(labels, "namespaceclass.akuity.io/managed")
	serviceAccount.SetLabels(labels)
	if err := kubeClient.Update(ctx, serviceAccount); err != nil {
		t.Fatalf("remove managed label: %v", err)
	}

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	labels = namespace.GetLabels()
	delete(labels, "namespaceclass.akuity.io/name")
	namespace.SetLabels(labels)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("remove namespace class label: %v", err)
	}

	waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "DeleteFailed")
}

func TestLabelRemovalDeleteFailureRetriesCleanupPeriodically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, func(options *ncmanager.Options) {
		options.ReconcileInterval = 200 * time.Millisecond
	})

	createNamespaceClassWithServiceAccount(t, ctx, kubeClient, "public-network", "public-app")
	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	serviceAccount := waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")

	labels := serviceAccount.GetLabels()
	delete(labels, "namespaceclass.akuity.io/managed")
	serviceAccount.SetLabels(labels)
	if err := kubeClient.Update(ctx, serviceAccount); err != nil {
		t.Fatalf("remove managed label: %v", err)
	}

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	labels = namespace.GetLabels()
	delete(labels, "namespaceclass.akuity.io/name")
	namespace.SetLabels(labels)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("remove namespace class label: %v", err)
	}
	waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "DeleteFailed")

	serviceAccount = waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	labels = serviceAccount.GetLabels()
	labels["namespaceclass.akuity.io/managed"] = "true"
	serviceAccount.SetLabels(labels)
	if err := kubeClient.Update(ctx, serviceAccount); err != nil {
		t.Fatalf("restore managed label: %v", err)
	}

	waitForObjectDeleted(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	waitForBindingDeleted(t, ctx, kubeClient, "web-portal")
}

func TestDamagedInventorySetsDeleteFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	createNamespaceClassWithServiceAccount(t, ctx, kubeClient, "public-network", "public-app")
	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	binding := waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")

	if err := unstructured.SetNestedSlice(binding.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "not/a/group/version",
			"kind":       "ServiceAccount",
			"namespace":  "web-portal",
			"name":       "public-app",
		},
	}, "status", "inventory"); err != nil {
		t.Fatalf("damage inventory: %v", err)
	}
	if err := kubeClient.Status().Update(ctx, binding); err != nil {
		t.Fatalf("update damaged binding inventory: %v", err)
	}

	namespace = newUnstructured(namespaceGVK, "", "web-portal")
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, namespace); err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	labels := namespace.GetLabels()
	delete(labels, "namespaceclass.akuity.io/name")
	namespace.SetLabels(labels)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("remove namespace class label: %v", err)
	}

	waitForBindingCondition(t, ctx, kubeClient, "web-portal", "Ready", "False", "DeleteFailed")
}

func TestTemplatesAreRenderedBeforeApply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	namespaceClass := newUnstructured(namespaceClassGVK, "", "public-network")
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "{{ .Namespace.Name }}-app",
				"labels": map[string]interface{}{
					"class": "{{ .Class.Name }}",
					"team":  "{{ .Namespace.Labels.team.example.com/name }}",
				},
				"annotations": map[string]interface{}{
					"owner": "{{ .Namespace.Annotations.owner.example.com/name }}",
				},
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
		"team.example.com/name":         "platform",
	})
	namespace.SetAnnotations(map[string]string{
		"owner.example.com/name": "forest",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	serviceAccount := waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "web-portal-app")
	if got := serviceAccount.GetLabels()["class"]; got != "public-network" {
		t.Fatalf("expected rendered class label, got %q", got)
	}
	if got := serviceAccount.GetLabels()["team"]; got != "platform" {
		t.Fatalf("expected rendered namespace label, got %q", got)
	}
	if got := serviceAccount.GetAnnotations()["owner"]; got != "forest" {
		t.Fatalf("expected rendered namespace annotation, got %q", got)
	}
	binding := waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
	if !hasInventoryEntry(t, binding, "v1", "ServiceAccount", "web-portal", "web-portal-app") {
		t.Fatalf("expected rendered ServiceAccount inventory entry, got %#v", binding.Object["status"])
	}
}

func TestNamespaceClassBindingDeletionRequeuesNamespace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, nil)

	createNamespaceClassWithServiceAccount(t, ctx, kubeClient, "public-network", "public-app")
	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	binding := waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
	if err := kubeClient.Delete(ctx, binding); err != nil {
		t.Fatalf("delete binding: %v", err)
	}

	waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
}

func TestPeriodicRequeueRepairsDeletedManagedResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	kubeClient := startControllerEnvtest(t, ctx, func(options *ncmanager.Options) {
		options.ReconcileInterval = 200 * time.Millisecond
	})

	createNamespaceClassWithServiceAccount(t, ctx, kubeClient, "public-network", "public-app")
	namespace := newUnstructured(namespaceGVK, "", "web-portal")
	namespace.SetLabels(map[string]string{
		"namespaceclass.akuity.io/name": "public-network",
	})
	if err := kubeClient.Create(ctx, namespace); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	serviceAccount := waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
	waitForBindingClass(t, ctx, kubeClient, "web-portal", "public-network")
	if err := kubeClient.Delete(ctx, serviceAccount); err != nil {
		t.Fatalf("delete managed serviceaccount: %v", err)
	}

	waitForObject(t, ctx, kubeClient, serviceAccountGVK, "web-portal", "public-app")
}

func startControllerEnvtest(t *testing.T, ctx context.Context, configure func(*ncmanager.Options)) client.Client {
	t.Helper()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join(repoRoot(t), "config", "crd", "bases")},
	}
	restConfig, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Fatalf("stop envtest: %v", err)
		}
	})

	options := ncmanager.Options{
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: freeLocalAddress(t),
		LeaderElection:         false,
	}
	if configure != nil {
		configure(&options)
	}
	mgr, err := ncmanager.New(restConfig, options)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	managerCtx, stopManager := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Start(managerCtx)
	}()
	t.Cleanup(func() {
		stopManager()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("manager returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("manager did not stop")
		}
	})

	kubeClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return kubeClient
}

func createNamespaceClassWithServiceAccount(t *testing.T, ctx context.Context, kubeClient client.Client, className, serviceAccountName string) {
	t.Helper()

	namespaceClass := newUnstructured(namespaceClassGVK, "", className)
	if err := unstructured.SetNestedSlice(namespaceClass.Object, []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": serviceAccountName,
			},
		},
	}, "spec", "resources"); err != nil {
		t.Fatalf("set resources: %v", err)
	}
	if err := kubeClient.Create(ctx, namespaceClass); err != nil {
		t.Fatalf("create namespaceclass: %v", err)
	}
}

func labelNamespaceClass(t *testing.T, ctx context.Context, kubeClient client.Client, namespaceName, className string) {
	t.Helper()

	namespace := newUnstructured(namespaceGVK, "", namespaceName)
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace); err != nil {
		t.Fatalf("get namespace for label: %v", err)
	}
	labels := namespace.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["namespaceclass.akuity.io/name"] = className
	namespace.SetLabels(labels)
	if err := kubeClient.Update(ctx, namespace); err != nil {
		t.Fatalf("label namespace: %v", err)
	}
}

func createManagedConfigMapWithFieldOwner(t *testing.T, ctx context.Context, kubeClient client.Client, namespace *unstructured.Unstructured, name, fieldOwner string, data map[string]interface{}) {
	t.Helper()

	configMap := newUnstructured(configMapGVK, namespace.GetName(), name)
	configMap.SetLabels(map[string]string{
		"namespaceclass.akuity.io/managed":   "true",
		"namespaceclass.akuity.io/class":     "public-network",
		"namespaceclass.akuity.io/namespace": namespace.GetName(),
	})
	configMap.SetAnnotations(map[string]string{
		"namespaceclass.akuity.io/owner-namespace-uid": string(namespace.GetUID()),
	})
	if err := unstructured.SetNestedStringMap(configMap.Object, stringMapFromInterfaceMap(data), "data"); err != nil {
		t.Fatalf("set configmap data: %v", err)
	}
	if err := kubeClient.Patch(ctx, configMap, client.Apply, client.FieldOwner(fieldOwner)); err != nil {
		t.Fatalf("apply managed configmap with field owner %q: %v", fieldOwner, err)
	}
}

func stringMapFromInterfaceMap(values map[string]interface{}) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		result[key] = value.(string)
	}
	return result
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

func waitForBindingCondition(t *testing.T, ctx context.Context, kubeClient client.Client, name, conditionType, status, reason string) *unstructured.Unstructured {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		binding := newUnstructured(namespaceClassBindingGVK, "", name)
		err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, binding)
		if err == nil && hasCondition(t, binding, conditionType, status, reason) {
			return binding
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("binding %q did not get condition %s=%s/%s: %v", name, conditionType, status, reason, lastErr)
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

func waitForNamespaceFinalizer(t *testing.T, ctx context.Context, kubeClient client.Client, name, finalizer string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		namespace := newUnstructured(namespaceGVK, "", name)
		err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, namespace)
		if err == nil && hasFinalizer(namespace, finalizer) {
			return
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("namespace %q did not get finalizer %q: %v", name, finalizer, lastErr)
}

func waitForNamespaceFinalizerRemoved(t *testing.T, ctx context.Context, kubeClient client.Client, name, finalizer string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		namespace := newUnstructured(namespaceGVK, "", name)
		err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, namespace)
		if apierrors.IsNotFound(err) {
			return
		}
		if err == nil && !hasFinalizer(namespace, finalizer) {
			return
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("namespace %q still has finalizer %q: %v", name, finalizer, lastErr)
}

func hasFinalizer(object *unstructured.Unstructured, finalizer string) bool {
	for _, item := range object.GetFinalizers() {
		if item == finalizer {
			return true
		}
	}
	return false
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
		entryNamespace, _ := entry["namespace"].(string)
		if entry["apiVersion"] == apiVersion && entry["kind"] == kind && entryNamespace == namespace && entry["name"] == name {
			return true
		}
	}
	return false
}
