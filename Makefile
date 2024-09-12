PLATFORM           ?= linux/amd64
REGISTRY_NAME      ?= index.docker.io
IMAGE_NAME         ?= linode/linode-blockstorage-csi-driver
REV                := $(shell git describe --long --tags --dirty 2> /dev/null || echo "dev")
ifdef DEV_TAG_EXTENSION
IMAGE_VERSION      ?= $(REV)-$(DEV_TAG_EXTENSION)
else
IMAGE_VERSION      ?= $(REV)
endif
IMAGE_TAG          ?= $(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)
GOLANGCI_LINT_IMG  := golangci/golangci-lint:v1.59-alpine
RELEASE_DIR        ?= release
DOCKERFILE         ?= Dockerfile

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

#####################################################################
# Formatting and Linting
#####################################################################
.PHONY: fmt
fmt:
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) go fmt ./...

.PHONY: vet
vet: fmt
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) go vet ./...

.PHONY: lint
lint: vet
	docker run --platform=$(PLATFORM) --rm -v $(PWD):/app -w /app ${GOLANGCI_LINT_IMG} golangci-lint run -v

.PHONY: verify
verify:
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) go mod verify

.PHONY: clean
clean:
	@GOOS=linux go clean -i -r -x ./...
	-rm -rf _output
	-rm -rf $(RELEASE_DIR)
	-rm -rf ./linode-blockstorage-csi-driver

#####################################################################
# Dev Setup
#####################################################################

CLUSTER_NAME         ?= csi-driver-cluster-$(shell git rev-parse --short HEAD)
K8S_VERSION          ?= "v1.29.1"
CAPI_VERSION         ?= "v1.6.3"
HELM_VERSION         ?= "v0.2.1"
CAPL_VERSION         ?= "v0.3.1"
CONTROLPLANE_NODES   ?= 1
WORKER_NODES         ?= 0

.PHONY: build
build:
	CGO_ENABLED=1 go build -o linode-blockstorage-csi-driver -a -ldflags '-X main.vendorVersion='${IMAGE_VERSION}'' ./main.go

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build --platform=$(PLATFORM) --progress=plain -t $(IMAGE_TAG) --build-arg REV=$(IMAGE_VERSION) -f ./$(DOCKERFILE) .

.PHONY: docker-push
docker-push:
	docker push $(IMAGE_TAG)

.PHONY: local-docker-setup
local-docker-setup: build docker-build docker-push

.PHONY: mgmt-and-capl-cluster
mgmt-and-capl-cluster: local-docker-setup mgmt-cluster capl-cluster

.PHONY: capl-cluster
capl-cluster:
	# Create a CAPL cluster without CSI driver and wait for it to be ready
	clusterctl generate cluster $(CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) \
		--infrastructure linode-linode:$(CAPL_VERSION) \
		--control-plane-machine-count $(CONTROLPLANE_NODES) --worker-machine-count $(WORKER_NODES) \
		--flavor kubeadm-vpcless \
		| yq 'select(.metadata.name != "$(CLUSTER_NAME)-csi-driver-linode")' \
		| kubectl apply -f -
	kubectl wait --for=condition=ControlPlaneReady  cluster/$(CLUSTER_NAME) --timeout=600s
	clusterctl get kubeconfig $(CLUSTER_NAME) > test-cluster-kubeconfig.yaml

	# Install CSI driver and wait for it to be ready
	cat tests/e2e/setup/linode-secret.yaml | envsubst | KUBECONFIG=test-cluster-kubeconfig.yaml kubectl apply -f -
	hack/generate-yaml.sh $(IMAGE_VERSION) $(IMAGE_NAME) > templates.yaml
	# kubectl apply -f templates.yaml
	# KUBECONFIG=test-cluster-kubeconfig.yaml kubectl rollout status -n kube-system daemonset/csi-linode-node --timeout=600s
	# KUBECONFIG=test-cluster-kubeconfig.yaml kubectl rollout status -n kube-system statefulset/csi-linode-controller --timeout=600s

.PHONY: mgmt-cluster
mgmt-cluster:
	# Create a mgmt cluster
	ctlptl apply -f tests/e2e/setup/ctlptl-config.yaml
	clusterctl init \
		--wait-providers \
		--wait-provider-timeout 600 \
		--core cluster-api:${CAPI_VERSION} \
		--addon helm:${HELM_VERSION} \
		--infrastructure linode-linode:${CAPL_VERSION}

.PHONY: cleanup-cluster
cleanup-cluster:
	-kubectl delete cluster --all
	-kind delete cluster -n capl
	-rm -f luks.key

#####################################################################
# Test Setup
#####################################################################

.PHONY: generate-mock
generate-mock:
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) mockgen -source=internal/driver/nodeserver_helpers.go -destination=mocks/mock_nodeserver.go -package=mocks
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) mockgen -source=pkg/mount-manager/device-utils.go -destination=mocks/mock_deviceutils.go -package=mocks
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) mockgen -source=pkg/mount-manager/fs-utils.go -destination=mocks/mock_fsutils.go -package=mocks

.PHONY: test
test: vet verify generate-mock
	docker run --platform=$(PLATFORM) -it $(IMAGE_TAG) go test `go list ./... | grep -v ./mocks$$` -cover $(TEST_ARGS)

.PHONY: elevated-test
elevated-test:
	sudo go test `go list ./... | grep -v ./mocks$$` -cover -tags=elevated $(TEST_ARGS)

.PHONY: e2e-test
e2e-test:
	openssl rand -out luks.key 64
	CONTROLPLANE_NODES=$(CONTROLPLANE_NODES) WORKER_NODES=$(WORKER_NODES) KUBECONFIG=test-cluster-kubeconfig.yaml LUKS_KEY=$$(base64 luks.key | tr -d '\n') chainsaw test ./tests/e2e --parallel 2

.PHONY: csi-sanity-test
csi-sanity-test:
	KUBECONFIG=test-cluster-kubeconfig.yaml ./tests/csi-sanity/run-tests.sh

.PHONY: upstream-e2e-tests
upstream-e2e-tests:
	OS=$(OS) ARCH=$(ARCH_SHORT) K8S_VERSION=$(K8S_VERSION) KUBECONFIG=test-cluster-kubeconfig.yaml ./tests/upstream-e2e/run-tests.sh

#####################################################################
# CI Setup
#####################################################################
.PHONY: ci
ci: vet lint test build

#####################################################################
# Release
#####################################################################
.PHONY: release
release:
	mkdir -p $(RELEASE_DIR)
	./hack/release-yaml.sh $(IMAGE_VERSION)
	cp ./internal/driver/deploy/releases/linode-blockstorage-csi-driver-$(IMAGE_VERSION).yaml ./$(RELEASE_DIR)
	sed -e 's/appVersion: "latest"/appVersion: "$(IMAGE_VERSION)"/g' ./helm-chart/csi-driver/Chart.yaml
	tar -czvf ./$(RELEASE_DIR)/helm-chart-$(IMAGE_VERSION).tgz -C ./helm-chart/csi-driver .
