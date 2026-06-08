#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

KUBECTL="${KUBECTL:-kubectl}"
RELEASE_NAME="${RELEASE_NAME:-namespace-class}"
RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-namespace-class-system}"
CONTROLLER_WAIT_TIMEOUT="${CONTROLLER_WAIT_TIMEOUT:-120s}"
MANUAL_TEST_NAME="${MANUAL_TEST_NAME:-namespace-class-manual}"

wait_seconds="${CONTROLLER_WAIT_TIMEOUT%s}"
if [[ "$wait_seconds" == "$CONTROLLER_WAIT_TIMEOUT" ]]; then
  wait_seconds=120
fi

wait_for_resource() {
  local description="$1"
  shift

  for ((i = 0; i < wait_seconds; i++)); do
    if "$KUBECTL" "$@" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  echo "timed out waiting for $description" >&2
  exit 1
}

wait_for_binding_ready() {
  local name="$1"
  local expected_reason="${2:-BindingRecorded}"

  for ((i = 0; i < wait_seconds; i++)); do
    condition="$("$KUBECTL" get namespaceclassbinding "$name" -o jsonpath='{range .status.conditions[?(@.type=="Ready")]}{.status}{"/"}{.reason}{end}' 2>/dev/null || true)"
    if [[ "$condition" == "True/$expected_reason" ]]; then
      return
    fi
    sleep 1
  done

  echo "timed out waiting for NamespaceClassBinding $name Ready=True/$expected_reason" >&2
  exit 1
}

wait_for_binding_denied() {
  local name="$1"

  for ((i = 0; i < wait_seconds; i++)); do
    condition="$("$KUBECTL" get namespaceclassbinding "$name" -o jsonpath='{range .status.conditions[?(@.type=="Ready")]}{.status}{"/"}{.reason}{end}' 2>/dev/null || true)"
    if [[ "$condition" == "False/GVKDenied" ]]; then
      return
    fi
    sleep 1
  done

  echo "timed out waiting for NamespaceClassBinding $name Ready=False/GVKDenied" >&2
  exit 1
}

echo "Checking cluster and controller"
"$KUBECTL" cluster-info >/dev/null
"$KUBECTL" wait --for=condition=Established crd/namespaceclasses.namespaceclass.akuity.io --timeout="$CONTROLLER_WAIT_TIMEOUT"
"$KUBECTL" wait --for=condition=Established crd/namespaceclassbindings.namespaceclass.akuity.io --timeout="$CONTROLLER_WAIT_TIMEOUT"
"$KUBECTL" -n "$RELEASE_NAMESPACE" rollout status "deployment/$RELEASE_NAME-controller" --timeout="$CONTROLLER_WAIT_TIMEOUT"
"$KUBECTL" -n "$RELEASE_NAMESPACE" wait --for=condition=Available "deployment/$RELEASE_NAME-controller" --timeout="$CONTROLLER_WAIT_TIMEOUT"

echo "Creating manual NamespaceClass scenarios with prefix $MANUAL_TEST_NAME"
"$KUBECTL" apply -f - <<YAML
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $MANUAL_TEST_NAME-public
  labels:
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: "{{ .Namespace.Name }}-public-sa"
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "{{ .Namespace.Name }}-public-config"
      data:
        class: "{{ .Class.Name }}"
        namespace: "{{ .Namespace.Name }}"
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      metadata:
        name: "{{ .Namespace.Name }}-public-network"
      spec:
        podSelector: {}
        policyTypes:
          - Ingress
        ingress:
          - from:
              - ipBlock:
                  cidr: 0.0.0.0/0
---
apiVersion: v1
kind: Namespace
metadata:
  name: $MANUAL_TEST_NAME-public
  labels:
    namespaceclass.akuity.io/name: $MANUAL_TEST_NAME-public
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
---
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $MANUAL_TEST_NAME-switch-public
  labels:
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: "{{ .Namespace.Name }}-public-sa"
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "{{ .Namespace.Name }}-public-config"
      data:
        mode: public
---
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $MANUAL_TEST_NAME-switch-internal
  labels:
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: "{{ .Namespace.Name }}-internal-sa"
    - apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "{{ .Namespace.Name }}-internal-config"
      data:
        mode: internal
---
apiVersion: v1
kind: Namespace
metadata:
  name: $MANUAL_TEST_NAME-switch
  labels:
    namespaceclass.akuity.io/name: $MANUAL_TEST_NAME-switch-public
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
---
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $MANUAL_TEST_NAME-denied
  labels:
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
spec:
  resources:
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRoleBinding
      metadata:
        name: $MANUAL_TEST_NAME-denied-admin
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: ClusterRole
        name: cluster-admin
      subjects:
        - kind: ServiceAccount
          name: default
          namespace: $MANUAL_TEST_NAME-denied
---
apiVersion: v1
kind: Namespace
metadata:
  name: $MANUAL_TEST_NAME-denied
  labels:
    namespaceclass.akuity.io/name: $MANUAL_TEST_NAME-denied
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
---
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $MANUAL_TEST_NAME-cluster
  labels:
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
spec:
  resources:
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRole
      metadata:
        name: "{{ .Namespace.Name }}-reader"
      rules:
        - apiGroups:
            - ""
          resources:
            - pods
          verbs:
            - get
            - list
---
apiVersion: v1
kind: Namespace
metadata:
  name: $MANUAL_TEST_NAME-cluster
  labels:
    namespaceclass.akuity.io/name: $MANUAL_TEST_NAME-cluster
    namespaceclass.akuity.io/manual-test: $MANUAL_TEST_NAME
YAML

echo "Waiting for initial reconciles"
wait_for_binding_ready "$MANUAL_TEST_NAME-public"
wait_for_resource "ServiceAccount $MANUAL_TEST_NAME-public-public-sa" get serviceaccount "$MANUAL_TEST_NAME-public-public-sa" --namespace "$MANUAL_TEST_NAME-public"
wait_for_resource "ConfigMap $MANUAL_TEST_NAME-public-public-config" get configmap "$MANUAL_TEST_NAME-public-public-config" --namespace "$MANUAL_TEST_NAME-public"
wait_for_resource "NetworkPolicy $MANUAL_TEST_NAME-public-public-network" get networkpolicy "$MANUAL_TEST_NAME-public-public-network" --namespace "$MANUAL_TEST_NAME-public"

wait_for_binding_ready "$MANUAL_TEST_NAME-switch"
wait_for_resource "ServiceAccount $MANUAL_TEST_NAME-switch-public-sa" get serviceaccount "$MANUAL_TEST_NAME-switch-public-sa" --namespace "$MANUAL_TEST_NAME-switch"
wait_for_resource "ConfigMap $MANUAL_TEST_NAME-switch-public-config" get configmap "$MANUAL_TEST_NAME-switch-public-config" --namespace "$MANUAL_TEST_NAME-switch"

wait_for_binding_denied "$MANUAL_TEST_NAME-denied"
if "$KUBECTL" get clusterrolebinding "$MANUAL_TEST_NAME-denied-admin" >/dev/null 2>&1; then
  echo "expected denied ClusterRoleBinding $MANUAL_TEST_NAME-denied-admin not to exist" >&2
  exit 1
fi

wait_for_binding_ready "$MANUAL_TEST_NAME-cluster"
wait_for_resource "ClusterRole $MANUAL_TEST_NAME-cluster-reader" get clusterrole "$MANUAL_TEST_NAME-cluster-reader"

echo
echo "Manual test scenarios are ready."
echo
echo "Observe current state:"
echo "  make manual-test-status MANUAL_TEST_NAME=$MANUAL_TEST_NAME"
echo
echo "Trigger class switch:"
echo "  kubectl label namespace $MANUAL_TEST_NAME-switch namespaceclass.akuity.io/name=$MANUAL_TEST_NAME-switch-internal --overwrite"
echo
echo "Trigger cluster-scoped cleanup:"
echo "  kubectl delete namespace $MANUAL_TEST_NAME-cluster --wait=false"
echo
echo "Cleanup all manual test resources:"
echo "  make manual-test-cleanup MANUAL_TEST_NAME=$MANUAL_TEST_NAME"
