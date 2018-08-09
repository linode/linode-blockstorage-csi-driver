#!/usr/bin/env bash

pushd $GOPATH/src/github.com/displague/csi-linode/hack/gendocs
go run main.go
popd
