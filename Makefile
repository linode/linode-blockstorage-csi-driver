PLATFORM                ?= linux/amd64
REGISTRY_NAME           ?= index.docker.io
DOCKER_USER             ?= linode
IMAGE_NAME              ?= linode-blockstorage-csi-driver
REV                     := $(shell git branch --show-current 2> /dev/null || echo "dev")
ifdef DEV_TAG_EXTENSION
IMAGE_VERSION           ?= $(REV)-$(DEV_TAG_EXTENSION)
else
IMAGE_VERSION           ?= $(REV)
endif
IMAGE_TAG               ?= $(REGISTRY_NAME)/$(DOCKER_USER)/$(IMAGE_NAME):$(IMAGE_VERSION)
GOLANGCI_LINT_IMG       := golangci/golangci-lint:v1.59-alpine
RELEASE_DIR             ?= release
DOCKERFILE              ?= Dockerfile
GOLANGCI_LINT_VERSION   ?= v1.61.0
E2E_SELECTOR            ?= all
LINODE_FIREWALL_ENABLED ?= true

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
	docker run --rm -w /workdir -v $(PWD):/workdir --platform=$(PLATFORM) -it $(IMAGE_TAG) go fmt ./...

.PHONY: vet
vet: fmt
	docker run --rm -w /workdir -v $(PWD):/workdir --platform=$(PLATFORM) -it $(IMAGE_TAG) go vet ./...

.PHONY: lint
lint: vet
	docker run --rm -w /workdir -v $(PWD):/workdir --platform=$(PLATFORM) -it $(IMAGE_TAG) golangci-lint run -v -c .golangci.yml --fix

.PHONY: verify
verify:
	docker run --rm --platform=$(PLATFORM) -it $(IMAGE_TAG) go mod verify

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
CAPI_VERSION         ?= "v1.8.5"
HELM_VERSION         ?= "v0.2.1"
CAPL_VERSION         ?= "v0.7.1"
CONTROLPLANE_NODES   ?= 1
WORKER_NODES         ?= 1
GRAFANA_PORT ?= 3000
GRAFANA_USERNAME ?= admin
GRAFANA_PASSWORD ?= admin
DATA_RETENTION_PERIOD ?= 15d  # Prometheus data retention period
KUBECONFIG ?= test-cluster-kubeconfig.yaml

.PHONY: build
build:
	CGO_ENABLED=1 go build -o linode-blockstorage-csi-driver -a -ldflags '-X main.vendorVersion='${IMAGE_VERSION}'' ./main.go

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build --platform=$(PLATFORM) --progress=plain \
		-t $(IMAGE_TAG) \
		--build-arg REV=$(IMAGE_VERSION) \
		--build-arg GOLANGCI_LINT_VERSION=$(GOLANGCI_LINT_VERSION) \
		-f ./$(DOCKERFILE) .

.PHONY: docker-push
docker-push:
	docker push $(IMAGE_TAG)

.PHONY: docker-setup
docker-setup: docker-build docker-push

.PHONY: mgmt-and-capl-cluster
mgmt-and-capl-cluster: docker-setup mgmt-cluster capl-cluster

.PHONY: capl-cluster
capl-cluster: generate-capl-cluster-manifests create-capl-cluster generate-csi-driver-manifests install-csi

.PHONY: generate-capl-cluster-manifests
generate-capl-cluster-manifests:
	# Create the CAPL cluster manifests without any CSI driver stuff
	LINODE_FIREWALL_ENABLED=$(LINODE_FIREWALL_ENABLED) clusterctl generate cluster $(CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) \
		--infrastructure linode-linode:$(CAPL_VERSION) \
		--control-plane-machine-count $(CONTROLPLANE_NODES) --worker-machine-count $(WORKER_NODES) \
		| yq 'select(.metadata.name != "$(CLUSTER_NAME)-csi-driver-linode")' > capl-cluster-manifests.yaml

.PHONY: create-capl-cluster
create-capl-cluster:
	# Create a CAPL cluster without CSI driver and wait for it to be ready
	kubectl apply -f capl-cluster-manifests.yaml
	kubectl wait --for=condition=ControlPlaneReady cluster/$(CLUSTER_NAME) --timeout=600s || (kubectl get cluster -o yaml; kubectl get linodecluster -o yaml; kubectl get linodemachines -o yaml)
	kubectl wait --for=condition=NodeHealthy=true machines -l cluster.x-k8s.io/cluster-name=$(CLUSTER_NAME) --timeout=900s
	clusterctl get kubeconfig $(CLUSTER_NAME) > test-cluster-kubeconfig.yaml
	KUBECONFIG=$(KUBECONFIG) kubectl wait --for=condition=Ready nodes --all --timeout=600s
	cat tests/e2e/setup/linode-secret.yaml | envsubst | KUBECONFIG=$(KUBECONFIG) kubectl apply -f -

.PHONY: generate-csi-driver-manifests
generate-csi-driver-manifests:
	hack/generate-yaml.sh $(IMAGE_VERSION) $(DOCKER_USER)/$(IMAGE_NAME) > csi-manifests.yaml

.PHONY: install-csi
install-csi:
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f csi-manifests.yaml
	KUBECONFIG=$(KUBECONFIG) kubectl rollout status -n kube-system daemonset/csi-linode-node --timeout=600s
	KUBECONFIG=$(KUBECONFIG) kubectl rollout status -n kube-system statefulset/csi-linode-controller --timeout=600s

.PHONY: mgmt-cluster
mgmt-cluster:
	# Create a mgmt cluster
	ctlptl apply -f tests/e2e/setup/ctlptl-config.yaml
	clusterctl init \
		--wait-providers \
		--wait-provider-timeout 600 \
		--core cluster-api:${CAPI_VERSION} \
		--addon helm:${HELM_VERSION} \
		--bootstrap kubeadm:$(CAPI_VERSION) \
		--control-plane kubeadm:$(CAPI_VERSION) \
		--infrastructure linode-linode:${CAPL_VERSION}

.PHONY: cleanup-cluster
cleanup-cluster:
	-kubectl delete cluster --all
	-kubectl delete linodefirewalls --all
	-kubectl delete lvpc --all
	-kind delete cluster -n capl
	-rm -f luks.key

#####################################################################
# Test Setup
#####################################################################

.PHONY: generate-mock
generate-mock:
	mockgen -source=pkg/mount-manager/safe_mounter.go -destination=mocks/mock_safe-mounter.go -package=mocks
	mockgen -source=pkg/device-manager/device.go -destination=mocks/mock_device.go -package=mocks
	mockgen -source=pkg/filesystem/filesystem.go -destination=mocks/mock_filesystem.go -package=mocks
	mockgen -source=pkg/linode-client/linode_client.go -destination=mocks/mock_linodeclient.go -package=mocks
	mockgen -source=pkg/cryptsetup-client/cryptsetup_client.go -destination=mocks/mock_cryptsetupclient.go -package=mocks
	mockgen -source=internal/driver/metadata.go -destination=mocks/mock_metadata.go -package=mocks
	mockgen -source=pkg/hwinfo/hwinfo.go -destination=mocks/mock_hwinfo.go -package=mocks

.PHONY: test
test:
	docker run --rm --platform=$(PLATFORM) --privileged -it $(IMAGE_TAG) go test `go list ./... | grep -v ./mocks$$` -cover $(TEST_ARGS)

.PHONY: e2e-test
e2e-test:
	openssl rand -out luks.key 64
	KUBECONFIG=$(KUBECONFIG) LUKS_KEY=$$(base64 luks.key | tr -d '\n') chainsaw test ./tests/e2e --parallel 2 --selector $(E2E_SELECTOR)

.PHONY: csi-sanity-test
csi-sanity-test:
	KUBECONFIG=$(KUBECONFIG) ./tests/csi-sanity/run-tests.sh

.PHONY: upstream-e2e-tests
upstream-e2e-tests:
	OS=$(OS) ARCH=$(ARCH_SHORT) K8S_VERSION=$(K8S_VERSION) KUBECONFIG=$(KUBECONFIG) ./tests/upstream-e2e/run-tests.sh

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

#####################################################################
# Grafana Dashboard End to End Installation
#####################################################################
.PHONY: grafana-dashboard
grafana-dashboard: install-prometheus install-grafana setup-dashboard

#####################################################################
# Monitoring Tools Installation
#####################################################################
.PHONY: install-prometheus
install-prometheus:
	KUBECONFIG=$(KUBECONFIG) DATA_RETENTION_PERIOD=$(DATA_RETENTION_PERIOD) \
		./hack/install-prometheus.sh --timeout=600s

.PHONY: install-grafana
install-grafana:
	KUBECONFIG=$(KUBECONFIG) GRAFANA_PORT=$(GRAFANA_PORT) \
		GRAFANA_USERNAME=$(GRAFANA_USERNAME) GRAFANA_PASSWORD=$(GRAFANA_PASSWORD) \
		./hack/install-grafana.sh --timeout=600s

.PHONY: setup-dashboard
setup-dashboard:
	KUBECONFIG=$(KUBECONFIG) ./hack/setup-dashboard.sh --namespace=monitoring --dashboard-file=observability/metrics/dashboard.json

.PHONY: setup-tracing
setup-tracing:
	KUBECONFIG=$(KUBECONFIG) ./hack/setup-tracing.sh

.PHONY: lke-test
lke-test:
	# Set temporary image tag using ttl.sh
	$(eval DEV_TAG_EXTENSION := $(shell git rev-parse --short HEAD))
	$(eval TEMP_IMAGE_TAG := ttl.sh/$(IMAGE_NAME)-$(IMAGE_VERSION)-$(DEV_TAG_EXTENSION):1h)

	# Build and push the image with the temporary tag
	IMAGE_TAG=$(TEMP_IMAGE_TAG) $(MAKE) docker-build docker-push

	# Update the image in the DaemonSet and StatefulSet
	kubectl -n kube-system set image ds csi-linode-node csi-linode-plugin=$(TEMP_IMAGE_TAG)
	kubectl -n kube-system set image sts csi-linode-controller csi-linode-plugin=$(TEMP_IMAGE_TAG)
