# TODO

## Open

- [ ] Implement `NamespaceClass` and `NamespaceClassBinding` Go API types.
- [ ] Implement namespace reconciliation with dynamic client apply/delete.
- [ ] Implement binding status inventory updates.
- [ ] Expand envtest coverage for reconciler behavior: class creation, class switching, class update, and deletion cleanup.
- [ ] Decide first-slice admission webhook scope.
- [ ] Replace Helm chart placeholder image defaults with the final image build/publish flow.

## Harness / Design Review Backlog

### High

- [ ] Upgrade `make smoke` from CRD/sample dry-run to behavior-level e2e smoke: create `NamespaceClass`, create a labeled `Namespace`, wait for `NamespaceClassBinding`, verify managed resources, switch class, verify old resources are removed and new resources are created, then clean up.
- [ ] Decide and document namespace finalizer behavior for cluster-scoped managed resources. Recommended decision: when cluster-scoped resources are supported, the controller must add a namespace finalizer so cluster-scoped cleanup can complete before namespace deletion finishes.
- [ ] Clarify `NamespaceClass` missing/deleted binding lifecycle. Recommended decision: cleanup succeeds -> delete binding; cleanup fails -> keep binding with a condition such as `ClassNotFound` or `CleanupFailed`.

### Medium

- [ ] Pin the `setup-envtest` install version in `Makefile` instead of using `@latest`, so envtest behavior remains reproducible.
- [ ] Add dependency drift targets such as `make mod-tidy` and `make mod-check`, and include the check in the aggregate verification path.
- [ ] Add `make scripts-check` for shell and Ruby syntax checks, then include it in `make check`.
- [ ] Add RBAC feedback with a target such as `make rbac-check`, using `kubectl auth can-i` for the controller service account and key resource operations.
- [ ] Decide first-slice GVK policy semantics. Recommended first slice: runtime policy defaults to allow all, mark it high risk in docs, and defer enforceable allowlist/denylist admission to a later slice.
- [ ] Tighten template variable scope. Recommended first slice: support only `.Namespace.Name`, `.Namespace.UID`, and `.Class.Name`; defer labels/annotations until escaping and key lookup syntax are defined.

### Low

- [ ] Rename or clarify `manifests-check`; it is currently an offline YAML/shape lint, while server-side Kubernetes validation is covered by smoke.
- [ ] Document `make envtest-tools` in `README.md` so users can explicitly prefetch envtest assets before running `make envtest`.
