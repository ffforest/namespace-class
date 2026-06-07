# NamespaceClass Watch Fan-Out

## Goal

When a `NamespaceClass` changes, every namespace currently bound to that class is reconciled automatically. Existing namespaces should not require a manual namespace update or periodic resync to pick up class resource changes.

## Scope

- Watch `NamespaceClass` objects in the existing namespace reconciler.
- Use `NamespaceClassBinding.spec.className` as the fan-out source.
- Add a cache field index for bindings by class name.
- Enqueue one namespace reconcile request per matching binding.
- Update envtest and smoke so class updates are verified without annotating the namespace.

## Out Of Scope

- Dynamic informer watches for arbitrary managed child resources.
- Periodic reconciliation or global resync policy.
- Cleanup after class deletion or namespace label removal.
- Label-selector fan-out for namespaces that do not have a binding yet.

## Verification

- `make envtest`
- `make check`
- `make deploy-local`
- `make smoke`
