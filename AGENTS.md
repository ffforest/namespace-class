# Project Notes

## Document Index

- Project goals and quick entry point: `README.md`
- Stable domain language and boundaries: `CONTEXT.md`
- Current design: `docs/design/namespaceclass-design.md`
- Chinese design reference: `docs/design/namespaceclass-design-zh.md`
- Implementation plans: `docs/plans/`
- ADRs: `docs/adr/`
- Progress, TODOs, completion log, and learnings: `docs/progress/progress.md`, `docs/progress/todos.md`, `docs/progress/done.md`, `docs/progress/learnings.md`

## Workflow

- Before adding a feature, check whether it affects API shape, controller reconciliation, inventory, RBAC, templates, Helm manifests, or the verification harness described in `docs/design/namespaceclass-design.md`. If it does, update the design or write an ADR first.
- Before substantial implementation work, write an implementation plan in `docs/plans/{date}-{topic}.md` with explicit acceptance commands.
- Code changes follow TDD by default: write a failing unit test, envtest, or cluster smoke check first; implement the smallest code that passes; then refactor and run the smallest sufficient `make` target.
- Pure documentation, comments, or mechanical formatting changes do not require tests. Changes that affect controller behavior, CRD/schema, status, inventory, RBAC, template rendering, or Helm manifests must have matching unit, envtest, or smoke coverage first.
- Do not bypass Makefile conventions with temporary verification scripts. If a new script is truly needed, put it under `scripts/` and wire it into the Makefile.
- Do not commit local minikube state, kubeconfig, downloaded tools, build artifacts, or logs.

## Verification Rules

- Documentation-only changes: run `make docs-check`.
- Go code changes: run at least `make test` and `make vet`. If the change touches Kubernetes API behavior, CRDs, status, or reconciler behavior, also run `make envtest`.
- Manifest, CRD, or Helm chart changes: run `make manifests-check` and `make helm-template`.
- Controller behavior or live-cluster interaction changes: run `make cluster-check`, `make deploy-local`, and `make smoke` against local minikube.
- Prefer `make check` before finishing. It includes formatting checks, module drift checks, lint, unit tests, envtest, vet, script checks, manifest lint, and Helm rendering. If local Go, kubectl, helm, envtest assets, or minikube are unavailable, state the missing prerequisite and the substitute verification that was completed.

## Status Sync

- Add new TODOs to `docs/progress/todos.md`.
- Move completed TODOs to `docs/progress/done.md` with the date and a short note.
- Record important design tradeoffs in `docs/adr/`.
- Record reusable debugging lessons in `docs/progress/learnings.md`.
