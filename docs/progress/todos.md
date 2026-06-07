# TODO

## Open

- [ ] Implement admission webhook deployment, TLS certificate generation/rotation, and caBundle injection after the runtime GVK guard.
- [ ] Replace Helm chart placeholder image defaults with the final image build/publish flow.

## Harness / Design Review Backlog

### High

No open high-priority harness/design items.

### Medium

- [ ] Pin the `setup-envtest` install version in `Makefile` instead of using `@latest`, so envtest behavior remains reproducible.
- [ ] Add dependency drift targets such as `make mod-tidy` and `make mod-check`, and include the check in the aggregate verification path.
- [ ] Add `make scripts-check` for shell and Ruby syntax checks, then include it in `make check`.
- [ ] Tighten template variable scope. Recommended first slice: support only `.Namespace.Name`, `.Namespace.UID`, and `.Class.Name`; defer labels/annotations until escaping and key lookup syntax are defined.

### Low

- [ ] Rename or clarify `manifests-check`; it is currently an offline YAML/shape lint, while server-side Kubernetes validation is covered by smoke.
- [ ] Document `make envtest-tools` in `README.md` so users can explicitly prefetch envtest assets before running `make envtest`.
