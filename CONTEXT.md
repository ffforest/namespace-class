# NamespaceClass Context

## Domain Language

- `NamespaceClass`: cluster-scoped CRD that declares raw Kubernetes resource templates.
- `NamespaceClassBinding`: cluster-scoped controller-owned CRD that records one namespace's class binding, observed generations, conditions, and managed resource inventory.
- Managed resource: any Kubernetes object created or updated by the controller from a `NamespaceClass` template.
- Inventory: durable list of managed resource identities, keyed by `apiVersion`, `kind`, `namespace`, and `name`.
- Desired set: resources rendered from the current namespace and its referenced `NamespaceClass`.
- Stale resource: resource recorded in inventory but absent from the current desired set.

## Stable Decisions

- Namespace binding is expressed through label `namespaceclass.akuity.io/name`.
- `NamespaceClass` supports both namespaced and cluster-scoped resources.
- Safety risks from arbitrary resources are handled with RBAC boundaries, ownership markers, GVK policy, admission validation, and explicit status.
- First implementation does not dynamically watch every managed GVK. Drift is repaired through primary-object watches and periodic resync.

## Open Questions

- Exact GVK allowlist/denylist defaults.
- Whether to add a validating admission webhook in the first implementation slice or after the basic controller loop.
- Final production image name and registry.
- Whether to install Go system-wide or rely on a user-provided Go toolchain.

