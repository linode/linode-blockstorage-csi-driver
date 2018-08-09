#!/bin/bash
set -xeou pipefail

GOPATH=$(go env GOPATH)
REPO_ROOT="$GOPATH/src/github.com/displague/csi-linode"

export APPSCODE_ENV=prod

pushd $REPO_ROOT

rm -rf dist
./hack/docker/setup.sh
./hack/docker/setup.sh release
rm dist/.tag

popd
