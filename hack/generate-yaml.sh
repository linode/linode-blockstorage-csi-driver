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

cd $(dirname "$0")/../
file=./deploy/kubernetes/overlays/release/kustomization.yaml
CSI_VERSION=$TAG envsubst < "$file.template" > $file

kustomize build "$(dirname $file)"
