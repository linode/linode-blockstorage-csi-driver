VERSION ?= v0.0.1
NAME=linode-csi-plugin
DOCKER_ORG=displague

all: publish

# TODO add push back
# publish: compile verify test build push clean
publish: compile verify test build clean

$(GOPATH)/bin/dep:
	@go get -u github.com/golang/dep/cmd/dep

vendor: $(GOPATH)/bin/dep
	@dep ensure

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${NAME}

build:
	@echo "==> Building the docker image"
	@docker build -t $(DOCKER_ORG)/$(NAME):$(VERSION) .

push:
	@echo "==> Publishing $(DOCKER_ORG)/$(NAME):$(VERSION)"
	@echo "$$DOCKER_PASSWORD" | docker login -u "$$DOCKER_USERNAME" --password-stdin
	@docker push $(DOCKER_ORG)/$(NAME):$(VERSION)
	@echo "==> Your image is now available at $(DOCKER_ORG)/$(NAME):$(VERSION)"

test:
	go test -v ./...

verify:
	# vendor/github.com/kubernetes/repo-infra/verify/verify-boilerplate.sh --rootdir=${CURDIR}
	vendor/github.com/kubernetes/repo-infra/verify/verify-go-src.sh -v --rootdir ${CURDIR}

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: all push fetch build-image clean verify test
