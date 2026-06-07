package controller

import (
	"context"
	"fmt"

	namespaceclassv1alpha1 "github.com/forest/namespace-class/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
)

type NamespaceReconciler struct {
	client.Client
	APIReader client.Reader
}

func SetupNamespaceReconciler(mgr ctrl.Manager) error {
	skipNameValidation := true
	reconciler := &NamespaceReconciler{
		Client:    mgr.GetClient(),
		APIReader: mgr.GetAPIReader(),
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("namespaceclass-binding").
		WithOptions(crcontroller.Options{SkipNameValidation: &skipNameValidation}).
		For(&corev1.Namespace{}).
		Complete(reconciler)
}

func (r *NamespaceReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, client.ObjectKey{Name: request.Name}, namespace); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get namespace: %w", err)
	}

	if !namespace.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	className := namespace.Labels[namespaceclassv1alpha1.ClassLabelKey]
	if className == "" {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileBinding(ctx, namespace, className); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) reconcileBinding(ctx context.Context, namespace *corev1.Namespace, className string) error {
	observedClassGeneration, condition := r.classCondition(ctx, className)

	bindingName := namespace.Name
	binding := &namespaceclassv1alpha1.NamespaceClassBinding{}
	key := client.ObjectKey{Name: bindingName}
	err := r.Get(ctx, key, binding)
	if apierrors.IsNotFound(err) {
		binding = &namespaceclassv1alpha1.NamespaceClassBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: namespaceclassv1alpha1.GroupVersion.String(),
				Kind:       "NamespaceClassBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: bindingName,
			},
			Spec: namespaceclassv1alpha1.NamespaceClassBindingSpec{
				NamespaceName: namespace.Name,
				ClassName:     className,
			},
		}
		if err := r.Create(ctx, binding); err != nil {
			return fmt.Errorf("create namespaceclassbinding: %w", err)
		}
		if err := r.Get(ctx, key, binding); err != nil {
			return fmt.Errorf("get created namespaceclassbinding: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get namespaceclassbinding: %w", err)
	} else if binding.Spec.NamespaceName != namespace.Name || binding.Spec.ClassName != className {
		binding.Spec.NamespaceName = namespace.Name
		binding.Spec.ClassName = className
		if err := r.Update(ctx, binding); err != nil {
			return fmt.Errorf("update namespaceclassbinding spec: %w", err)
		}
		if err := r.Get(ctx, key, binding); err != nil {
			return fmt.Errorf("get updated namespaceclassbinding: %w", err)
		}
	}

	binding.Status.ObservedNamespaceUID = string(namespace.UID)
	binding.Status.ObservedClassGeneration = observedClassGeneration
	condition.ObservedGeneration = binding.Generation
	meta.SetStatusCondition(&binding.Status.Conditions, condition)
	if err := r.Status().Update(ctx, binding); err != nil {
		return fmt.Errorf("update namespaceclassbinding status: %w", err)
	}

	return nil
}

func (r *NamespaceReconciler) classCondition(ctx context.Context, className string) (int64, metav1.Condition) {
	namespaceClass := &namespaceclassv1alpha1.NamespaceClass{}
	if err := r.APIReader.Get(ctx, client.ObjectKey{Name: className}, namespaceClass); err != nil {
		if apierrors.IsNotFound(err) {
			return 0, metav1.Condition{
				Type:    namespaceclassv1alpha1.ConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  namespaceclassv1alpha1.ReasonClassNotFound,
				Message: fmt.Sprintf("NamespaceClass %q was not found", className),
			}
		}
		return 0, metav1.Condition{
			Type:    namespaceclassv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ClassReadFailed",
			Message: err.Error(),
		}
	}

	return namespaceClass.Generation, metav1.Condition{
		Type:    namespaceclassv1alpha1.ConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  namespaceclassv1alpha1.ReasonBindingRecorded,
		Message: "NamespaceClass binding recorded",
	}
}
