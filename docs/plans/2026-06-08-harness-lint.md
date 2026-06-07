# Harness and Lint Cleanup

## Scope

- Close the remaining `Harness / Design Review Backlog` items.
- Add a project-local `golangci-lint` flow and run it once.
- Do not change the admission webhook backlog item.

## Steps

1. Update `Makefile` with pinned envtest tool installation, dependency drift checks, script syntax checks, clearer manifest lint naming, and golangci-lint targets.
2. Add a minimal golangci-lint v2 config that expands beyond `go vet` without changing controller behavior.
3. Document the new harness commands in `README.md`.
4. Move completed backlog items from `docs/progress/todos.md` to `docs/progress/done.md`.

## Verification

- `make mod-check`
- `make scripts-check`
- `make manifests-lint`
- `make lint`
- `make check`
