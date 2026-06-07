# Managed Resource Apply Plan

## Goal

Add the first managed-resource reconciliation slice:

1. Read raw resources from `NamespaceClass.spec.resources`.
2. Convert each raw object into `unstructured.Unstructured`.
3. Resolve whether each GVK is namespaced or cluster-scoped.
4. For namespaced resources, force the target namespace.
5. Apply resources with server-side apply.
6. Record successfully applied resources in `NamespaceClassBinding.status.inventory`.

## Scope

This slice verifies a simple namespaced resource, `ServiceAccount`.

It does not implement stale resource deletion, class switching cleanup, class update fan-out, cluster-scoped cleanup finalizers, admission webhooks, or GVK allowlist/denylist enforcement.

## Verification

1. Add an envtest that creates a `NamespaceClass` containing a `ServiceAccount`, then creates a labeled `Namespace`.
2. Confirm the test fails before implementation because the `ServiceAccount` and inventory entry are missing.
3. Implement the minimal dynamic apply path.
4. Run `make envtest`, `make check`, `make deploy-local`, and `make smoke`.
