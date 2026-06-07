# Progress

## 2026-06-06 Repository Bootstrap

- Created initial repository skeleton for the NamespaceClass controller solution.
- Added project-level docs, Makefile verification entry points, local tool installer, CRD manifests, sample resources, Helm chart skeleton, and minimal Go packages for inventory identity and template rendering.
- Current implementation is intentionally skeletal; next work should proceed through a focused implementation plan.

## 2026-06-07 Envtest Harness

- Added project-local `setup-envtest` integration through `make envtest-tools`.
- Added `make envtest`, which runs build-tagged envtest integration tests with local `KUBEBUILDER_ASSETS`.
- Added an envtest-backed CRD/status test that starts a real API server and etcd, installs the project CRDs, creates `NamespaceClass`, `Namespace`, and `NamespaceClassBinding`, then verifies the binding status subresource can persist inventory.
- `make check` now includes envtest, so the aggregate local harness covers ordinary unit tests, envtest integration tests, vet, manifest checks, and Helm rendering.

## 2026-06-07 Binding Reconciler

- Added typed Go API objects for `NamespaceClass` and `NamespaceClassBinding`, including manual deepcopy methods and scheme registration.
- Added a namespace reconciler that watches `Namespace`, reads the `namespaceclass.akuity.io/name` label, and records the binding in cluster-scoped `NamespaceClassBinding`.
- Binding status now records `observedNamespaceUID`, `observedClassGeneration`, and a Ready condition for the binding-recorded path.
- Added envtest coverage that starts the manager and verifies a labeled namespace produces a Ready binding.
- Extended minikube smoke to verify binding creation when the controller is deployed.
- Changed local deploys to use unique timestamped image tags because reusing `namespace-class-controller:dev` can leave minikube running an older same-tag image.
