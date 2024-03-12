export GO111MODULE=on

IMAGE_VERSION=canary
K8S_VERSION?=""
GINKGO_PATH := $(shell go env GOPATH)/bin/ginkgo

.PHONY: test clean check-token test-manifest

test-manifest:
	../hack/generate-yaml.sh $(IMAGE_VERSION) > test/manifest/linode-blockstorage-csi-driver.yaml

${GINKGO_PATH}:
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.13.2

test: ${GINKGO_PATH} check-token test-manifest
	go list -m; \
	ginkgo -r --vv --procs=6 --trace --cover $(TEST_ARGS) -- --k8s-version=${K8S_VERSION} $(SUITE_ARGS)

clean: check-token
	cd test; \
	./scripts/delete_cluster.sh csi-linode-for-reuse

check-token:
	@if test "$(LINODE_API_TOKEN)" = "" ; then \
	  echo "LINODE_API_TOKEN must be set"; \
	  exit 1; \
	fi
