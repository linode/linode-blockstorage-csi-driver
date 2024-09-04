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
./${OUT_DIR}/kubernetes/test/bin/e2e.test                   `# runs kubernetes e2e tests` \
    --ginkgo.vv                                             `# enables verbose output` \
    --ginkgo.focus='External.Storage'                       `# only run external storage tests` \
    --ginkgo.skip='\[Disruptive\]'                          `# skip disruptive tests as they need ssh access to nodes` \
    --ginkgo.skip='volume-expand'                           `# skip volume-expand as its done manually for now` \
    --ginkgo.skip='snapshottable'                           `# skip as we don't support snapshots` \
    --ginkgo.skip='snapshottable-stress'                    `# skip as we don't support snapshots` \
    --ginkgo.skip='\[Feature:VolumeSnapshotDataSource\]'    `# skip as we don't support snapshots` \
    --ginkgo.skip='\[Feature:Windows\]'                     `# skip as we don't support windows` \
    --ginkgo.flake-attempts=3                               `# retry 3 times for flaky tests` \
    --ginkgo.timeout=2h                                     `# tests can run for max 2 hours` \
    -storage.testdriver=tests/upstream-e2e/test-driver.yaml `# configuration file for storage driver capabilities`

# Remove downloaded files and binaries
rm -rf ${OUT_DIR}
