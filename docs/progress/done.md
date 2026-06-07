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
- Added a no-op controller-runtime manager entrypoint with health and readiness probes.
- Added a local image and deployment loop: `Dockerfile`, `make image-build`, `make image-load`, `make deploy-local`, `make wait-crds`, `make wait-controller`, and `make undeploy-local`.
- Upgraded cluster smoke checks to wait for CRD establishment and verify controller Deployment availability when the controller is installed.
