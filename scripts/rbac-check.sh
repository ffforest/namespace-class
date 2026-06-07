#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$ROOT_DIR/.tools/bin:$PATH"

KUBECTL="${KUBECTL:-kubectl}"
RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-namespace-class-system}"
SERVICE_ACCOUNT_NAME="${SERVICE_ACCOUNT_NAME:-namespace-class-controller}"
SAMPLE_NAMESPACE="${SAMPLE_NAMESPACE:-default}"
RBAC_SUBJECT="system:serviceaccount:$RELEASE_NAMESPACE:$SERVICE_ACCOUNT_NAME"

missing=0

can_i() {
  local verb="$1"
  local resource="$2"
  shift 2
  "$KUBECTL" auth can-i "$verb" "$resource" --as "$RBAC_SUBJECT" "$@" 2>/dev/null || true
}

require_can_i() {
  local verb="$1"
  local resource="$2"
  local description="$3"
  shift 3

  local result
  result="$(can_i "$verb" "$resource" "$@")"
  if [[ "$result" == "yes" ]]; then
    printf 'ok: %s can %s %s (%s)\n' "$RBAC_SUBJECT" "$verb" "$resource" "$description"
    return
  fi

  printf 'missing: %s cannot %s %s (%s)\n' "$RBAC_SUBJECT" "$verb" "$resource" "$description" >&2
  missing=1
}

warn_if_can_i() {
  local verb="$1"
  local resource="$2"
  local description="$3"
  shift 3

  local result
  result="$(can_i "$verb" "$resource" "$@")"
  if [[ "$result" == "yes" ]]; then
    printf 'warning: %s can %s %s (%s)\n' "$RBAC_SUBJECT" "$verb" "$resource" "$description" >&2
  else
    printf 'ok: %s cannot %s %s (%s)\n' "$RBAC_SUBJECT" "$verb" "$resource" "$description"
  fi
}

"$KUBECTL" get serviceaccount "$SERVICE_ACCOUNT_NAME" --namespace "$RELEASE_NAMESPACE" >/dev/null

echo "Checking required controller RBAC for $RBAC_SUBJECT"

for verb in get list watch update patch; do
  require_can_i "$verb" namespaces "namespace watch and finalizer updates"
done

for verb in get list watch; do
  require_can_i "$verb" namespaceclasses.namespaceclass.akuity.io "read NamespaceClass definitions"
done

for verb in get list watch create update patch delete; do
  require_can_i "$verb" namespaceclassbindings.namespaceclass.akuity.io "manage binding inventory"
done

for verb in update patch; do
  require_can_i "$verb" namespaceclassbindings.namespaceclass.akuity.io "write binding status" --subresource=status
done

for verb in get create update patch delete; do
  require_can_i "$verb" serviceaccounts "representative namespaced managed resource" --namespace "$SAMPLE_NAMESPACE"
done

for verb in get create update patch delete; do
  require_can_i "$verb" clusterroles.rbac.authorization.k8s.io "representative cluster-scoped managed resource"
done

echo "Checking broad/high-risk permissions for visibility"
warn_if_can_i create '*' "broad resource RBAC; production installs should narrow this when possible"
warn_if_can_i delete '*' "broad resource RBAC; production installs should narrow this when possible"
warn_if_can_i create clusterrolebindings.rbac.authorization.k8s.io "runtime GVK policy denies ClusterRoleBinding by default"
warn_if_can_i create validatingwebhookconfigurations.admissionregistration.k8s.io "high-impact cluster-scoped admission resource"

if ((missing != 0)); then
  echo "RBAC check failed: required permissions are missing" >&2
  exit 1
fi

echo "RBAC check completed"
