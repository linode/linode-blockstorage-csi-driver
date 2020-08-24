export GO111MODULE=on

REGISTRY_NAME=index.docker.io/linode
IMAGE_NAME=linode-blockstorage-csi-driver
IMAGE_VERSION=canary
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)

.PHONY: test reuse-and-test clean check-token install-terraform test-manifest

test-manifest:
	../hack/generate-yaml.sh $(IMAGE_VERSION) > test/manifest/linode-blockstorage-csi-driver.yaml

reuse-and-test: SUITE_ARGS='--reuse'

test reuse-and-test: $(GOPATH)/bin/ginkgo check-token test-manifest
	go list -m; \
	ginkgo -r --v --progress --trace --cover $(TEST_ARGS) -- --v=3 --image=${IMAGE_TAG} $(SUITE_ARGS)

clean: check-token
	cd test; \
	./scripts/delete_cluster.sh csi-linode-for-reuse; \
	rm terraform.tfstate; \
	rm -rf terraform.tfstate.d

check-token:
	@if test "$(LINODE_API_TOKEN)" = "" ; then \
	  echo "LINODE_API_TOKEN must be set"; \
	  exit 1; \
	fi

install-terraform:
	sudo apt-get install wget unzip
	wget https://releases.hashicorp.com/terraform/0.11.13/terraform_0.11.13_linux_amd64.zip
	unzip terraform_0.11.13_linux_amd64.zip
	sudo mv terraform /usr/local/bin/
