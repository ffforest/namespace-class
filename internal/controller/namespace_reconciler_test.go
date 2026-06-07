package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	namespaceclassv1alpha1 "github.com/forest/namespace-class/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileDoesNotCleanupInventoryWhenNamespaceClassReadFails(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := namespaceclassv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add namespaceclass scheme: %v", err)
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web-portal",
			UID:  types.UID("namespace-uid"),
			Labels: map[string]string{
				namespaceclassv1alpha1.ClassLabelKey: "public-network",
			},
		},
	}
	binding := &namespaceclassv1alpha1.NamespaceClassBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "web-portal"},
		Spec: namespaceclassv1alpha1.NamespaceClassBindingSpec{
			NamespaceName: "web-portal",
			ClassName:     "public-network",
		},
		Status: namespaceclassv1alpha1.NamespaceClassBindingStatus{
			ObservedNamespaceUID:    "namespace-uid",
			ObservedClassGeneration: 1,
			Inventory: []namespaceclassv1alpha1.ResourceRef{{
				APIVersion: "v1",
				Kind:       "ServiceAccount",
				Namespace:  "web-portal",
				Name:       "old-app",
			}},
		},
	}
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-app",
			Namespace: "web-portal",
			Labels: map[string]string{
				namespaceclassv1alpha1.ManagedLabelKey:        "true",
				namespaceclassv1alpha1.ClassLabelOwnerKey:     "public-network",
				namespaceclassv1alpha1.NamespaceLabelOwnerKey: "web-portal",
			},
			Annotations: map[string]string{
				namespaceclassv1alpha1.OwnerNamespaceUIDAnnoKey: "namespace-uid",
			},
		},
	}

	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespace, binding, serviceAccount).
		Build()
	classReadErr := errors.New("api reader unavailable")
	reconciler := &NamespaceReconciler{
		Client: kubeClient,
		APIReader: namespaceClassFailingReader{
			delegate: kubeClient,
			err:      classReadErr,
		},
	}

	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Name: "web-portal"}})
	if err == nil {
		t.Errorf("expected NamespaceClass read failure to be returned")
	} else if !strings.Contains(err.Error(), classReadErr.Error()) {
		t.Errorf("expected error to include class read failure %q, got %q", classReadErr.Error(), err.Error())
	}

	gotBinding := &namespaceclassv1alpha1.NamespaceClassBinding{}
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: "web-portal"}, gotBinding); err != nil {
		t.Errorf("expected binding to remain after class read failure: %v", err)
	}
	if len(gotBinding.Status.Inventory) != 1 {
		t.Fatalf("expected binding inventory to remain unchanged, got %#v", gotBinding.Status.Inventory)
	}
	if gotBinding.Status.Inventory[0] != binding.Status.Inventory[0] {
		t.Fatalf("expected binding inventory to remain unchanged, got %#v", gotBinding.Status.Inventory[0])
	}
	if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: "web-portal", Name: "old-app"}, &corev1.ServiceAccount{}); err != nil {
		t.Errorf("expected managed resource to remain after class read failure: %v", err)
	}
}

func TestTemplateValuesIncludeNamespaceLabelsAndAnnotations(t *testing.T) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web-portal",
			UID:  types.UID("namespace-uid"),
			Labels: map[string]string{
				"team.example.com/name": "platform",
			},
			Annotations: map[string]string{
				"owner.example.com/name": "forest",
			},
		},
	}

	values := templateValues(namespace, "public-network")

	if got := values[".Namespace.Labels.team.example.com/name"]; got != "platform" {
		t.Fatalf("expected namespace label template value, got %q", got)
	}
	if got := values[".Namespace.Annotations.owner.example.com/name"]; got != "forest" {
		t.Fatalf("expected namespace annotation template value, got %q", got)
	}
}

type namespaceClassFailingReader struct {
	delegate client.Reader
	err      error
}

func (r namespaceClassFailingReader) Get(ctx context.Context, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
	if _, ok := object.(*namespaceclassv1alpha1.NamespaceClass); ok {
		return r.err
	}
	return r.delegate.Get(ctx, key, object, options...)
}

func (r namespaceClassFailingReader) List(ctx context.Context, list client.ObjectList, options ...client.ListOption) error {
	return r.delegate.List(ctx, list, options...)
}
