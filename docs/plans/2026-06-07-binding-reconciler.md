# Binding Reconciler Plan

## Goal

Create the first business reconciliation slice:

1. Add Go API types for `NamespaceClass` and `NamespaceClassBinding`.
2. Register the API group in the controller manager scheme.
3. Watch `Namespace` objects.
4. When a namespace has `namespaceclass.akuity.io/name`, create or update the matching cluster-scoped `NamespaceClassBinding`.
5. Record basic binding status so the controller behavior is observable.
6. Upgrade smoke to verify binding creation when the controller is deployed.

## Scope

This slice does not apply, update, or delete arbitrary managed resources from `NamespaceClass.spec.resources`.

It also does not implement class-switch cleanup, class-update fan-out, namespace finalizers, admission webhooks, or GVK allowlist/denylist enforcement.

## Verification

1. Add an envtest that starts the manager, creates a `NamespaceClass` and labeled `Namespace`, and waits for `NamespaceClassBinding`.
2. Run the envtest before implementation and confirm it fails because the binding is not created.
3. Implement the minimum API types and reconciler needed to pass.
4. Run `make envtest`, `make check`, `make deploy-local`, and `make smoke`.

## Local Image Note

Because local development often reuses `namespace-class-controller:dev`, `make deploy-local` builds and deploys a unique timestamped tag by default and restarts the Deployment after `minikube image load`; otherwise Helm can report a successful upgrade while the existing Pod keeps running a previous same-tag image.
