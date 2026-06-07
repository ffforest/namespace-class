#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

RELEASE_NAME="${RELEASE_NAME:-namespace-class}"
RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-namespace-class-system}"
CRD_WAIT_TIMEOUT="${CRD_WAIT_TIMEOUT:-60s}"
CONTROLLER_WAIT_TIMEOUT="${CONTROLLER_WAIT_TIMEOUT:-120s}"

cleanup_behavior_smoke() {
  if [[ -n "${BEHAVIOR_SMOKE_NAME:-}" ]]; then
    kubectl delete --ignore-not-found=true serviceaccount "$BEHAVIOR_SMOKE_NAME-app" --namespace "$BEHAVIOR_SMOKE_NAME"
    kubectl delete --ignore-not-found=true serviceaccount "$BEHAVIOR_SMOKE_NAME-old" --namespace "$BEHAVIOR_SMOKE_NAME"
    kubectl delete --ignore-not-found=true serviceaccount "$BEHAVIOR_SMOKE_NAME-internal" --namespace "$BEHAVIOR_SMOKE_NAME"
    kubectl delete --ignore-not-found=true namespaceclassbinding "$BEHAVIOR_SMOKE_NAME"
    kubectl delete --ignore-not-found=true namespace "$BEHAVIOR_SMOKE_NAME" --wait=false
    kubectl delete --ignore-not-found=true namespaceclass "$BEHAVIOR_SMOKE_NAME-internal"
    kubectl delete --ignore-not-found=true namespaceclass "$BEHAVIOR_SMOKE_NAME"
  fi
}

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

  echo "Checking controller creates NamespaceClassBinding and reconciles managed resources"
  BEHAVIOR_SMOKE_NAME="${BEHAVIOR_SMOKE_NAME:-namespace-class-smoke-$(date +%s)}"
  trap cleanup_behavior_smoke EXIT

  kubectl apply -f - <<YAML
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $BEHAVIOR_SMOKE_NAME
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: $BEHAVIOR_SMOKE_NAME-old
---
apiVersion: v1
kind: Namespace
metadata:
  name: $BEHAVIOR_SMOKE_NAME
  labels:
    namespaceclass.akuity.io/name: $BEHAVIOR_SMOKE_NAME
YAML

  wait_seconds="${CONTROLLER_WAIT_TIMEOUT%s}"
  if [[ "$wait_seconds" == "$CONTROLLER_WAIT_TIMEOUT" ]]; then
    wait_seconds=120
  fi
  for ((i = 0; i < wait_seconds; i++)); do
    if kubectl get "namespaceclassbinding/$BEHAVIOR_SMOKE_NAME" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  kubectl get "namespaceclassbinding/$BEHAVIOR_SMOKE_NAME"
  kubectl wait --for=condition=Ready "namespaceclassbinding/$BEHAVIOR_SMOKE_NAME" --timeout="$CONTROLLER_WAIT_TIMEOUT"
  class_name="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{.spec.className}')"
  if [[ "$class_name" != "$BEHAVIOR_SMOKE_NAME" ]]; then
    echo "expected binding className $BEHAVIOR_SMOKE_NAME, got $class_name" >&2
    exit 1
  fi
  observed_uid="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{.status.observedNamespaceUID}')"
  if [[ -z "$observed_uid" ]]; then
    echo "expected binding status.observedNamespaceUID to be set" >&2
    exit 1
  fi
  kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-old" --namespace "$BEHAVIOR_SMOKE_NAME"
  inventory_name="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{.status.inventory[0].name}')"
  if [[ "$inventory_name" != "$BEHAVIOR_SMOKE_NAME-old" ]]; then
    echo "expected first inventory entry to be $BEHAVIOR_SMOKE_NAME-old, got $inventory_name" >&2
    exit 1
  fi
  inventory_namespace="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{.status.inventory[0].namespace}')"
  if [[ "$inventory_namespace" != "$BEHAVIOR_SMOKE_NAME" ]]; then
    echo "expected first inventory entry namespace to be $BEHAVIOR_SMOKE_NAME, got $inventory_namespace" >&2
    exit 1
  fi

  echo "Checking controller automatically reconciles NamespaceClass updates"
  kubectl apply -f - <<YAML
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $BEHAVIOR_SMOKE_NAME
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: $BEHAVIOR_SMOKE_NAME-app
YAML

  for ((i = 0; i < wait_seconds; i++)); do
    if kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-app" --namespace "$BEHAVIOR_SMOKE_NAME" >/dev/null 2>&1; then
      break
    fi
    if ((i == wait_seconds - 1)); then
      echo "expected ServiceAccount $BEHAVIOR_SMOKE_NAME-app to be created" >&2
      exit 1
    fi
    sleep 1
  done
  kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-app" --namespace "$BEHAVIOR_SMOKE_NAME"

  for ((i = 0; i < wait_seconds; i++)); do
    if ! kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-old" --namespace "$BEHAVIOR_SMOKE_NAME" >/dev/null 2>&1; then
      break
    fi
    if ((i == wait_seconds - 1)); then
      echo "expected stale ServiceAccount $BEHAVIOR_SMOKE_NAME-old to be deleted" >&2
      exit 1
    fi
    sleep 1
  done

  for ((i = 0; i < wait_seconds; i++)); do
    inventory_names="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{range .status.inventory[*]}{.name}{" "}{end}')"
    if [[ " $inventory_names " == *" $BEHAVIOR_SMOKE_NAME-app "* && " $inventory_names " != *" $BEHAVIOR_SMOKE_NAME-old "* ]]; then
      break
    fi
    if ((i == wait_seconds - 1)); then
      echo "expected inventory to contain $BEHAVIOR_SMOKE_NAME-app and omit $BEHAVIOR_SMOKE_NAME-old, got: $inventory_names" >&2
      exit 1
    fi
    sleep 1
  done

  echo "Checking controller switches namespace classes"
  kubectl apply -f - <<YAML
apiVersion: namespaceclass.akuity.io/v1alpha1
kind: NamespaceClass
metadata:
  name: $BEHAVIOR_SMOKE_NAME-internal
spec:
  resources:
    - apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: $BEHAVIOR_SMOKE_NAME-internal
YAML
  kubectl label namespace "$BEHAVIOR_SMOKE_NAME" namespaceclass.akuity.io/name="$BEHAVIOR_SMOKE_NAME-internal" --overwrite

  for ((i = 0; i < wait_seconds; i++)); do
    if kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-internal" --namespace "$BEHAVIOR_SMOKE_NAME" >/dev/null 2>&1; then
      break
    fi
    if ((i == wait_seconds - 1)); then
      echo "expected ServiceAccount $BEHAVIOR_SMOKE_NAME-internal to be created after class switch" >&2
      exit 1
    fi
    sleep 1
  done
  kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-internal" --namespace "$BEHAVIOR_SMOKE_NAME"

  for ((i = 0; i < wait_seconds; i++)); do
    if ! kubectl get serviceaccount "$BEHAVIOR_SMOKE_NAME-app" --namespace "$BEHAVIOR_SMOKE_NAME" >/dev/null 2>&1; then
      break
    fi
    if ((i == wait_seconds - 1)); then
      echo "expected switched-away ServiceAccount $BEHAVIOR_SMOKE_NAME-app to be deleted" >&2
      exit 1
    fi
    sleep 1
  done

  for ((i = 0; i < wait_seconds; i++)); do
    class_name="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{.spec.className}')"
    inventory_names="$(kubectl get namespaceclassbinding "$BEHAVIOR_SMOKE_NAME" -o jsonpath='{range .status.inventory[*]}{.name}{" "}{end}')"
    if [[ "$class_name" == "$BEHAVIOR_SMOKE_NAME-internal" && " $inventory_names " == *" $BEHAVIOR_SMOKE_NAME-internal "* && " $inventory_names " != *" $BEHAVIOR_SMOKE_NAME-app "* ]]; then
      break
    fi
    if ((i == wait_seconds - 1)); then
      echo "expected binding to switch to $BEHAVIOR_SMOKE_NAME-internal with matching inventory, got class=$class_name inventory=$inventory_names" >&2
      exit 1
    fi
    sleep 1
  done
else
  echo "Controller Deployment not found; skipping controller readiness check"
fi

echo "Smoke check completed"
