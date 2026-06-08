#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

KUBECTL="${KUBECTL:-kubectl}"
MANUAL_TEST_NAME="${MANUAL_TEST_NAME:-namespace-class-manual}"

delete() {
  echo "+ kubectl $*"
  "$KUBECTL" "$@" || true
}

echo "Cleaning manual test resources with prefix $MANUAL_TEST_NAME"

for namespace in \
  "$MANUAL_TEST_NAME-public" \
  "$MANUAL_TEST_NAME-switch" \
  "$MANUAL_TEST_NAME-denied" \
  "$MANUAL_TEST_NAME-cluster"; do
  delete label namespace "$namespace" namespaceclass.akuity.io/name- --overwrite
done

delete delete --ignore-not-found=true clusterrolebinding "$MANUAL_TEST_NAME-denied-admin"
delete delete --ignore-not-found=true clusterrole "$MANUAL_TEST_NAME-cluster-reader"

delete delete --ignore-not-found=true namespaceclassbinding "$MANUAL_TEST_NAME-public"
delete delete --ignore-not-found=true namespaceclassbinding "$MANUAL_TEST_NAME-switch"
delete delete --ignore-not-found=true namespaceclassbinding "$MANUAL_TEST_NAME-denied"
delete delete --ignore-not-found=true namespaceclassbinding "$MANUAL_TEST_NAME-cluster"

delete delete --ignore-not-found=true namespaceclass "$MANUAL_TEST_NAME-public"
delete delete --ignore-not-found=true namespaceclass "$MANUAL_TEST_NAME-switch-public"
delete delete --ignore-not-found=true namespaceclass "$MANUAL_TEST_NAME-switch-internal"
delete delete --ignore-not-found=true namespaceclass "$MANUAL_TEST_NAME-denied"
delete delete --ignore-not-found=true namespaceclass "$MANUAL_TEST_NAME-cluster"

delete delete --ignore-not-found=true namespace "$MANUAL_TEST_NAME-public" --wait=false
delete delete --ignore-not-found=true namespace "$MANUAL_TEST_NAME-switch" --wait=false
delete delete --ignore-not-found=true namespace "$MANUAL_TEST_NAME-denied" --wait=false
delete delete --ignore-not-found=true namespace "$MANUAL_TEST_NAME-cluster" --wait=false

echo
echo "Cleanup requested. Re-run this script if any terminating resources still appear after controller reconciliation."
