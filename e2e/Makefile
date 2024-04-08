export GO111MODULE=on

IMAGE_VERSION?=canary
K8S_VERSION?=""
GINKGO_PATH := $(shell go env GOPATH)/bin/ginkgo

.PHONY: test clean check-token install-dependencies test-manifest

test-manifest:
	../hack/generate-yaml.sh $(IMAGE_VERSION) > test/manifest/linode-blockstorage-csi-driver.yaml

${GINKGO_PATH}:
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.13.2

test-cluster: clean-cluster
	./test/scripts/create_management_cluster.sh $(K8S_VERSION)

clean-cluster:
	ctlptl delete cluster kind-management --ignore-not-found
	rm -rf ./test/kind-management.conf

test: ${GINKGO_PATH} check-token test-manifest test-cluster
	go list -m; \
	ginkgo -r --vv --procs=6 --trace --cover $(TEST_ARGS) -- --v=3 --k8s-version=${K8S_VERSION} $(SUITE_ARGS)

	$(MAKE) clean-cluster

clean: check-token
	cd test; \
	./scripts/delete_cluster.sh csi-linode-for-reuse

check-token:
	@if test "$(LINODE_API_TOKEN)" = "" ; then \
	  echo "LINODE_API_TOKEN must be set"; \
	  exit 1; \
	fi

install-dependencies:
	sudo apt-get install curl
	curl -fsSL https://github.com/tilt-dev/ctlptl/releases/download/v0.8.28/ctlptl.0.8.28.linux.x86_64.tar.gz | sudo tar -xzv -C /usr/local/bin ctlptl
	chmod +x /usr/local/bin/ctlptl
	curl -fsSL https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.6.3/clusterctl-linux-amd64 | sudo cat > /usr/local/bin/clusterctl
	chmod +x /usr/local/bin/clusterctl

