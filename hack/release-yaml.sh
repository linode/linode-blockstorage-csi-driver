#!/bin/bash

TAG=$(git describe --abbrev=0)

for a in pkg/linode-bs/deploy/kubernetes/0*; do
	echo "# $a"
	cat $a
	echo -e "\n---"
done > "pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver-${TAG}.yaml"
