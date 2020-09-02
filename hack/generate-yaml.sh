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

# Get the last manifest in the folder
cd $(dirname "$0")/../
manifests=pkg/linode-bs/deploy/kubernetes/0
last="$(ls -dq "${manifests}"* | tail -n 1)"

# Build release manifest
for manifest in "${manifests}"*; do
    echo "# ${manifest}"
    echo "$(cat ${manifest})" | sed -e "s|{{ .Values.image.tag }}|"${TAG}"|"

    # Don't add the separator if it's the last manifest
    if [[ "${manifest}" != "${last}" ]]; then
        echo -e "---"
    fi
done
