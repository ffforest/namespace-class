# NamespaceClass Design

## 1. Background

Kubernetes administrators want to define reusable classes of namespaces. A `NamespaceClass` describes the additional resources, policies, or configuration that should be created and maintained when a `Namespace` is bound to that class.

Example classes:

- `public-network`: create policies that allow public network access.
- `internal-network`: create policies that restrict access to internal networks or VPN ranges.

The design does not hard-code specific resource kinds. The core mechanism is a generic controller that accepts raw Kubernetes resource templates in `NamespaceClass` and reconciles them for namespaces labeled with that class.

## 2. Goals

1. Provide a `NamespaceClass` CRD for declaring namespace classes and their managed resource templates.
2. Watch namespace creation and updates, then reconcile resources based on the namespace class label.
3. Support switching a namespace from one class to another, including cleanup of resources that are no longer desired.
4. Support updates to a `NamespaceClass`, then fan out reconciliation to every namespace currently bound to that class.
5. Support arbitrary Kubernetes resources, including namespaced and cluster-scoped resources.
6. Follow the standard Kubernetes declarative reconciliation model.
7. Keep reconciliation recoverable when apply or delete operations partially fail.

## 3. Non-Goals

1. No rich template language. The first version only supports a small set of string substitutions.
2. No class inheritance.
3. No cross-cluster distribution.
4. No UI.
5. No automatic adoption of existing resources that are not already marked as managed by this controller.
6. No guarantee that cluster-scoped resources can be globally named without conflicts. Template authors are responsible for unique names.
7. No dynamic informer watch for every arbitrary managed resource kind in the first version.

## 4. API Design

### 4.1 NamespaceClass

`NamespaceClass` is a cluster-scoped CRD.

Recommended API group:

```text
namespaceclass.akuity.io
```

Example:

```yaml
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: public-network
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: public-app
```

The key field is:

```yaml
spec:
  resources:
    - <raw Kubernetes object>
```

Go shape:

```go
type NamespaceClassSpec struct {
    Resources []runtime.RawExtension `json:"resources,omitempty"`
}
```

The CRD schema preserves unknown fields for `spec.resources`, because every entry is a raw Kubernetes object.

### 4.2 Namespace Binding

A namespace is bound to a class through a label:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: web-portal
  labels:
    namespaceclass.akuity.io/name: public-network
```

The label key is:

```text
namespaceclass.akuity.io/name
```

The controller uses the namespace name as the reconcile key. `NamespaceClass` and `NamespaceClassBinding` events are converted into namespace reconcile requests.

### 4.3 NamespaceClassBinding

The controller creates one cluster-scoped `NamespaceClassBinding` for each namespace that has been managed by `NamespaceClass`.

This object is not the primary user-facing API. It is durable controller state for:

- the namespace name,
- the currently observed class name,
- observed generations,
- readiness conditions,
- managed resource inventory.

Example:

```yaml
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClassBinding
metadata:
  name: web-portal
spec:
  namespaceName: web-portal
  className: public-network
status:
  observedNamespaceUID: "..."
  observedClassGeneration: 3
  inventory:
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      namespace: web-portal
      name: allow-public-ingress
  conditions:
    - type: Ready
      status: "True"
      reason: BindingRecorded
```

The binding is cluster-scoped because:

1. Namespace deletion removes namespaced objects. Inventory stored inside the namespace could disappear before cluster-scoped resources are cleaned up.
2. `NamespaceClass` is cluster-scoped, so binding state also naturally belongs at cluster scope.
3. Administrators can query one object to inspect class binding, status, generation, inventory, and failure conditions.

The binding name is the namespace name, because namespace names are cluster-unique.

## 5. Resource Template Rules

Every object in `NamespaceClass.spec.resources` must contain:

```yaml
apiVersion: ...
kind: ...
metadata:
  name: ...
```

The controller processes each template as follows:

1. Render supported string templates.
2. Parse it into `unstructured.Unstructured`.
3. Validate required `apiVersion`, `kind`, and `metadata.name`.
4. Validate the GVK against the runtime allowlist/denylist policy.
5. Use `RESTMapper` to resolve whether the resource is namespaced or cluster-scoped.
6. Force namespaced resources into the target namespace.
7. Clear namespace for cluster-scoped resources.
8. Reject existing resources that are not already marked as managed by this namespace binding.
9. Add controller management labels and annotations.
10. Apply through server-side apply.

The current implementation normalizes namespace scope during reconciliation:

- namespaced resources are placed in the target namespace, even if the template
  specified another namespace;
- cluster-scoped resources have `metadata.namespace` cleared.

This keeps runtime behavior deterministic. A stricter validating webhook should
reject a namespaced template that explicitly targets a different namespace,
because that usually indicates a configuration error or an attempted escape from
the namespace binding.

Managed metadata:

```yaml
metadata:
  labels:
    namespaceclass.akuity.io/managed: "true"
    namespaceclass.akuity.io/class: public-network
    namespaceclass.akuity.io/namespace: web-portal
  annotations:
    namespaceclass.akuity.io/owner-namespace-uid: "<namespace-uid>"
```

Templates should not set controller-reserved labels or annotations. The current
runtime path overwrites reserved keys when it attaches ownership metadata. The
planned webhook should reject those templates earlier so authors get a clear
configuration error instead of relying on overwrite behavior.

### 5.1 Template Variables

The first version supports a deliberately small string-substitution variable set:

```text
{{ .Namespace.Name }}
{{ .Namespace.UID }}
{{ .Namespace.Labels.<key> }}
{{ .Namespace.Annotations.<key> }}
{{ .Class.Name }}
```

Constraints:

1. Variables are only rendered inside string values.
2. No conditionals, loops, functions, external lookups, or cross-resource references.
3. Unknown template variables make the whole class reconciliation fail before any apply begins.

The main use case is unique naming for cluster-scoped resources:

```yaml
metadata:
  name: "{{ .Namespace.Name }}-public-access"
```

### 5.2 Admission Webhook

Because `spec.resources` stores raw objects, CRD schema validation alone cannot fully validate entries.

A validating admission webhook is a planned enhancement. It should reject invalid resources earlier, before reconcile time.

The planned webhook should validate at least:

1. `apiVersion`, `kind`, and `metadata.name` are present.
2. Desired resource identities are unique within one `NamespaceClass`.
3. Templates do not set controller-reserved labels or annotations.
4. Namespaced resources do not explicitly target a namespace different from the
   bound namespace.
5. Template variables are within the supported variable set.
6. The GVK is allowed by configured allowlist/denylist policy.

The current implementation instead enforces key safety checks at runtime:

- required fields,
- duplicate desired resource identity,
- GVK allowlist/denylist,
- ownership conflicts,
- namespace scope normalization,
- unresolved GVK/scope errors.

The webhook cannot replace reconcile-time validation, because apply can still fail due to API discovery, RBAC, quotas, admission policies, field ownership conflicts, or target object state.

## 6. Controller Architecture

### 6.1 Watched Objects

The controller watches:

1. `Namespace`
2. `NamespaceClass`
3. `NamespaceClassBinding`

`Namespace` is the primary reconcile object. Events from `NamespaceClass` and `NamespaceClassBinding` only enqueue namespace reconcile requests.

The first implementation does not dynamically watch every managed resource GVK. `NamespaceClass.spec.resources` can reference arbitrary GVKs, including CRDs installed after the controller starts. Dynamic managed-resource watches would require dynamic informers, discovery cache handling, RESTMapper refresh, informer lifecycle management, RBAC failure handling, and memory controls.

Instead, the first version repairs drift through:

1. namespace events,
2. class fan-out events,
3. binding events,
4. periodic requeue.

### 6.2 Reconcile Flow

Namespace reconciliation is the only path that mutates managed resources.

High-level flow:

```text
Namespace event
  -> read Namespace
  -> if deleting: finalizer cleanup
  -> else read namespaceclass.akuity.io/name
  -> if label missing: cleanup binding inventory and delete binding
  -> if label exists: reconcile binding and desired resources
```

Detailed flow for a labeled namespace:

1. Read the referenced `NamespaceClass`.
2. If the API server returns `NotFound`, treat the desired set as empty and clean up old inventory.
3. If any other read error occurs, return an error and requeue. Do not clean up resources.
4. Ensure the namespace finalizer is present before managed resources are created.
5. Create or update the `NamespaceClassBinding` spec.
6. Read previous inventory from binding status.
7. Render, parse, validate, and prepare every desired resource before applying any resource.
8. Apply desired resources one by one through server-side apply.
9. If apply partially fails, record `oldInventory + appliedRefs`, write a failure condition, and skip stale deletion.
10. Only after every desired resource applies successfully, delete stale resources.
11. Write final inventory and readiness condition.

### 6.3 NamespaceClass Fan-Out

When a `NamespaceClass` is created, updated, or deleted:

1. List `NamespaceClassBinding` objects indexed by `spec.className`.
2. Extract each binding's `spec.namespaceName`.
3. Enqueue those namespaces.
4. Let namespace reconciliation perform all actual apply/delete work.

This avoids scanning all namespaces on every class update.

### 6.4 NamespaceClassBinding Watch

The controller also watches `NamespaceClassBinding`.

If a binding is deleted while the namespace still has a class label, the binding watch enqueues the namespace and the controller recreates the binding.

This is the first-slice drift repair path for binding state. Managed resources are still not watched dynamically.

## 7. Inventory Design

The controller must remember what it previously managed for a namespace. Without durable inventory, it cannot reliably delete resources that are no longer present in the current desired set.

The first version stores inventory in:

```text
NamespaceClassBinding.status.inventory
```

Each entry stores only identity:

```yaml
apiVersion: v1
kind: ServiceAccount
namespace: web-portal
name: public-app
```

Identity is:

```text
apiVersion + kind + namespace + name
```

Name alone is not enough because different kinds can share names. For cluster-scoped resources, `namespace` is empty.

Inventory does not store full object specs. This keeps binding status smaller and reduces the risk of stale state.

## 8. Apply and Delete Strategy

### 8.1 Server-Side Apply

Managed resources are created and updated with server-side apply using a fixed field manager:

```text
namespace-class-controller
```

Benefits:

1. It matches Kubernetes declarative controller conventions.
2. It avoids overwriting unrelated fields managed by others.
3. It surfaces field ownership conflicts.
4. It supports drift repair through repeated apply.

The controller does not use force ownership by default.

### 8.2 Existing Resources

If a desired resource already exists:

1. If it has controller ownership markers for the same namespace UID, the controller may update it.
2. If it does not have matching ownership markers, the controller refuses to manage it and records `Ready=False` / `ApplyConflict`.

This prevents accidental overwrite or adoption of user-created objects.

### 8.3 Partial Apply

Kubernetes does not provide a transaction across multiple resources. The correct behavior is recoverability, not rollback.

The controller handles partial apply as follows:

1. Render, parse, validate, and prepare all desired resources before applying any resource.
2. Apply resources one by one.
3. Record every successfully applied resource in `appliedRefs`.
4. If apply fails midway, do not delete stale resources.
5. Write `status.inventory = oldInventory + appliedRefs`.
6. Write `Ready=False` with `ApplyFailed`, `ApplyConflict`, or a more specific reason.
7. Requeue and let the next reconcile continue from durable state.

Key principle:

- Do not lose inventory for resources that were already created.
- Do not delete old resources until the new desired set has been fully applied.

### 8.4 Stale Deletion

After all desired resources apply successfully, the controller deletes resources that are in old inventory but not in the new desired set.

Deletion is based on inventory and ownership markers:

1. If the stale object is missing, treat it as already deleted.
2. If the object exists but no longer has matching ownership markers, do not delete it.
3. If deletion fails, keep inventory and write `Ready=False` / `DeleteFailed`.
4. If deletion succeeds, final successful status records only the desired inventory.

## 9. Switching Classes

When a namespace switches from class A to class B:

1. Namespace watch enqueues the namespace.
2. Reconciler reads the new class label.
3. Binding spec is updated to the new class.
4. Resources for class B are rendered and applied.
5. Only after class B applies successfully, stale resources from class A are deleted.
6. Binding inventory is updated to class B's desired set.

No separate "old class name" is required. Accurate inventory is enough to compute stale resources.

## 10. Updating Classes

When a `NamespaceClass` changes:

1. The class watch fires.
2. The controller lists bindings by `spec.className`.
3. Bound namespaces are enqueued.
4. Each namespace reconciles against the latest class generation.
5. New desired resources are created.
6. Removed desired resources are deleted as stale.
7. Binding status records `observedClassGeneration`.

Class updates never apply resources directly. They only fan out namespace reconcile requests.

## 11. Status and Observability

`NamespaceClassBinding.status` is the primary per-namespace observability surface.

It contains:

- `observedNamespaceUID`
- `observedClassGeneration`
- `inventory`
- `conditions`

Common condition reasons:

- `BindingRecorded`
- `ClassNotFound`
- `CleanupFailed`
- `GVKDenied`
- `ApplyConflict`
- `ApplyFailed`
- `DeleteFailed`
- `DuplicateResource`

Example failure:

```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      reason: ApplyConflict
      message: "resource already exists and is not owned by this NamespaceClass binding"
```

Suggested Kubernetes Events:

1. `NamespaceClassNotFound`
2. `ResourceApplyFailed`
3. `ResourceDeleteFailed`
4. `ResourceConflict`
5. `InventoryCorrupted`
6. `ReconcileSucceeded`

Suggested metrics:

1. reconcile latency,
2. workqueue depth,
3. apply error count by GVK and reason,
4. delete error count by GVK and reason,
5. ownership conflict count,
6. class fan-out count,
7. periodic drift repair count.

`NamespaceClass.status` is not required for the first version. A future version
could summarize the number of referencing namespaces and the number of
ready/degraded bindings for that class.

The first implementation focuses on durable binding status. Events, logs, and
metrics are useful future additions and should not replace durable conditions.

## 12. Security and Permissions

Supporting arbitrary resources is powerful but risky.

Main risk:

If a user can create or update `NamespaceClass`, they may be able to make the controller create high-impact cluster resources. For example, a `ClusterRoleBinding` could grant privileges.

This is the primary security risk. It exists because the product requirement is
to support arbitrary resources, including cluster-scoped resources. The
controller's service account becomes the execution identity for those templates.
If the controller has broad write permissions, a `NamespaceClass` author can
proxy those permissions through the controller.

Current controls:

1. `NamespaceClass` write access should be limited to cluster administrators.
2. Runtime GVK policy supports allowlist and denylist.
3. Denylist wins over allowlist.
4. `rbac.authorization.k8s.io/v1/ClusterRoleBinding` is denied by default.
5. Denied resources write `Ready=False` / `GVKDenied` and are not applied.
6. The controller refuses to adopt unmanaged existing resources.
7. Deletes require inventory plus ownership markers.

Current risk:

The demo Helm chart grants broad controller permissions so it can support arbitrary resources. Production deployments should narrow RBAC when possible, usually together with a restrictive GVK allowlist.

High-impact examples include:

1. `ClusterRoleBinding` privilege escalation,
2. cluster-scoped resource naming conflicts across namespaces,
3. accidental deletion of cluster-scoped resources when class or binding state is wrong,
4. applying resources blocked by namespace `ResourceQuota` or admission policies,
5. controller RBAC drift where the configured GVK policy allows a resource but
   the service account is not authorized to apply or delete it.

The `make rbac-check` harness target should treat missing required permissions
as failures and broad/high-risk permissions as warnings. Runtime apply/delete
authorization failures should be recorded as binding conditions and, in future
versions, metrics.

Admission webhook enforcement is planned but not installed yet.

## 13. Edge Cases

### 13.1 Very Large Namespace Counts

If a class is used by thousands or tens of thousands of namespaces, class
updates can fan out many reconcile requests.

Risks:

1. API server load from many get/apply/delete calls.
2. Workqueue backlog.
3. Large numbers of status updates.
4. High memory pressure if the controller scans and materializes too much state.
5. Long convergence time after a class update.

Mitigations:

1. Fan out by binding index instead of scanning all namespaces.
2. Keep reconcile idempotent.
3. Use rate limiting and controller-runtime workqueue behavior.
4. Keep inventory identity-only.
5. Configure `MaxConcurrentReconciles` conservatively. A reasonable first
   production range is 5 to 20, then tune with API server metrics.
6. Consider batching or throttling fan-out in future versions.
7. Track queue depth, reconcile latency, fan-out size, and error rates.

The design intentionally does not promise a fixed convergence SLA before load
testing, because the actual limit depends on namespace count, resource count per
class, API server capacity, admission latency, and RBAC/cache behavior.

### 13.2 Large NamespaceClass Resource Lists

A single class may contain many resources.

Risks:

1. Long reconcile duration.
2. Partial apply failure.
3. Large binding inventory approaching Kubernetes object size limits.

Mitigations:

1. Prepare all resources before apply.
2. Preserve partial apply inventory.
3. Store only resource identity in inventory.
4. Surface failure status clearly.
5. Add a configurable maximum resource count per class in a future webhook.
6. Ask administrators to split very large classes into smaller classes or a
   separate composition mechanism if they exceed operational limits.

### 13.3 Class Update Storms

Frequent updates to a class can enqueue many reconciles.

Risks:

1. The same namespace can be reconciled repeatedly while the class is changing.
2. Status updates can amplify API server load.
3. A broken class can produce a large volume of repeated failures.

Mitigations:

1. Watch predicates only react to generation changes.
2. Reconcile always reads current state instead of trusting event payloads.
3. Workqueue coalescing helps collapse repeated events.
4. Use exponential backoff on repeated failures.
5. Track fan-out count and backlog metrics.

### 13.4 Fast Class Switching

A namespace may switch classes multiple times quickly.

Mitigations:

1. Reconcile reads the latest namespace label.
2. Binding spec records the observed class.
3. Inventory diff is based on current desired set and previous inventory.
4. Apply-before-delete avoids deleting old resources before new resources are fully created.
5. Partial apply failure records successfully applied resources in inventory and
   skips stale deletion until all desired resources have applied.

### 13.5 Namespace Deletion

Namespaced resources are normally garbage-collected by namespace deletion. Cluster-scoped resources are not.

The controller adds a namespace finalizer once it has resolved a class and is about to create managed resources.

During namespace deletion:

1. Read binding inventory.
2. Filter to cluster-scoped inventory entries.
3. Delete those cluster-scoped resources if ownership markers match.
4. Delete the binding.
5. Remove the namespace finalizer.

Risk:

If cluster-scoped cleanup repeatedly fails, the namespace finalizer can keep the
namespace in `Terminating`. If a deployment policy disables cluster-scoped
managed resources entirely, the controller can avoid adding the namespace
finalizer and rely on Kubernetes namespace garbage collection for namespaced
resources.

### 13.6 NamespaceClass Deletion

If a namespace still references a deleted class, the default behavior is to treat the desired set as empty.

This deletes previously managed resources from binding inventory and removes the binding after cleanup.

Risk:

An accidental class deletion can trigger large-scale cleanup, especially with cluster-scoped resources.

Important guard:

Only an explicit API `NotFound` is treated as class deletion. Transient read errors, permission errors, cache errors, or discovery problems are returned as errors and do not trigger cleanup.

### 13.7 API Discovery Failure

The controller uses RESTMapper to determine whether each desired resource is namespaced or cluster-scoped.

If REST mapping fails:

1. The controller does not guess scope.
2. No resource is applied if prepare fails before apply.
3. Binding status records `Ready=False` / `ApplyFailed`.
4. Later reconciles can succeed after the CRD or discovery state is fixed.

### 13.8 Target Resource Already Exists

A desired resource may already exist before the controller tries to manage it.

Policy:

1. If it has matching ownership markers for the same namespace binding, reconcile
   may continue.
2. If it is unmanaged or owned by another namespace binding, reconcile must fail
   with `Ready=False` / `ApplyConflict`.
3. The controller should not auto-adopt unmanaged resources.

This prevents a `NamespaceClass` template from accidentally or intentionally
overwriting resources that were created by users or other controllers.

### 13.9 Inventory Loss or Damage

If binding inventory is deleted or corrupted, cleanup can become unsafe.

Current first version:

1. Binding deletion is repaired by requeueing the namespace when the binding watch observes deletion.
2. Damaged inventory that cannot be parsed causes delete failure status rather than unsafe deletion.
3. The controller does not delete resources it cannot confidently identify.

Future enhancement:

1. Rebuild best-effort inventory by listing resources with matching management
   labels when the GVK set is known.
2. Record `Ready=False` / `InventoryCorrupted` if rebuild is incomplete.
3. Continue to avoid unsafe deletion for resources whose identity or ownership
   cannot be proven.

### 13.10 Partial Apply Failure

Kubernetes has no transaction across multiple resources. The controller cannot
guarantee "all resources applied" or "nothing changed".

Required behavior:

1. Render, parse, validate, and resolve all desired resources before applying any
   resource.
2. Record each successfully applied resource in an `appliedRefs` set.
3. If apply fails midway, write inventory as the union of old inventory and
   `appliedRefs`.
4. Do not delete stale resources after a partial apply failure.
5. Record `Ready=False` / `ApplyFailed` and retry.
6. Delete stale resources only after all desired resources apply successfully.

This keeps partially created resources recoverable and cleanable during retries.

### 13.11 Server-Side Apply Field Conflict

If another field manager owns a field that the template wants to change,
server-side apply can return a conflict.

Default policy:

1. Do not use forced field ownership by default.
2. Record `Ready=False` / `ApplyFailed` or `ApplyConflict`.
3. Let the administrator resolve the conflict or change the template.

Alternative:

A future opt-in `forceConflicts: true` policy could force ownership. This is
high risk because it can overwrite other controllers' field ownership and should
not be the default.

### 13.12 RBAC Insufficient

The configured GVK policy may allow a resource while the controller service
account lacks the actual Kubernetes permission to create, patch, or delete it.

Behavior:

1. The controller receives a Kubernetes authorization error.
2. Binding status records `Ready=False` / `ApplyFailed` or `DeleteFailed`.
3. Reconcile retries with rate limiting.
4. Metrics should make authorization failures visible.

The harness should include `make rbac-check` so deployment manifests can be
checked against the intended controller capabilities.

### 13.13 ResourceQuota or Admission Rejection

Apply may fail because a namespace quota, policy webhook, or built-in admission
plugin rejects the object.

Behavior:

1. The controller records failure status and relies on retry.
2. It does not bypass Kubernetes admission.
3. It should avoid stale deletion if desired resource apply has not completed.

### 13.14 Cluster-Scoped Resource Naming Conflict

Two namespaces can render the same cluster-scoped resource name.

Mitigations:

1. Templates should usually include `{{ .Namespace.Name }}` or another stable
   namespace-derived value in cluster-scoped names.
2. The controller refuses to manage a cluster-scoped resource with ownership
   markers that belong to another namespace binding.
3. GVK policy can deny cluster-scoped resource kinds that are too risky for the
   deployment.

### 13.15 Multi-Replica Controllers

Production controllers may run multiple replicas.

Leader election should be enabled for active/passive operation. Apply and delete
operations should still remain idempotent, because retries and repeated events
are normal.

If active/active behavior is introduced later, binding status updates must handle
resourceVersion conflicts by refetching and retrying instead of overwriting
another reconciler's status.

## 14. Design Tradeoffs

### 14.1 Raw Object API vs Typed API

The design uses raw Kubernetes objects in `spec.resources`.

Benefits:

1. Supports arbitrary resources as required.
2. Does not require bespoke API fields per resource type.
3. Can support newly installed CRDs.

Costs:

1. Weak CRD schema validation.
2. More errors appear during reconciliation.
3. Users must understand Kubernetes object schemas.

### 14.2 NamespaceClassBinding vs Namespace-Scoped ConfigMap Inventory

The design stores inventory in a cluster-scoped `NamespaceClassBinding`.

Benefits:

1. Inventory survives namespace deletion long enough to clean cluster-scoped resources.
2. Per-namespace status is queryable from one stable object.
3. Observed generation and inventory are kept together.

Costs:

1. Adds another CRD.
2. Requires binding lifecycle management.
3. Inventory damage still needs careful handling.
4. Namespace finalizers can make failed cluster-scoped cleanup block namespace
   deletion until the failure is resolved.

### 14.3 Server-Side Apply vs Create/Update

Server-side apply is preferred because it is closer to declarative controller behavior and surfaces field conflicts.

Cost:

The controller must handle field ownership conflicts and avoid force ownership by default.

### 14.4 Conservative Ownership vs Auto-Adoption

The controller does not adopt existing resources that lack matching ownership markers.

Benefits:

1. Avoids overwriting user-managed resources.
2. Keeps ownership boundaries explainable.
3. Makes conflict recovery clear.

Cost:

Administrators may need to rename templates or manually migrate existing resources.

### 14.5 Supporting Arbitrary Resources vs Security Risk

The problem requires arbitrary resources, including cluster-scoped resources.

This increases security risk. The design accepts the product requirement but makes the risk explicit and mitigates it through:

1. admin-only `NamespaceClass` writes,
2. runtime GVK policy,
3. default `ClusterRoleBinding` deny,
4. ownership markers,
5. inventory-based deletion,
6. future admission webhook validation.

The safest production posture is to combine all three controls:

1. narrow who can write `NamespaceClass`,
2. configure a deployment-specific GVK allowlist,
3. grant the controller only the RBAC verbs required by that allowlist.

### 14.6 Dynamic Informers vs Primary Object Events

Dynamic informers could repair managed-resource drift faster, but they significantly increase complexity for arbitrary GVKs.

The first version chooses:

1. `Namespace` watch,
2. `NamespaceClass` fan-out,
3. `NamespaceClassBinding` watch,
4. periodic requeue.

Dynamic managed-resource watches remain a future enhancement.

## 15. Test Plan

Core tests:

1. A labeled namespace creates a binding and desired resources.
2. Managed resources are recorded in binding inventory.
3. Class updates fan out to existing namespaces.
4. Class switching creates new resources and deletes stale resources.
5. Label removal cleans old resources and deletes the binding.
6. Class deletion cleans old resources only when the class read returns `NotFound`.
7. Non-`NotFound` class read errors do not clean resources.
8. Cluster-scoped resources are recorded and cleaned during namespace deletion.
9. GVK policy denies high-risk resources.
10. Existing unmanaged resources produce `ApplyConflict`.
11. Server-side apply field conflicts are not force-owned.
12. Partial apply records successful inventory and skips stale deletion.
13. Unknown GVK and RESTMapper failures produce stable status.
14. Template variables render correctly and unknown variables fail before apply.
15. Binding deletion is repaired by requeueing the namespace.
16. Periodic requeue repairs deleted managed resources.
17. Reserved controller labels and annotations cannot be abused to bypass
    ownership checks.
18. Namespaced templates with an explicit foreign namespace are normalized by
    the current runtime path and should be rejected once the webhook is enabled.
19. Inventory damage produces failure status rather than unsafe deletion.
20. Delete failures preserve stale inventory and report `DeleteFailed`.

Admission webhook tests to add when the webhook is implemented:

1. Reject missing `apiVersion`, `kind`, or `metadata.name`.
2. Reject duplicate desired resource identity.
3. Reject controller-reserved labels and annotations.
4. Reject unsupported template variables.
5. Reject denied GVKs.
6. Reject namespaced resources that explicitly target another namespace.

Scale tests to consider:

1. A class referenced by many namespaces, for example 10,000 namespaces in a
   non-local performance environment.
2. A class with many resource templates, including enough entries to approach
   inventory object-size concerns.
3. Rapid class updates.
4. High-frequency class switching.
5. Workqueue de-duplication and backlog behavior under repeated class updates.
6. Reconcile throughput with different `MaxConcurrentReconciles` values.

## 16. Success Criteria

The design is successful when:

1. Namespace creation with a class label creates the desired resources.
2. Namespace class switching is eventually consistent and safe.
3. NamespaceClass updates reconcile all bound namespaces.
4. The controller does not overwrite resources it does not own.
5. Partial failure remains recoverable and cleanable.
6. Administrators can explain why each managed resource exists, which class created it, and which namespace it belongs to.
7. High-risk arbitrary-resource behavior is bounded by RBAC, GVK policy, ownership markers, and explicit status.
8. Large-scale and error-path behavior is idempotent, retryable, and observable.

## 17. Summary

This design treats `NamespaceClass` as a namespace-level resource template set and relies on controller reconciliation, durable inventory, server-side apply, and ownership boundaries for correctness.

The first version keeps the API small:

```text
Namespace label -> NamespaceClass -> raw resource templates
Inventory -> cluster-scoped NamespaceClassBinding.status.inventory
Apply -> server-side apply
Delete -> inventory diff plus ownership checks
```

It satisfies the core problem requirements while leaving clear future paths:

- admission webhook validation for stronger safety,
- dynamic informer watches for faster drift repair,
- narrower production RBAC with deployment-specific GVK allowlists,
- metrics and events for operational visibility,
- inventory rebuild for damaged binding state.
