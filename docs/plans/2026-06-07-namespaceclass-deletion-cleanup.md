# NamespaceClass Deletion Cleanup

## Goal

When a namespace still references a `NamespaceClass` that no longer exists, the controller treats the desired set as empty, deletes previously managed resources from binding inventory, and removes the `NamespaceClassBinding` after cleanup succeeds.

## Scope

- Watch `NamespaceClass` delete events and fan out to currently bound namespaces.
- Reconcile a namespace whose class is missing by cleaning resources from binding inventory.
- Delete `NamespaceClassBinding` after successful cleanup.
- Keep the binding around on cleanup failure so the next reconcile can retry.
- Add envtest and smoke coverage for deleting a class while a namespace still references it.

## Out Of Scope

- Namespace deletion finalizers.
- Cluster-scoped resource finalizer behavior.
- Rebuilding a damaged or missing inventory.
- Recreating resources automatically if a deleted class is later recreated with the same name.

## Verification

- `make envtest`
- `make check`
- `make deploy-local`
- `make smoke`
