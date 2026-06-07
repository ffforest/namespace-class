# Cluster-Scoped Resource Finalizer

## Goal

Support cluster-scoped managed resources through namespace deletion by using a namespace finalizer to clean cluster-scoped inventory before the namespace disappears.

## First Slice

- A `NamespaceClass` may define a cluster-scoped resource such as `ClusterRole`.
- The controller records cluster-scoped resources in `NamespaceClassBinding.status.inventory` with an empty `namespace` field.
- Before creating managed resources for an existing class, the controller adds a namespace finalizer.
- When the namespace is deleting, the controller deletes cluster-scoped resources recorded in the binding inventory, deletes the binding, then removes the finalizer.
- Namespaced resources are not force-deleted during namespace deletion; Kubernetes namespace garbage collection owns that path.

## Design Choice

The controller adds the finalizer only after it can resolve the referenced `NamespaceClass`. This avoids a loop where a namespace points at a missing class, cleanup succeeds, the finalizer is removed, and the next namespace event immediately re-adds it without any managed resources to protect.

During namespace deletion, cleanup is intentionally limited to inventory entries whose `namespace` is empty. This keeps the finalizer focused on resources Kubernetes cannot clean automatically and reduces the risk of a terminating namespace being stuck because a namespaced child object cannot be read or deleted.

## Out Of Scope

- Admission allowlist or denylist for high-risk cluster-scoped GVKs.
- Rebuilding missing inventory.
- Recovering cluster-scoped resources created by older controller versions before a finalizer existed.
- Conflict resolution when multiple namespaces attempt to manage the same cluster-scoped resource name.

## TDD Verification

1. Add an envtest that creates a namespace class with a `ClusterRole`, creates a labeled namespace, waits for the `ClusterRole`, binding inventory, and namespace finalizer, deletes the namespace, then expects the `ClusterRole` and binding to be deleted.
2. Run that envtest and confirm it fails before implementation.
3. Implement the finalizer lifecycle and namespace-deletion cleanup.
4. Run the envtest, then `make check`, then `make deploy-local`.
