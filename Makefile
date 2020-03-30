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

REGISTRY_NAME=index.docker.io/linode
IMAGE_NAME=linode-blockstorage-csi-driver
IMAGE_VERSION=canary
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)
REV=$(shell git describe --long --tags --dirty)

export GO111MODULE=on

.PHONY: all
all: linode

.PHONY: vendor
vendor:
	go mod vendor

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet: fmt
	go vet ./...

.PHONY: test
test: vet
	go test -v ./... -cover

.PHONY: linode
linode: vendor test
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.vendorVersion=$(REV) -extldflags "-static"' -o _output/linode ./app/linode

.PHONY: linode-container
linode-container: linode
	docker build -t $(IMAGE_TAG) -f ./app/linode/Dockerfile .

.PHONY: push
push: linode-container
	docker push $(IMAGE_TAG)

.PHONY: verify
verify:
	go mod verify

.PHONY: clean
clean:
	@GOOS=linux go clean -i -r -x ./...
	-rm -rf _output
