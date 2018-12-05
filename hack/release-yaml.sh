#!/bin/bash

TAG=${1?$(git describe --abbrev=0)}
RELEASES="pkg/linode-bs/deploy/releases/"
TAGGED_RELEASE="linode-blockstorage-csi-driver-${TAG}.yaml"
GENERIC_RELEASE="linode-blockstorage-csi-driver.yaml"

for manifest in pkg/linode-bs/deploy/kubernetes/0*; do
	echo "# $manifest"
	sed -e "s|{{ .Values.image.tag }}|${TAG}|" "$manifest"
	echo -e "\n---"
done > "$RELEASES/$TAGGED_RELEASE"
ln -fs "$TAGGED_RELEASE" "$RELEASES/$GENERIC_RELEASE"
