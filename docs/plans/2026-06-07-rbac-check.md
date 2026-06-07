# RBAC Check Harness

## Goal

Make the controller ServiceAccount RBAC posture visible and repeatable in the local harness.

## Scope

- Add `make rbac-check`.
- Check required controller permissions for namespaces, NamespaceClass, NamespaceClassBinding, representative namespaced managed resources, and representative cluster-scoped managed resources.
- Report broad/high-risk permissions such as `*/*` and `ClusterRoleBinding` creation as warnings, not failures.
- Keep the target out of `make check` because it depends on a live cluster and deployed controller ServiceAccount.

## Out Of Scope

- Narrowing the Helm chart RBAC.
- Generating RBAC dynamically from GVK policy.
- Admission webhook certificate or deployment work.

## Verification

- `bash -n scripts/rbac-check.sh`
- `make rbac-check`
- `make docs-check`
