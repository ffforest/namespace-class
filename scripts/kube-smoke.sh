#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

kubectl cluster-info >/dev/null
kubectl get nodes

echo "Checking CRDs are installed"
kubectl get crd namespaceclasses.namespaceclass.akuity.io
kubectl get crd namespaceclassbindings.namespaceclass.akuity.io

echo "Server-side dry-run sample resources"
kubectl apply --dry-run=server -f "$ROOT_DIR/config/samples"

echo "Smoke check completed"

