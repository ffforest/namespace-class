# GVK Policy Runtime Guard

## Goal

Prevent `NamespaceClass` authors from using the controller ServiceAccount as a privilege proxy for high-risk resources such as `ClusterRoleBinding`.

## First Slice

- Implement a reusable GVK policy engine.
- Default policy is allow-all with a default denylist for `rbac.authorization.k8s.io/v1/ClusterRoleBinding`.
- Support optional allowlist and denylist configuration through controller flags and Helm values.
- Denylist wins over allowlist.
- If allowlist is non-empty, resources outside the allowlist are denied.
- Controller reconciliation validates all desired resources before applying any resource.
- Denied resources update `NamespaceClassBinding` with `Ready=False` and a clear deny reason.

## Out Of Scope

- Admission webhook server and `ValidatingWebhookConfiguration`.
- Webhook TLS/cert generation, rotation, and caBundle injection.
- Full security taxonomy for every high-risk Kubernetes GVK.
- User/subject-aware policy decisions.

## Verification

- Unit tests for policy parsing and allow/deny precedence.
- Envtest that proves default policy denies `ClusterRoleBinding` and does not create it.
- Smoke check that a denied `ClusterRoleBinding` is rejected by runtime guard in minikube.
- `make check`
- `make deploy-local`
