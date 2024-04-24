#!/bin/bash -e
set -o pipefail
# Generate manifests for deployment on Kubernetes

# A tag name _must_ be supplied as the first argument
TAG="${1}"
if [[ -z "${TAG}" ]]; then
    echo "Tag name to release must be supplied as the first argument"
    echo "e.g. $ hack/release-yaml.sh v1.0.0"
    exit 1
fi

# An image name may be supplied as the second argument in order to pass a custom image name
IMAGE_NAME=${2:-"linode/linode-blockstorage-csi-driver"}
if [[ -z "${2}" ]]; then
    echo "Image name not supplied" >&2
    echo "default to $ hack/release-yaml.sh ${TAG} linode/linode-blockstorage-csi-driver" >&2
fi

cd $(dirname "$0")/../
file=./deploy/kubernetes/overlays/release/kustomization.yaml
CSI_VERSION=$TAG CSI_IMAGE_NAME=$IMAGE_NAME envsubst < "$file.template" > $file

kustomize build "$(dirname $file)"
