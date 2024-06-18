PLATFORM       ?= linux/amd64
REGISTRY_NAME  ?= index.docker.io/linode
IMAGE_NAME     ?= linode-blockstorage-csi-driver
REV            := $(shell git describe --long --tags --dirty 2> /dev/null || echo "dev")
IMAGE_VERSION  ?= $(REV)
IMAGE_TAG      ?= $(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)
GOLANGCI_LINT_IMG := golangci/golangci-lint:v1.59-alpine
RELEASE_DIR    ?= release

.PHONY: ci
ci: vet lint test build

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet: fmt
	go vet ./...

.PHONY: lint
lint: vet
	docker run --rm -v $(PWD):/app -w /app ${GOLANGCI_LINT_IMG} golangci-lint run -v
	docker run --rm -v $(PWD):/app -w /app/e2e ${GOLANGCI_LINT_IMG} golangci-lint run -v

.PHONY: test
test: vet verify
	go test -v ./... -cover $(TEST_ARGS)

.PHONY: elevated-test
elevated-test:
	sudo go test -v ./... -cover -tags=elevated $(TEST_ARGS)

.PHONY: build
build:
	go build -o linode-blockstorage-csi-driver -a -ldflags '-X main.vendorVersion='${REV}' -extldflags "-static"' ./main.go

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build --platform=$(PLATFORM) --progress=plain -t $(IMAGE_TAG) --build-arg REV=$(REV) -f ./Dockerfile .

.PHONY: docker-push
docker-push:
	echo "[reminder] Did you run `make docker-build`?"
	docker push $(IMAGE_TAG)

.PHONY: verify
verify:
	go mod verify

.PHONY: clean
clean:
	@GOOS=linux go clean -i -r -x ./...
	-rm -rf _output
	-rm -rf $(RELEASE_DIR)
	-rm -rf ./linode-blockstorage-csi-driver

.PHONY: release
release:
	mkdir -p $(RELEASE_DIR)
	./hack/release-yaml.sh $(IMAGE_VERSION)
	cp ./pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver-$(IMAGE_VERSION).yaml ./$(RELEASE_DIR)
	sed -e 's/appVersion: "latest"/appVersion: "$(IMAGE_VERSION)"/g' ./helm-chart/csi-driver/Chart.yaml
	tar -czvf ./$(RELEASE_DIR)/helm-chart-$(IMAGE_VERSION).tgz -C ./helm-chart/csi-driver .

local-deploy: kind ctlptl tilt kustomize clusterctl
	$(CTLPTL) apply -f .tilt/ctlptl-config.yaml
	

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
CACHE_BIN ?= $(CURDIR)/bin
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
CTLPTL         ?= $(LOCALBIN)/ctlptl
CLUSTERCTL     ?= $(LOCALBIN)/clusterctl
KIND           ?= $(LOCALBIN)/kind
KUSTOMIZE      ?= $(LOCALBIN)/kustomize

## Tool Versions
CTLPTL_VERSION           ?= v0.8.28
CLUSTERCTL_VERSION       ?= v1.6.3
KIND_VERSION             ?= v0.22.0
KUSTOMIZE_VERSION        ?= v5.3.0

.PHONY: tools
tools: ctlptl clusterctl kind kustomize

.PHONY: ctlptl
ctlptl: $(CTLPTL) ## Download ctlptl locally if necessary.
$(CTLPTL): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/tilt-dev/ctlptl/cmd/ctlptl@$(CTLPTL_VERSION)

.PHONY: clusterctl
clusterctl: $(CLUSTERCTL) ## Download clusterctl locally if necessary.
$(CLUSTERCTL): $(LOCALBIN)
	curl -fsSL https://github.com/kubernetes-sigs/cluster-api/releases/download/$(CLUSTERCTL_VERSION)/clusterctl-$(OS)-$(ARCH_SHORT) -o $(CLUSTERCTL)
	chmod +x $(CLUSTERCTL)

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary.
$(KIND): $(LOCALBIN)
	curl -fsSL https://github.com/kubernetes-sigs/kind/releases/download/$(KIND_VERSION)/kind-$(OS)-$(ARCH_SHORT) -o $(KIND)
	chmod +x $(KIND)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: tilt
tilt: $(TILT) ## Download tilt locally if necessary.
$(TILT): $(LOCALBIN)
	TILT_OS=$(OS); \
	if [ $$TILT_OS = "darwin" ]; then \
		TILT_OS=mac; \
	fi; \
	curl -fsSL https://github.com/tilt-dev/tilt/releases/download/v$(TILT_VERSION)/tilt.$(TILT_VERSION).$$TILT_OS.$(ARCH).tar.gz | tar -xzvm -C $(LOCALBIN) tilt

.PHONY: chainsaw
chainsaw: $(CHAINSAW) ## Download chainsaw locally if necessary.
$(CHAINSAW): $(CACHE_BIN)
	GOBIN=$(CACHE_BIN) go install github.com/kyverno/chainsaw@$(CHAINSAW_VERSION)
