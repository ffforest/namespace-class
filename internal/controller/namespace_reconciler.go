package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	namespaceclassv1alpha1 "github.com/forest/namespace-class/api/v1alpha1"
	"github.com/forest/namespace-class/internal/policy"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const fieldManager = "namespace-class-controller"
const bindingClassNameField = "spec.className"

type NamespaceReconciler struct {
	client.Client
	APIReader      client.Reader
	RESTMapper     meta.RESTMapper
	ResourcePolicy policy.Policy
}

func SetupNamespaceReconciler(mgr ctrl.Manager, resourcePolicy policy.Policy) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &namespaceclassv1alpha1.NamespaceClassBinding{}, bindingClassNameField, func(object client.Object) []string {
		binding, ok := object.(*namespaceclassv1alpha1.NamespaceClassBinding)
		if !ok || binding.Spec.ClassName == "" {
			return nil
		}
		return []string{binding.Spec.ClassName}
	}); err != nil {
		return fmt.Errorf("index namespaceclassbindings by class name: %w", err)
	}

	skipNameValidation := true
	reconciler := &NamespaceReconciler{
		Client:         mgr.GetClient(),
		APIReader:      mgr.GetAPIReader(),
		RESTMapper:     mgr.GetRESTMapper(),
		ResourcePolicy: resourcePolicy,
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("namespaceclass-binding").
		WithOptions(crcontroller.Options{SkipNameValidation: &skipNameValidation}).
		For(&corev1.Namespace{}).
		Watches(
			&namespaceclassv1alpha1.NamespaceClass{},
			handler.EnqueueRequestsFromMapFunc(reconciler.requestsForNamespaceClass),
			builder.WithPredicates(namespaceClassUpdateFanoutPredicate()),
		).
		Complete(reconciler)
}

func namespaceClassUpdateFanoutPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func (r *NamespaceReconciler) requestsForNamespaceClass(ctx context.Context, object client.Object) []reconcile.Request {
	bindings := &namespaceclassv1alpha1.NamespaceClassBindingList{}
	if err := r.List(ctx, bindings, client.MatchingFields{bindingClassNameField: object.GetName()}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "list NamespaceClassBindings for NamespaceClass fan-out", "namespaceClass", object.GetName())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(bindings.Items))
	seen := map[string]struct{}{}
	for _, binding := range bindings.Items {
		namespaceName := binding.Spec.NamespaceName
		if namespaceName == "" {
			namespaceName = binding.Name
		}
		if namespaceName == "" {
			continue
		}
		if _, found := seen[namespaceName]; found {
			continue
		}
		seen[namespaceName] = struct{}{}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{Name: namespaceName},
		})
	}
	return requests
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
		if hasNamespaceFinalizer(namespace) {
			if err := r.cleanupBindingForDeletingNamespace(ctx, namespace); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.removeNamespaceFinalizer(ctx, namespace); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	className := namespace.Labels[namespaceclassv1alpha1.ClassLabelKey]
	if className == "" {
		if err := r.cleanupBindingForNamespace(ctx, namespace); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.removeNamespaceFinalizer(ctx, namespace); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcileBinding(ctx, namespace, className); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) cleanupBindingForDeletingNamespace(ctx context.Context, namespace *corev1.Namespace) error {
	binding := &namespaceclassv1alpha1.NamespaceClassBinding{}
	if err := r.Get(ctx, client.ObjectKey{Name: namespace.Name}, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get namespaceclassbinding for namespace deletion cleanup: %w", err)
	}

	clusterScopedInventory := clusterScopedResourceRefs(binding.Status.Inventory)
	if err := r.deleteStaleManagedResources(ctx, namespace, clusterScopedInventory, nil); err != nil {
		return err
	}
	if err := r.Delete(ctx, binding); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespaceclassbinding after namespace deletion cleanup: %w", err)
	}
	return nil
}

func (r *NamespaceReconciler) cleanupBindingForNamespace(ctx context.Context, namespace *corev1.Namespace) error {
	binding := &namespaceclassv1alpha1.NamespaceClassBinding{}
	if err := r.Get(ctx, client.ObjectKey{Name: namespace.Name}, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get namespaceclassbinding for cleanup: %w", err)
	}

	previousInventory := append([]namespaceclassv1alpha1.ResourceRef(nil), binding.Status.Inventory...)
	if err := r.deleteStaleManagedResources(ctx, namespace, previousInventory, nil); err != nil {
		return err
	}
	if err := r.Delete(ctx, binding); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete namespaceclassbinding after cleanup: %w", err)
	}
	return nil
}

func (r *NamespaceReconciler) reconcileBinding(ctx context.Context, namespace *corev1.Namespace, className string) error {
	namespaceClass, observedClassGeneration, condition := r.classCondition(ctx, className)
	if namespaceClass != nil {
		if err := r.ensureNamespaceFinalizer(ctx, namespace); err != nil {
			return err
		}
	}

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

	previousInventory := append([]namespaceclassv1alpha1.ResourceRef(nil), binding.Status.Inventory...)
	if namespaceClass == nil {
		if err := r.deleteStaleManagedResources(ctx, namespace, previousInventory, nil); err != nil {
			binding.Status.ObservedNamespaceUID = string(namespace.UID)
			binding.Status.ObservedClassGeneration = observedClassGeneration
			condition.Reason = namespaceclassv1alpha1.ReasonCleanupFailed
			condition.Message = fmt.Sprintf("NamespaceClass %q was not found and cleanup failed: %v", className, err)
			condition.ObservedGeneration = binding.Generation
			meta.SetStatusCondition(&binding.Status.Conditions, condition)
			if statusErr := r.Status().Update(ctx, binding); statusErr != nil {
				return fmt.Errorf("cleanup missing namespaceclass: %w; update cleanup status: %w", err, statusErr)
			}
			return fmt.Errorf("cleanup missing namespaceclass: %w", err)
		}
		if err := r.Delete(ctx, binding); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete namespaceclassbinding after missing class cleanup: %w", err)
		}
		return r.removeNamespaceFinalizer(ctx, namespace)
	}

	inventory, err := r.applyManagedResources(ctx, namespace, namespaceClass)
	if err != nil {
		var deniedErr *policy.DeniedError
		if errors.As(err, &deniedErr) {
			binding.Status.ObservedNamespaceUID = string(namespace.UID)
			binding.Status.ObservedClassGeneration = observedClassGeneration
			binding.Status.Inventory = previousInventory
			condition.Status = metav1.ConditionFalse
			condition.Reason = namespaceclassv1alpha1.ReasonGVKDenied
			condition.Message = err.Error()
			condition.ObservedGeneration = binding.Generation
			meta.SetStatusCondition(&binding.Status.Conditions, condition)
			if statusErr := r.Status().Update(ctx, binding); statusErr != nil {
				return fmt.Errorf("update namespaceclassbinding policy denied status: %w", statusErr)
			}
			return nil
		}
		return err
	}
	if err := r.deleteStaleManagedResources(ctx, namespace, previousInventory, inventory); err != nil {
		return err
	}

	binding.Status.ObservedNamespaceUID = string(namespace.UID)
	binding.Status.ObservedClassGeneration = observedClassGeneration
	binding.Status.Inventory = inventory
	condition.ObservedGeneration = binding.Generation
	meta.SetStatusCondition(&binding.Status.Conditions, condition)
	if err := r.Status().Update(ctx, binding); err != nil {
		return fmt.Errorf("update namespaceclassbinding status: %w", err)
	}

	return nil
}

func clusterScopedResourceRefs(refs []namespaceclassv1alpha1.ResourceRef) []namespaceclassv1alpha1.ResourceRef {
	clusterScoped := []namespaceclassv1alpha1.ResourceRef{}
	for _, ref := range refs {
		if ref.Namespace == "" {
			clusterScoped = append(clusterScoped, ref)
		}
	}
	return clusterScoped
}

func (r *NamespaceReconciler) ensureNamespaceFinalizer(ctx context.Context, namespace *corev1.Namespace) error {
	if hasNamespaceFinalizer(namespace) {
		return nil
	}
	namespace.SetFinalizers(append(namespace.GetFinalizers(), namespaceclassv1alpha1.NamespaceFinalizer))
	if err := r.Update(ctx, namespace); err != nil {
		return fmt.Errorf("add namespace finalizer: %w", err)
	}
	return nil
}

func (r *NamespaceReconciler) removeNamespaceFinalizer(ctx context.Context, namespace *corev1.Namespace) error {
	if !hasNamespaceFinalizer(namespace) {
		return nil
	}
	finalizers := []string{}
	for _, finalizer := range namespace.GetFinalizers() {
		if finalizer != namespaceclassv1alpha1.NamespaceFinalizer {
			finalizers = append(finalizers, finalizer)
		}
	}
	namespace.SetFinalizers(finalizers)
	if err := r.Update(ctx, namespace); err != nil {
		return fmt.Errorf("remove namespace finalizer: %w", err)
	}
	return nil
}

func hasNamespaceFinalizer(namespace *corev1.Namespace) bool {
	for _, finalizer := range namespace.GetFinalizers() {
		if finalizer == namespaceclassv1alpha1.NamespaceFinalizer {
			return true
		}
	}
	return false
}

func (r *NamespaceReconciler) applyManagedResources(ctx context.Context, namespace *corev1.Namespace, namespaceClass *namespaceclassv1alpha1.NamespaceClass) ([]namespaceclassv1alpha1.ResourceRef, error) {
	resources, err := r.prepareManagedResources(ctx, namespace, namespaceClass)
	if err != nil {
		return nil, err
	}

	refs := make([]namespaceclassv1alpha1.ResourceRef, 0, len(resources))
	for _, resource := range resources {
		if err := r.Patch(ctx, resource.object, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
			return nil, fmt.Errorf("apply %s/%s: %w", resource.object.GetKind(), resource.object.GetName(), err)
		}
		refs = append(refs, resource.ref)
	}

	sort.Slice(refs, func(i, j int) bool {
		return resourceRefKey(refs[i]) < resourceRefKey(refs[j])
	})
	return refs, nil
}

type preparedManagedResource struct {
	object *unstructured.Unstructured
	ref    namespaceclassv1alpha1.ResourceRef
}

func (r *NamespaceReconciler) prepareManagedResources(ctx context.Context, namespace *corev1.Namespace, namespaceClass *namespaceclassv1alpha1.NamespaceClass) ([]preparedManagedResource, error) {
	resources := []preparedManagedResource{}
	for index, raw := range namespaceClass.Spec.Resources {
		object, err := rawToUnstructured(raw)
		if err != nil {
			return nil, fmt.Errorf("parse resource %d: %w", index, err)
		}

		ref, err := r.prepareManagedResource(ctx, object, namespace, namespaceClass.Name)
		if err != nil {
			return nil, fmt.Errorf("prepare %s/%s: %w", object.GetKind(), object.GetName(), err)
		}
		resources = append(resources, preparedManagedResource{object: object, ref: ref})
	}

	return resources, nil
}

func (r *NamespaceReconciler) deleteStaleManagedResources(ctx context.Context, namespace *corev1.Namespace, previousInventory, desiredInventory []namespaceclassv1alpha1.ResourceRef) error {
	desiredKeys := map[string]struct{}{}
	for _, ref := range desiredInventory {
		desiredKeys[resourceRefKey(ref)] = struct{}{}
	}

	for _, ref := range previousInventory {
		if _, found := desiredKeys[resourceRefKey(ref)]; found {
			continue
		}

		object, err := objectFromResourceRef(ref)
		if err != nil {
			return fmt.Errorf("prepare stale inventory ref: %w", err)
		}
		if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: object.GetNamespace(), Name: object.GetName()}, object); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("get stale %s/%s: %w", ref.Kind, ref.Name, err)
		}

		if !isManagedByNamespace(object, namespace) {
			return fmt.Errorf("stale resource %s/%s is not owned by this NamespaceClass binding", ref.Kind, ref.Name)
		}
		if err := r.Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete stale %s/%s: %w", ref.Kind, ref.Name, err)
		}
	}
	return nil
}

func resourceRefKey(ref namespaceclassv1alpha1.ResourceRef) string {
	return fmt.Sprintf("%s/%s/%s/%s", ref.APIVersion, ref.Kind, ref.Namespace, ref.Name)
}

func objectFromResourceRef(ref namespaceclassv1alpha1.ResourceRef) (*unstructured.Unstructured, error) {
	groupVersion, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("parse apiVersion %q: %w", ref.APIVersion, err)
	}
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(groupVersion.WithKind(ref.Kind))
	object.SetNamespace(ref.Namespace)
	object.SetName(ref.Name)
	return object, nil
}

func rawToUnstructured(raw runtime.RawExtension) (*unstructured.Unstructured, error) {
	if len(raw.Raw) == 0 {
		if raw.Object == nil {
			return nil, fmt.Errorf("resource body is empty")
		}
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(raw.Object)
		if err != nil {
			return nil, fmt.Errorf("convert object: %w", err)
		}
		return &unstructured.Unstructured{Object: content}, nil
	}

	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(raw.Raw, &object.Object); err != nil {
		return nil, fmt.Errorf("unmarshal raw object: %w", err)
	}
	return object, nil
}

func (r *NamespaceReconciler) prepareManagedResource(ctx context.Context, object *unstructured.Unstructured, namespace *corev1.Namespace, className string) (namespaceclassv1alpha1.ResourceRef, error) {
	gvk := object.GroupVersionKind()
	if gvk.Empty() || gvk.Kind == "" {
		return namespaceclassv1alpha1.ResourceRef{}, fmt.Errorf("apiVersion and kind are required")
	}
	if object.GetName() == "" {
		return namespaceclassv1alpha1.ResourceRef{}, fmt.Errorf("metadata.name is required")
	}
	if err := r.ResourcePolicy.Validate(gvk); err != nil {
		return namespaceclassv1alpha1.ResourceRef{}, err
	}

	mapping, err := r.RESTMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return namespaceclassv1alpha1.ResourceRef{}, fmt.Errorf("resolve REST mapping: %w", err)
	}
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		object.SetNamespace(namespace.Name)
	} else {
		object.SetNamespace("")
	}

	if err := r.ensureCanManage(ctx, object, namespace); err != nil {
		return namespaceclassv1alpha1.ResourceRef{}, err
	}
	addManagedMetadata(object, namespace, className)
	object.SetResourceVersion("")
	object.SetManagedFields(nil)

	return namespaceclassv1alpha1.ResourceRef{
		APIVersion: object.GetAPIVersion(),
		Kind:       object.GetKind(),
		Namespace:  object.GetNamespace(),
		Name:       object.GetName(),
	}, nil
}

func (r *NamespaceReconciler) ensureCanManage(ctx context.Context, desired *unstructured.Unstructured, namespace *corev1.Namespace) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	key := client.ObjectKey{Namespace: desired.GetNamespace(), Name: desired.GetName()}
	if err := r.APIReader.Get(ctx, key, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get existing resource: %w", err)
	}

	if !isManagedByNamespace(existing, namespace) {
		return fmt.Errorf("resource already exists and is not owned by this NamespaceClass binding")
	}
	return nil
}

func isManagedByNamespace(object *unstructured.Unstructured, namespace *corev1.Namespace) bool {
	labels := object.GetLabels()
	annotations := object.GetAnnotations()
	return labels[namespaceclassv1alpha1.ManagedLabelKey] == "true" &&
		annotations[namespaceclassv1alpha1.OwnerNamespaceUIDAnnoKey] == string(namespace.UID)
}

func addManagedMetadata(object *unstructured.Unstructured, namespace *corev1.Namespace, className string) {
	labels := object.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[namespaceclassv1alpha1.ManagedLabelKey] = "true"
	labels[namespaceclassv1alpha1.ClassLabelOwnerKey] = className
	labels[namespaceclassv1alpha1.NamespaceLabelOwnerKey] = namespace.Name
	object.SetLabels(labels)

	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[namespaceclassv1alpha1.OwnerNamespaceUIDAnnoKey] = string(namespace.UID)
	object.SetAnnotations(annotations)
}

func (r *NamespaceReconciler) classCondition(ctx context.Context, className string) (*namespaceclassv1alpha1.NamespaceClass, int64, metav1.Condition) {
	namespaceClass := &namespaceclassv1alpha1.NamespaceClass{}
	if err := r.APIReader.Get(ctx, client.ObjectKey{Name: className}, namespaceClass); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, 0, metav1.Condition{
				Type:    namespaceclassv1alpha1.ConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  namespaceclassv1alpha1.ReasonClassNotFound,
				Message: fmt.Sprintf("NamespaceClass %q was not found", className),
			}
		}
		return nil, 0, metav1.Condition{
			Type:    namespaceclassv1alpha1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  "ClassReadFailed",
			Message: err.Error(),
		}
	}

	return namespaceClass, namespaceClass.Generation, metav1.Condition{
		Type:    namespaceclassv1alpha1.ConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  namespaceclassv1alpha1.ReasonBindingRecorded,
		Message: "NamespaceClass binding recorded",
	}
}
