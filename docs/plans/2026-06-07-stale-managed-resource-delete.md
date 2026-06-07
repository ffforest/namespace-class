# Stale Managed Resource Delete

## Goal

When a namespace reconcile observes that a resource recorded in `NamespaceClassBinding.status.inventory` is no longer present in the current desired resource set, the controller deletes that stale managed resource.

## Scope

- Compare previous binding inventory with the desired refs produced by the current `NamespaceClass`.
- Delete only resources that are still marked as managed by the same namespace UID.
- Keep the binding inventory aligned with the desired refs after cleanup.
- Add envtest coverage for class resource updates that remove one managed resource and add another.
- Upgrade smoke to verify stale resource deletion in the deployed controller path.

## Out Of Scope

- Watching `NamespaceClass` updates and fan-out enqueueing all affected namespaces.
- Cleanup after a namespace label is removed.
- Finalizer-based namespace deletion cleanup.
- Cluster-scoped resource finalizer semantics.

## Verification

- `make envtest`
- `make check`
- `make deploy-local`
- `make smoke`
