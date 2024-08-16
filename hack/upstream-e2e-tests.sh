#!/bin/bash
set -euf -o pipefail

URL="https://dl.k8s.io/release/${K8S_VERSION}/kubernetes-test-${OS}-${ARCH}.tar.gz"

# output dir where downloaded files will be stored
OUT_DIR="./binaries"
mkdir -p ${OUT_DIR}
OUT_TAR="${OUT_DIR}/k8s.tar.gz"

# Download k8s test tar archive
curl -L ${URL} -o ${OUT_TAR}
tar xzvf ${OUT_TAR} -C ${OUT_DIR}

# Run k8s e2e tests for storage driver
./${OUT_DIR}/kubernetes/test/bin/e2e.test \
    -ginkgo.v \
    -ginkgo.focus='External.Storage' \
    --ginkgo.skip='disruptive' \
    --ginkgo.skip='ephemeral' \
    --ginkgo.skip='volume-expand' \
    --ginkgo.skip='multiVolume' \
    --ginkgo.skip='snapshottable' \
    --ginkgo.skip='snapshottable-stress' \
    --ginkgo.skip='\[Feature:VolumeSnapshotDataSource\]' \
    --ginkgo.skip='\[Feature:Windows\]' \
    --ginkgo.flake-attempts=3 \
    --ginkgo.timeout=2h \
    -storage.testdriver=tests/upstream-e2e/test-driver.yaml

# Remove downloaded files and binaries
rm -rf ${OUT_DIR}
