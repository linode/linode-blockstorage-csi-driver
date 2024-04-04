#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export CLUSTER_NAME="$1"
export KUBECONFIG="$(pwd)/${CLUSTER_NAME}-management.conf"

timeout 5m kubectl delete linodemachine --all --wait
timeout 5m kubectl delete linodecluster --all --wait
timeout 5m kind delete cluster -n ${CLUSTER_NAME}
