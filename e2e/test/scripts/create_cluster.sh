#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export LINODE_TOKEN="$1"
export CLUSTER_NAME="$2"
export KUBERNETES_VERSION="$3"
export CAPL_VERSION="0.1.0"
export WORKER_MACHINE_COUNT=1
export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
export LINODE_MACHINE_TYPE=g6-standard-2
export KUBECONFIG="$(realpath "$(dirname "$0")/../kind-management.conf")"

if [[ -z "$4" ]]
then
  export LINODE_REGION="eu-west"
else
  export LINODE_REGION="$4"
fi

kubectl create ns ${CLUSTER_NAME} ||:
(cd $(realpath "$(dirname "$0")"); clusterctl generate cluster ${CLUSTER_NAME} \
  --target-namespace ${CLUSTER_NAME} \
  --flavor clusterclass-kubeadm \
  --config clusterctl.yaml \
  | kubectl apply --wait -f -)

c=8
until kubectl get secret -n ${CLUSTER_NAME} ${CLUSTER_NAME}-kubeconfig; do
  sleep $(((c--)))
done

clusterctl get kubeconfig -n ${CLUSTER_NAME} ${CLUSTER_NAME} > "$(pwd)/${CLUSTER_NAME}.conf"

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"

c=16
until kubectl version; do
  sleep $(((c--)))
done

(set +x ; kubectl create secret generic -n kube-system linode --from-literal="token=${LINODE_TOKEN}")

kubectl apply -f $(realpath "$(dirname "$0")/../manifest/linode-blockstorage-csi-driver.yaml")

# For backward compatibility
export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"
