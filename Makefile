# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PLATFORM       ?= linux/amd64
REGISTRY_NAME  ?= index.docker.io/linode
IMAGE_NAME     ?= linode-blockstorage-csi-driver
REV            := $(shell git describe --long --tags --dirty)
IMAGE_VERSION  ?= $(REV)
IMAGE_TAG      ?= $(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)

export GO111MODULE=on

.PHONY: fmt
# TODO(PR): does this actually check or just format?
fmt:
	go fmt ./...

.PHONY: vet
vet: fmt
	go vet ./...

.PHONY: test
test: vet verify
	go test -v ./... -cover $(TEST_ARGS)

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build --platform=$(PLATFORM) --progress=plain -t $(IMAGE_TAG) --build-arg REV=$(REV) -f ./app/linode/Dockerfile .

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
