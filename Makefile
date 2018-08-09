VERSION ?= v0.0.1
NAME=linode-csi-plugin

all: publish

publish: compile build push clean

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${NAME}

build:
	@echo "==> Building the docker image"
	@docker build -t displague/linode-csi-plugin:$(VERSION) .

push:
	@echo "==> Publishing displague/linode-csi-plugin:$(VERSION)"
	@docker push displague/linode-csi-plugin:$(VERSION)
	@echo "==> Your image is now available at displague/linode-csi-plugin:$(VERSION)"

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: all push fetch build-image clean
