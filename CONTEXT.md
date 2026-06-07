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
- `NamespaceClassBinding.status.inventory` is the source of truth for managed resource cleanup.
- Managed resource identity is `apiVersion + kind + namespace + name`; name alone is never enough.
- Namespace reconciliation is the only path that mutates managed resources. `NamespaceClass` events enqueue affected namespaces instead of applying resources directly.
- `NamespaceClass` create/update/delete fan-out uses the controller cache index on `NamespaceClassBinding.spec.className`, then enqueues each bound namespace from `binding.spec.namespaceName`.
- Managed resources are created and updated with server-side apply using a fixed field manager.
- Existing resources without controller ownership markers are not adopted or overwritten.
- Removing a namespace class label makes the desired set empty for that namespace; cleanup is driven by binding inventory.
- Deleting a `NamespaceClass` makes the desired set empty for namespaces that still reference it; default behavior is cleanup based on binding inventory.
- The controller adds namespace finalizer `namespaceclass.akuity.io/finalizer` before creating managed resources for a resolved class. During namespace deletion it uses binding inventory to delete cluster-scoped managed resources, deletes the binding, then removes the finalizer. Namespaced managed resources are left to Kubernetes namespace garbage collection during namespace deletion.
- Runtime GVK policy defaults to allow-all plus denylist, with `rbac.authorization.k8s.io/v1/ClusterRoleBinding` denied by default. Denylist wins over allowlist; when allowlist is non-empty, unmanaged GVKs outside the allowlist are denied. Admission webhook enforcement is planned but not installed yet.
- Template support is intentionally small: string substitution only, with no loops, conditionals, functions, external lookups, or cross-resource references.
- Safety risks from arbitrary resources are handled with RBAC boundaries, ownership markers, GVK policy, explicit status, and planned admission validation.
- Server-side apply does not use force ownership by default. Existing unmanaged resources and field-manager conflicts are reported on `NamespaceClassBinding` instead of being overwritten.
- First implementation does not dynamically watch every managed GVK. Drift repair uses `Namespace`, `NamespaceClass`, and `NamespaceClassBinding` watches plus a simple periodic requeue; dynamic informers for arbitrary managed resources are deferred.
- `NamespaceClass` missing/deleted cleanup treats the desired set as empty; cleanup success deletes the binding, while cleanup failure keeps the binding with a `CleanupFailed` condition for retry.
- The repository has a working envtest harness. `make envtest` starts a real API server and etcd, installs project CRDs, and validates CRD/status behavior.
- `make check` is the aggregate local verification entry point and includes unit tests, envtest, vet, manifest checks, and Helm rendering.
- `make rbac-check` is a live-cluster harness target for the deployed controller ServiceAccount. It is intentionally not part of `make check` because it requires cluster access and an installed release.
- Current CRD manifests are handwritten YAML. Go API types and controller-gen are not wired in yet.

## Open Questions

- Admission webhook deployment, TLS certificate generation/rotation, and caBundle injection.
- Final production image name and registry.
- Whether and when to introduce controller-gen for CRD, RBAC, and generated deepcopy code.

## Original Problem Statement

# Namespace Class

> [!IMPORTANT]
> You’re welcome to use resources as you normally would, but please don’t rely on AI tools to do the work. We’ll be discussing your approach and design decisions in depth in the interview, so make sure you fully understand and can explain your technical decisions and the code.

## Description

Kubernetes admins wish to define a set of namespace "classes". A NamespaceClass defines a set of
complimentary resources, policies, etc... which are additionally created and managed
when a namespace is created from a certain class.

To solve this, we will introduce a new `NamespaceClass` CRD and controller to automate the maintenance
of these resources.

## Use Case

A Kubernetes operator should be able to apply a new kind of Kubernetes resource called NamespaceClass,
implemented as CRD (custom resource definition). For example, the admin might define two classes
of namespaces relating to network access:

* `public-network` - public-network namespaces would additionally contain policies allowing the namespace to be reachable over public internet
* `internal-network`- internal-network namespaces would additionally create policies restricting network access to only corporate VPN

```yaml
apiVersion: v1
kind: NamespaceClass
metadata:
  name: public-network
spec:
  # TODO: NetworkPolicies allowing ingress from the world
```

```yaml
apiVersion: v1
kind: NamespaceClass
metadata:
  name: internal-network
spec:
  # TODO: NetworkPolicies allowing egress/ingress to specific VPN IP address
```

To use a `NamespaceClass`, a namespace will have a label indicating which NamespaceClass it would
derive from. For example, the admin could create a `web-portal` namespace which allows public
egress into the namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: web-portal
  labels:
    namespaceclass.akuity.io/name: public-network
```

When the `web-portal` namespace is created, the controller would create the associated resources
defined in the `public-network` NamespaceClass.

NOTE: NamespaceClass should allow creating any kind of resources (not only `NetworkPolicy`/`ServiceAccount`).

## Requirements

NOTE: this problem is intended to have minimal requirements in order to allow freedom in the
design and architectural decisions. It only has the following requirements:

### NamespaceClass CRD

Design a new CustomResourceDefinition, `NamespaceClass`, which allows an operator the flexibility to
define the additional Kubernetes resources which should be created when a namespace of that class
is created (e.g. NetworkPolicies, ServiceAccounts, PodSecurityPolicies etc...). There are no
requirements on what the API spec should look like.

### NamespaceClass controller

Create a kubernetes controller, which monitors Namespaces and creates the additional resources
as defined by the class annotated in the namespace.

### Switching classes

A `Namespace` may be modified to switch to a different class. When this happens the controller should
handle the creation and deletion of the associated resources between the old and the new class

### Updating classes

A `NamespaceClass` may be modified to have a different set of resources. When this happens, existing
Namepaces of that class should then be updated to create or delete resources according to the
updated class.
