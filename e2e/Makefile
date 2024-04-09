export GO111MODULE=on

IMAGE_TAG?=canary
IMAGE_NAME?="linode/linode-blockstorage-csi-driver"
K8S_VERSION?="v1.28.7"

.PHONY: test clean check-token test-manifest

test-manifest: kustomize
	../hack/generate-yaml.sh $(IMAGE_TAG) $(IMAGE_NAME) > test/manifest/linode-blockstorage-csi-driver.yaml

test-cluster: ctlptl clean-cluster
	./test/scripts/create_management_cluster.sh $(K8S_VERSION)

clean-cluster: ctlptl
	ctlptl delete cluster kind-management --ignore-not-found
	rm -rf ./test/kind-management.conf

test: clusterctl ginkgo check-token test-manifest test-cluster
	go list -m; \
	ginkgo -r --vv --procs=1 --trace --cover $(TEST_ARGS) -- --v=3 --k8s-version=${K8S_VERSION} $(SUITE_ARGS)

	$(MAKE) clean-cluster

clean: check-token
	cd test; \
	./scripts/delete_cluster.sh csi-linode-for-reuse

check-token:
	@if test "$(LINODE_API_TOKEN)" = "" ; then \
	  echo "LINODE_API_TOKEN must be set"; \
	  exit 1; \
	fi

#####################################################################
# OS / ARCH
#####################################################################
OS=$(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(shell uname -m)
ARCH_SHORT=$(ARCH)
ifeq ($(ARCH_SHORT),x86_64)
ARCH_SHORT := amd64
else ifeq ($(ARCH_SHORT),aarch64)
ARCH_SHORT := arm64
endif

## --------------------------------------
## Build Dependencies
## --------------------------------------

##@ Build Dependencies:

## Location to install dependencies to

# Use CACHE_BIN for tools that cannot use devbox and LOCALBIN for tools that can use either method
CACHE_BIN ?= $(CURDIR)/../bin
LOCALBIN ?= $(CACHE_BIN)

DEVBOX_BIN ?= $(DEVBOX_PACKAGES_DIR)/bin

# if the $DEVBOX_PACKAGES_DIR env variable exists that means we are within a devbox shell and can safely
# use devbox's bin for our tools
ifdef DEVBOX_PACKAGES_DIR
	LOCALBIN = $(DEVBOX_BIN)
endif

export PATH := $(CACHE_BIN):$(PATH)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

##@ Tooling Binaries:
GINKGO         ?= $(LOCALBIN)/ginkgo
CTLPTL         ?= $(LOCALBIN)/ctlptl
CLUSTERCTL     ?= $(LOCALBIN)/clusterctl
KUSTOMIZE      ?= $(LOCALBIN)/kustomize

## Tool Versions
GINKGO_VERSION           ?= v2.13.2
CTLPTL_VERSION           ?= v0.8.28
CLUSTERCTL_VERSION       ?= v1.6.3
KUSTOMIZE_VERSION        ?= v5.3.0

.PHONY: tools
tools: ginkgo ctlptl clusterctl kustomize

.PHONY: ginkgo
ginkgo: $(GINKGO)
$(GINKGO): $(LOCALBIN) ## Download ginkgo locally if necessary.
	GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

.PHONY: ctlptl
ctlptl: $(CTLPTL) ## Download ctlptl locally if necessary.
$(CTLPTL): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/tilt-dev/ctlptl/cmd/ctlptl@$(CTLPTL_VERSION)

.PHONY: clusterctl
clusterctl: $(CLUSTERCTL) ## Download clusterctl locally if necessary.
$(CLUSTERCTL): $(LOCALBIN)
	curl -fsSL https://github.com/kubernetes-sigs/cluster-api/releases/download/$(CLUSTERCTL_VERSION)/clusterctl-$(OS)-$(ARCH_SHORT) -o $(CLUSTERCTL)
	chmod +x $(CLUSTERCTL)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)
