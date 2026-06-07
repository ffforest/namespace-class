# Namespace Class Switching

## Goal

When a namespace label changes from one `NamespaceClass` to another, the controller reconciles the namespace against the new class, creates the new desired resources, deletes old managed resources that are no longer desired, and updates the binding spec and inventory.

## Scope

- Add envtest coverage for switching a namespace from class A to class B.
- Verify the old class managed resource is deleted.
- Verify the new class managed resource is created.
- Verify `NamespaceClassBinding.spec.className` and `status.inventory` reflect the new class.
- Upgrade smoke to cover class switching in minikube.

## Out Of Scope

- Cleanup after removing the class label entirely.
- `NamespaceClass` deletion fan-out.
- Cluster-scoped finalizer behavior.
- Admission validation.

## Verification

- `make envtest`
- `make check`
- `make deploy-local`
- `make smoke`
