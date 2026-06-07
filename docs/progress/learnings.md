# Learnings

## 2026-06-07 Minikube same-tag images

When repeatedly deploying `namespace-class-controller:dev`, minikube can keep running an older image even after `minikube image load` and a Deployment restart. Use a unique local tag for `make deploy-local` so kubelet resolves a new image reference on every local deployment.

- For arbitrary Kubernetes resources, inventory must not live only inside the target namespace when cluster-scoped resources are supported. A cluster-scoped binding object is a safer default.
