# Learnings

- For arbitrary Kubernetes resources, inventory must not live only inside the target namespace when cluster-scoped resources are supported. A cluster-scoped binding object is a safer default.

