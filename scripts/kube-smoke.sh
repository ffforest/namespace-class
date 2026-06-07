#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

RELEASE_NAME="${RELEASE_NAME:-namespace-class}"
RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-namespace-class-system}"
CRD_WAIT_TIMEOUT="${CRD_WAIT_TIMEOUT:-60s}"
CONTROLLER_WAIT_TIMEOUT="${CONTROLLER_WAIT_TIMEOUT:-120s}"

kubectl cluster-info >/dev/null
kubectl get nodes

echo "Checking CRDs are installed"
kubectl get crd namespaceclasses.namespaceclass.akuity.io
kubectl get crd namespaceclassbindings.namespaceclass.akuity.io
kubectl wait --for=condition=Established crd/namespaceclasses.namespaceclass.akuity.io --timeout="$CRD_WAIT_TIMEOUT"
kubectl wait --for=condition=Established crd/namespaceclassbindings.namespaceclass.akuity.io --timeout="$CRD_WAIT_TIMEOUT"

echo "Server-side dry-run sample resources"
kubectl apply --dry-run=server -f "$ROOT_DIR/config/samples"

if kubectl -n "$RELEASE_NAMESPACE" get deployment "$RELEASE_NAME-controller" >/dev/null 2>&1; then
  echo "Checking controller Deployment is available"
  kubectl -n "$RELEASE_NAMESPACE" rollout status "deployment/$RELEASE_NAME-controller" --timeout="$CONTROLLER_WAIT_TIMEOUT"
  kubectl -n "$RELEASE_NAMESPACE" wait --for=condition=Available "deployment/$RELEASE_NAME-controller" --timeout="$CONTROLLER_WAIT_TIMEOUT"
else
  echo "Controller Deployment not found; skipping controller readiness check"
fi

echo "Smoke check completed"
