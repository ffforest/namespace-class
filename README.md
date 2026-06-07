# Namespace Class

This repository hosts a Kubernetes controller solution for the NamespaceClass problem.

The controller will let cluster admins define a cluster-scoped `NamespaceClass` whose raw Kubernetes resource templates are reconciled for namespaces labeled with that class.

## Scope

The intended solution covers:

- `NamespaceClass` CRD for raw resource templates.
- Cluster-scoped `NamespaceClassBinding` CRD for durable inventory and per-namespace status.
- A controller that watches `Namespace`, `NamespaceClass`, and `NamespaceClassBinding`.
- Server-side apply for managed resources.
- Support for both namespaced and cluster-scoped resources.
- Local verification through Go tests, manifest validation, Helm rendering, and minikube smoke checks.

## Quick Start

```bash
make tools
export PATH="$PWD/.tools/bin:$PATH"
make doctor
make check
```

If minikube is running:

```bash
make cluster-check
make deploy-crds
make smoke
```

## Common Commands

```bash
make help              # list commands
make tools             # install project-local kubectl and helm
make doctor            # check local prerequisites
make fmt               # check Go formatting
make test              # run Go unit tests
make envtest           # run envtest-backed integration tests
make vet               # run go vet
make manifests-check   # validate CRD manifests client-side
make helm-template     # render Helm chart
make check             # local aggregate verification
make cluster-check     # verify kubectl can reach minikube/current cluster
make deploy-crds       # install CRDs into the current cluster
make image-build       # build namespace-class-controller:dev locally
make image-load        # load namespace-class-controller:dev into minikube
make deploy-local      # build, load, install, restart, wait, and smoke-test locally
make smoke             # run cluster smoke checks
make rbac-check        # inspect deployed controller ServiceAccount RBAC
```

`make deploy-local` uses a unique local image tag derived from `IMAGE_TAG` and the current timestamp so minikube does not reuse an older same-tag image.

## Current State

This is an initial repository skeleton with a working verification harness and the design document. The controller implementation is intentionally minimal and will be built in small slices.

## Project Notes

- Design lives in `docs/design/namespaceclass-design.md`.
- Implementation plans live in `docs/plans/`.
- Progress tracking lives in `docs/progress/`.
- Tools installed by `make tools` are local to `.tools/bin` and are ignored by git.
