.DEFAULT_GOAL := help

SHELL := /bin/bash

ROOT_DIR := $(CURDIR)
LOCAL_BIN := $(ROOT_DIR)/.tools/bin
PATH := $(LOCAL_BIN):$(PATH)
LOCAL_KUBECTL := $(LOCAL_BIN)/kubectl
LOCAL_HELM := $(LOCAL_BIN)/helm
SETUP_ENVTEST := $(LOCAL_BIN)/setup-envtest
ENVTEST_ASSETS_FILE := $(ROOT_DIR)/.tools/envtest-assets-path

GO ?= go
RUBY ?= ruby
KUBECTL ?= $(if $(wildcard $(LOCAL_KUBECTL)),$(LOCAL_KUBECTL),kubectl)
HELM ?= $(if $(wildcard $(LOCAL_HELM)),$(LOCAL_HELM),helm)
MINIKUBE ?= minikube
BIN_DIR ?= $(ROOT_DIR)/bin
CONTROLLER_BIN := $(BIN_DIR)/namespace-class-controller
IMAGE_REPOSITORY ?= namespace-class-controller
IMAGE_TAG ?= dev
LOCAL_IMAGE_TAG ?= $(IMAGE_TAG)-$(shell date +%Y%m%d%H%M%S)
IMAGE_PULL_POLICY ?= IfNotPresent
IMAGE := $(IMAGE_REPOSITORY):$(IMAGE_TAG)
IMAGE_GOOS ?= linux
IMAGE_GOARCH ?= $(shell $(GO) env GOARCH)
CONTAINER_BIN := $(BIN_DIR)/$(IMAGE_GOOS)-$(IMAGE_GOARCH)/namespace-class-controller
RELEASE_NAME ?= namespace-class
RELEASE_NAMESPACE ?= namespace-class-system
ENVTEST_K8S_VERSION ?= 1.35.0
ENVTEST_ASSETS_DIR ?= $(ROOT_DIR)/.tools/envtest
CRD_WAIT_TIMEOUT ?= 60s
CONTROLLER_WAIT_TIMEOUT ?= 120s

.PHONY: help tools envtest-tools doctor build container-binary image-build image-load test envtest vet fmt fmt-fix docs-check manifests-check helm-template check \
	cluster-check deploy-crds wait-crds undeploy-crds deploy restart-controller wait-controller deploy-local deploy-local-with-image undeploy-local smoke clean

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Install project-local kubectl and helm into .tools/bin
	bash scripts/install-tools.sh

$(SETUP_ENVTEST):
	@mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) $(GO) install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

envtest-tools: $(SETUP_ENVTEST) ## Install project-local envtest apiserver/etcd binaries
	@mkdir -p $(ENVTEST_ASSETS_DIR)
	$(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(ENVTEST_ASSETS_DIR) -p path > $(ENVTEST_ASSETS_FILE)
	@echo "KUBEBUILDER_ASSETS=$$(cat $(ENVTEST_ASSETS_FILE))"

doctor: ## Check local prerequisites
	bash scripts/doctor.sh

build: ## Build controller binary
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(CONTROLLER_BIN) ./cmd/namespace-class-controller

container-binary: ## Build linux controller binary for container image
	@mkdir -p $(dir $(CONTAINER_BIN))
	CGO_ENABLED=0 GOOS=$(IMAGE_GOOS) GOARCH=$(IMAGE_GOARCH) $(GO) build -o $(CONTAINER_BIN) ./cmd/namespace-class-controller

image-build: container-binary ## Build local controller container image
	docker build --build-arg BINARY=$(patsubst $(ROOT_DIR)/%,%,$(CONTAINER_BIN)) -t $(IMAGE) .

image-load: image-build ## Load local controller image into minikube
	$(MINIKUBE) image load --overwrite=true --daemon=true $(IMAGE)

test: ## Run unit tests
	$(GO) test ./...

envtest: envtest-tools ## Run envtest-backed integration tests
	KUBEBUILDER_ASSETS="$$(cat $(ENVTEST_ASSETS_FILE))" $(GO) test -tags=envtest ./internal/envtest -count=1

vet: ## Run go vet
	$(GO) vet ./...

fmt: ## Check Go formatting
	@test -z "$$(gofmt -l $$(find . -path './.tools' -prune -o -path './bin' -prune -o -name '*.go' -print))" || { \
		echo "Go files need formatting:"; \
		gofmt -l $$(find . -path './.tools' -prune -o -path './bin' -prune -o -name '*.go' -print); \
		exit 1; \
	}

fmt-fix: ## Format Go files
	gofmt -w $$(find . -path './.tools' -prune -o -path './bin' -prune -o -name '*.go' -print)

docs-check: ## Check docs for trailing whitespace
	@! grep -RIn '[[:blank:]]$$' AGENTS.md README.md CONTEXT.md docs || { echo "Trailing whitespace found"; exit 1; }

manifests-check: ## Validate CRD manifests client-side
	$(RUBY) scripts/check-manifests.rb config/crd/bases config/samples

helm-template: ## Render Helm chart
	$(HELM) template $(RELEASE_NAME) charts/namespace-class \
		--namespace $(RELEASE_NAMESPACE) \
		--set image.repository=$(IMAGE_REPOSITORY) \
		--set image.tag=$(IMAGE_TAG) \
		--set image.pullPolicy=$(IMAGE_PULL_POLICY) >/tmp/namespace-class-helm-rendered.yaml

check: docs-check fmt test envtest vet manifests-check helm-template ## Run local aggregate verification

cluster-check: ## Verify kubectl/minikube cluster access
	$(MINIKUBE) status
	$(KUBECTL) cluster-info
	$(KUBECTL) get nodes

deploy-crds: ## Install CRDs into current cluster
	$(KUBECTL) apply -f config/crd/bases
	$(MAKE) wait-crds

wait-crds: ## Wait for CRDs to be Established
	$(KUBECTL) wait --for=condition=Established crd/namespaceclasses.namespaceclass.akuity.io --timeout=$(CRD_WAIT_TIMEOUT)
	$(KUBECTL) wait --for=condition=Established crd/namespaceclassbindings.namespaceclass.akuity.io --timeout=$(CRD_WAIT_TIMEOUT)

undeploy-crds: ## Remove CRDs from current cluster
	$(KUBECTL) delete --ignore-not-found=true -f config/crd/bases

deploy: deploy-crds ## Deploy controller chart into current cluster
	$(HELM) upgrade --install $(RELEASE_NAME) charts/namespace-class \
		--namespace $(RELEASE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMAGE_REPOSITORY) \
		--set image.tag=$(IMAGE_TAG) \
		--set image.pullPolicy=$(IMAGE_PULL_POLICY)

wait-controller: ## Wait for controller Deployment to become Available
	$(KUBECTL) -n $(RELEASE_NAMESPACE) rollout status deployment/$(RELEASE_NAME)-controller --timeout=$(CONTROLLER_WAIT_TIMEOUT)
	$(KUBECTL) -n $(RELEASE_NAMESPACE) wait --for=condition=Available deployment/$(RELEASE_NAME)-controller --timeout=$(CONTROLLER_WAIT_TIMEOUT)

restart-controller: ## Restart controller Deployment after loading a same-tag local image
	$(KUBECTL) -n $(RELEASE_NAMESPACE) rollout restart deployment/$(RELEASE_NAME)-controller

deploy-local: ## Build, load, deploy, and smoke-test controller in minikube
	$(MAKE) deploy-local-with-image IMAGE_TAG=$(LOCAL_IMAGE_TAG) IMAGE_PULL_POLICY=Never

deploy-local-with-image: deploy-crds image-load
	$(HELM) upgrade --install $(RELEASE_NAME) charts/namespace-class \
		--namespace $(RELEASE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMAGE_REPOSITORY) \
		--set image.tag=$(IMAGE_TAG) \
		--set image.pullPolicy=$(IMAGE_PULL_POLICY)
	$(MAKE) restart-controller
	$(MAKE) wait-controller
	$(MAKE) smoke

undeploy-local: ## Uninstall local controller Helm release
	@if $(HELM) status $(RELEASE_NAME) --namespace $(RELEASE_NAMESPACE) >/dev/null 2>&1; then \
		$(HELM) uninstall $(RELEASE_NAME) --namespace $(RELEASE_NAMESPACE); \
	else \
		echo "release $(RELEASE_NAME) not installed in namespace $(RELEASE_NAMESPACE)"; \
	fi

smoke: ## Run minikube/current-cluster smoke checks
	RELEASE_NAME=$(RELEASE_NAME) RELEASE_NAMESPACE=$(RELEASE_NAMESPACE) CRD_WAIT_TIMEOUT=$(CRD_WAIT_TIMEOUT) CONTROLLER_WAIT_TIMEOUT=$(CONTROLLER_WAIT_TIMEOUT) bash scripts/kube-smoke.sh

clean: ## Remove local build outputs
	rm -rf $(BIN_DIR) .runtime coverage reports
