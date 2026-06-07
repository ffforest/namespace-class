# Controller Gap Closure

## Goal

Close the remaining controller correctness gaps around failure status, server-side apply ownership, partial apply durability, template rendering, drift repair, and negative coverage.

## Scope

- Record stable `NamespaceClassBinding` conditions for unmanaged existing resource conflicts, SSA conflicts, ordinary apply failures, duplicate desired identities, and delete failures.
- Remove default server-side apply force ownership. Any future force mode must be explicit and documented as high risk.
- Preserve inventory for resources that were successfully applied before a later apply failure.
- Render the small supported template variable set inside raw resources before preparing managed resources.
- Add first-slice drift repair through `NamespaceClassBinding` watch and periodic namespace requeue.
- Add negative envtest coverage for unmanaged conflicts, SSA conflicts, partial apply, unknown GVK, duplicate identity, template rendering, binding deletion drift, and managed resource deletion drift.

## Out Of Scope

- Admission webhook deployment and TLS/cert management.
- Dynamic informers for arbitrary managed GVKs.
- Full inventory rebuild when the binding is missing and no primary object event occurs.
- Production metrics.

## Verification

- Targeted envtests for each new behavior.
- `make envtest`
- `make check`
- `make deploy-local`
