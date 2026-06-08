#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

KUBECTL="${KUBECTL:-kubectl}"
MANUAL_TEST_NAME="${MANUAL_TEST_NAME:-namespace-class-manual}"

show() {
  local title="$1"
  shift

  echo
  echo "== $title =="
  echo "+ $*"
  "$@" || true
}

show_binding() {
  local name="$1"

  echo
  echo "== NamespaceClassBinding/$name =="
  if ! "$KUBECTL" get namespaceclassbinding "$name" >/dev/null 2>&1; then
    echo "not found"
    return
  fi

  "$KUBECTL" get namespaceclassbinding "$name" -o jsonpath='class={.spec.className} namespace={.spec.namespaceName} observedNamespaceUID={.status.observedNamespaceUID} ready={range .status.conditions[?(@.type=="Ready")]}{.status}{"/"}{.reason}{end}{"\n"}'
  "$KUBECTL" get namespaceclassbinding "$name" -o jsonpath='{range .status.inventory[*]}inventory={.apiVersion}{"/"}{.kind}{" namespace="}{.namespace}{" name="}{.name}{"\n"}{end}'
}

show "Manual test namespaces" "$KUBECTL" get namespace -l "namespaceclass.akuity.io/manual-test=$MANUAL_TEST_NAME" --show-labels
show "Manual test NamespaceClasses" "$KUBECTL" get namespaceclass -l "namespaceclass.akuity.io/manual-test=$MANUAL_TEST_NAME"

show_binding "$MANUAL_TEST_NAME-public"
show_binding "$MANUAL_TEST_NAME-switch"
show_binding "$MANUAL_TEST_NAME-denied"
show_binding "$MANUAL_TEST_NAME-cluster"

show "Happy path managed resources" "$KUBECTL" get serviceaccount,configmap,networkpolicy --namespace "$MANUAL_TEST_NAME-public"
show "Class switch managed resources" "$KUBECTL" get serviceaccount,configmap --namespace "$MANUAL_TEST_NAME-switch"
show "Denied ClusterRoleBinding should be absent" "$KUBECTL" get clusterrolebinding "$MANUAL_TEST_NAME-denied-admin"
show "Cluster-scoped managed resource" "$KUBECTL" get clusterrole "$MANUAL_TEST_NAME-cluster-reader"
show "Cluster scenario namespace finalizers" "$KUBECTL" get namespace "$MANUAL_TEST_NAME-cluster" -o jsonpath='{.metadata.finalizers}{"\n"}'

echo
echo "Trigger class switch:"
echo "  kubectl label namespace $MANUAL_TEST_NAME-switch namespaceclass.akuity.io/name=$MANUAL_TEST_NAME-switch-internal --overwrite"
echo
echo "Trigger cluster-scoped cleanup:"
echo "  kubectl delete namespace $MANUAL_TEST_NAME-cluster --wait=false"
echo
echo "Cleanup all manual test resources:"
echo "  make manual-test-cleanup MANUAL_TEST_NAME=$MANUAL_TEST_NAME"
