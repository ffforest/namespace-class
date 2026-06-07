# Done

## 2026-06-06

- Initialized repository skeleton and local verification harness.

## 2026-06-07

- Installed Go system-wide with Homebrew and verified `make doctor`.
- Verified local minikube cluster connectivity with `make cluster-check`.
- Installed `NamespaceClass` and `NamespaceClassBinding` CRDs into minikube with `make deploy-crds`.
- Verified cluster smoke path with `make smoke`.
- Added envtest harness with local `setup-envtest` assets, an `envtest` build-tagged CRD/status integration test, and `make envtest`.
- Updated `make check` to include envtest-backed integration coverage.
