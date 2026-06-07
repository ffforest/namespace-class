#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

missing=0

check() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "missing: $name"
    missing=1
    return
  fi
  echo "found: $name ($(command -v "$name"))"
}

check go
check kubectl
check helm
check minikube

if [[ "$missing" -ne 0 ]]; then
  echo ""
  echo "Install project-local kubectl/helm with: make tools"
  echo "Install Go separately, then rerun make doctor."
  exit 1
fi

go version
kubectl version --client
helm version --short
minikube version

echo ""
echo "Toolchain looks available. Run make cluster-check to verify cluster access."

