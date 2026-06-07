# Label Removal Cleanup

## Goal

When a namespace no longer has the `namespaceclass.akuity.io/name` label, the controller treats that namespace as no longer managed by `NamespaceClass`, deletes resources recorded in the existing binding inventory, and removes the `NamespaceClassBinding` after cleanup succeeds.

## Scope

- Add envtest coverage for removing the namespace class label.
- Delete previously managed resources using `NamespaceClassBinding.status.inventory`.
- Delete the cluster-scoped `NamespaceClassBinding` after successful cleanup.
- Upgrade smoke to verify label removal cleanup in minikube.

## Out Of Scope

- Namespace deletion finalizers.
- `NamespaceClass` deletion fan-out.
- Cluster-scoped resource finalizer behavior.
- Rebuilding a damaged or missing inventory.

## Verification

- `make envtest`
- `make check`
- `make deploy-local`
- `make smoke`
