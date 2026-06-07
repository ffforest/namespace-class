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
- Added handwritten Go API types and scheme registration for `NamespaceClass` and `NamespaceClassBinding`.
- Added namespace reconciliation for the first business slice: labeled namespaces now create or update cluster-scoped `NamespaceClassBinding` objects with basic Ready status.
- Upgraded smoke to create a temporary `NamespaceClass` and labeled `Namespace`, wait for `NamespaceClassBinding` Ready, verify binding fields, and clean up.
- Changed `make deploy-local` to use a unique timestamped local image tag so minikube does not keep running an older same-tag image.
- Added dynamic server-side apply for rendered `NamespaceClass.spec.resources`, starting with namespaced resources such as `ServiceAccount`.
- Added managed-resource inventory updates in `NamespaceClassBinding.status.inventory` for successfully applied resources.
- Upgraded envtest and smoke to verify a managed `ServiceAccount` is created and recorded in binding inventory.
- Added stale managed resource deletion: resources present in previous binding inventory but absent from the current desired set are deleted only when ownership markers match the namespace UID.
- Upgraded envtest and smoke to verify a `NamespaceClass` resource update creates the new managed `ServiceAccount`, deletes the stale one, and updates binding inventory.
- Added `NamespaceClass` create/update watch fan-out using a `NamespaceClassBinding.spec.className` cache index, so class resource updates automatically enqueue bound namespaces.
- Upgraded envtest and smoke to verify `NamespaceClass` updates are applied without manually modifying the namespace.
