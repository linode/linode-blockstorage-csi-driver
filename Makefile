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

REGISTRY_NAME=hub.docker.com/linode
IMAGE_NAME=linode-blockstorage-csi-driver
IMAGE_VERSION=canary
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)
REV=$(shell git describe --long --tags --dirty)

.PHONY: all linode clean linode-container

all: linode

test:
	go test -v ./... -cover
	go vet ./...
vendor: 
	@GO111MODULE=on go mod vendor
linode: vendor
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X github.com/linode/linode-blockstorage-csi-driver/pkg/hostpath.vendorVersion=$(REV) -extldflags "-static"' -o _output/linode ./app/linode
linode-container: linode
	docker build -t $(IMAGE_TAG) -f ./app/linode/Dockerfile .
push: linode-container
	@echo "$$DOCKER_PASSWORD" | docker login -u "$$DOCKER_USERNAME" --password-stdin
	docker push $(IMAGE_TAG)
verify:
	@GO111MODULE=on go mod verify
clean:
	@GOOS=linux go clean -i -r -x ./...
	-rm -rf _output
