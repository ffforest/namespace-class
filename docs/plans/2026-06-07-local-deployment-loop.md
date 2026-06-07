# Local Deployment Loop Plan

## Goal

Build the first deployable controller slice:

1. Controller process starts as a no-op controller-runtime manager.
2. Manager exposes health and readiness probes.
3. Local image can be built without a remote registry.
4. Image can be loaded into minikube.
5. Helm can install or upgrade the controller Deployment.
6. CRD and controller readiness have explicit wait targets.
7. Smoke checks verify CRDs and, when deployed, controller readiness.

## Scope

This slice does not implement NamespaceClass reconciliation behavior.

## Implementation Steps

1. Add envtest coverage for a manager that starts and serves `/healthz` and `/readyz`.
   Verify with `make envtest`.
2. Replace the skeleton `main` with a no-op controller-runtime manager.
   Verify with `make build`, `make test`, and `make vet`.
3. Add `Dockerfile`, image build/load targets, and local deploy targets.
   Verify with `make image-build` and `make image-load`.
4. Add Helm probes and explicit `wait-crds` / `wait-controller` targets.
   Verify with `make helm-template`, `make deploy-local`, and `make smoke`.

## Accepted Defaults

- Local image: `namespace-class-controller:dev`.
- CRDs are applied separately from the Helm chart.
- Helm manages the controller Deployment, ServiceAccount, ClusterRole, and ClusterRoleBinding.
- First smoke verifies infrastructure readiness; behavior-level smoke comes after reconciliation exists.
- Current broad controller RBAC remains explicit risk because arbitrary resources are in scope.
