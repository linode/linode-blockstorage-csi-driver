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

.PHONY: test
test: vet verify generate-mock
	go test `go list ./... | grep -v ./mocks$$` -cover $(TEST_ARGS)

.PHONY: elevated-test
elevated-test:
	sudo go test `go list ./... | grep -v ./mocks$$` -cover -tags=elevated $(TEST_ARGS)

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
	cp ./internal/driver/deploy/releases/linode-blockstorage-csi-driver-$(IMAGE_VERSION).yaml ./$(RELEASE_DIR)
	sed -e 's/appVersion: "latest"/appVersion: "$(IMAGE_VERSION)"/g' ./helm-chart/csi-driver/Chart.yaml
	tar -czvf ./$(RELEASE_DIR)/helm-chart-$(IMAGE_VERSION).tgz -C ./helm-chart/csi-driver .

.PHONY: generate-mock
generate-mock:
	mockgen -source=internal/driver/nodeserver_helpers.go -destination=mocks/mock_nodeserver.go -package=mocks
	mockgen -source=pkg/mount-manager/device-utils.go -destination=mocks/mock_deviceutils.go -package=mocks

## --------------------------------------
## Testing
## --------------------------------------

##@ Testing:

TEST_IMAGE_TAG ?= $(shell git rev-parse --abbrev-ref HEAD)
TEST_IMAGE_NAME ?= "linode/linode-blockstorage-csi-driver"
K8S_VERSION ?= "v1.29.1"
CAPI_VERSION ?= "v1.6.3"
HELM_VERSION ?= "v0.2.1"
CAPL_VERSION ?= "v0.3.1"

# Setting unique cluster name
TEST_CLUSTER_NAME ?= csi-driver-cluster-$(shell git rev-parse --short HEAD)

.PHONY: test-image-tags
test-image-tags:
	@echo $(TEST_IMAGE_TAG)

.PHONY: remote-cluster-deploy
remote-cluster-deploy:
	# Create a CAPL test cluster without CSI driver and wait for it to be ready
	clusterctl generate cluster $(TEST_CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) \
		--infrastructure linode-linode:$(CAPL_VERSION) \
		--flavor kubeadm-vpcless | yq 'select(.metadata.name != "$(TEST_CLUSTER_NAME)-csi-driver-linode")' | kubectl apply -f -
	kubectl wait --for=condition=ControlPlaneReady  cluster/$(TEST_CLUSTER_NAME) --timeout=600s
	clusterctl get kubeconfig $(TEST_CLUSTER_NAME) > test-cluster-kubeconfig.yaml

	# Install CSI driver and wait for it to be ready
	cat e2e/setup/linode-secret.yaml | envsubst | KUBECONFIG=test-cluster-kubeconfig.yaml kubectl apply -f -
	hack/generate-yaml.sh $(TEST_IMAGE_TAG) $(TEST_IMAGE_NAME) |KUBECONFIG=test-cluster-kubeconfig.yaml kubectl apply -f -
	KUBECONFIG=test-cluster-kubeconfig.yaml kubectl rollout status -n kube-system daemonset/csi-linode-node --timeout=600s
	KUBECONFIG=test-cluster-kubeconfig.yaml kubectl rollout status -n kube-system statefulset/csi-linode-controller --timeout=600s


.PHONY: local-deploy
local-deploy:
	ctlptl apply -f e2e/setup/ctlptl-config.yaml
	clusterctl init \
		--wait-providers \
		--wait-provider-timeout 600 \
		--core cluster-api:${CAPI_VERSION} \
		--addon helm:${HELM_VERSION} \
		--infrastructure linode-linode:${CAPL_VERSION}

.PHONY: cleanup-cluster
cleanup-cluster:
	-kubectl delete cluster $(TEST_CLUSTER_NAME)
	-kind delete cluster -n capl

.PHONY: e2e-test
e2e-test:
	KUBECONFIG=test-cluster-kubeconfig.yaml chainsaw test ./e2e/test --parallel 3
