#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export LINODE_TOKEN="$1"
export CLUSTER_NAME="$2"
export KUBERNETES_VERSION="$3"
export WORKER_MACHINE_COUNT=2
export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
export LINODE_MACHINE_TYPE=g6-standard-2
export CAPI_VERSION="v1.6.3"
export HELM_VERSION="v0.1.1-alpha.1"
export CAPLI_VERSION="0.1.0"
export KUBECONFIG="$(pwd)/${CLUSTER_NAME}-management.conf"

if [[ -z "$4" ]]
then
  export LINODE_REGION="us-sea"
else
  export LINODE_REGION="$4"
fi

METADATA_REGIONS="nl-ams in-maa us-ord id-cgk us-lax es-mad us-mia it-mil jp-osa fr-par br-gru us-sea se-sto us-iad"
[[ "$METADATA_REGIONS" =~ .*"${LINODE_REGION}".* ]] || (echo "Given region doesn't support Metadata service" ; exit 1)

prepare_images() {
  local images="$(echo "$1" | grep -e "^[[:space:]]*image:[?[:space:]]" | awk '{print $2}')"

  echo "${images//[\'\"]}" | xargs -I {} sh -c 'docker pull '{}' ; kind -n '${CLUSTER_NAME}' load docker-image '{}
}

cat <<EOF | ctlptl apply -f -
apiVersion: ctlptl.dev/v1alpha1
kind: Cluster
product: kind
kindV1Alpha4Cluster:
  name: ${CLUSTER_NAME}
  nodes:
    - role: control-plane
      image: kindest/node:${KUBERNETES_VERSION}
EOF

prepare_images "$(cat $(realpath "$(dirname "$0")/infrastructure-linode/${CAPLI_VERSION}/infrastructure-components.yaml"))"
prepare_images "$(clusterctl init list-images \
  --core cluster-api:${CAPI_VERSION} \
  --addon helm:${HELM_VERSION} \
  | xargs -I {} echo 'image: '{})"

clusterctl init \
  --wait-providers \
  --core cluster-api:${CAPI_VERSION} \
  --addon helm:${HELM_VERSION} \
  --infrastructure linode:${CAPLI_VERSION} \
  --config $(realpath "$(dirname "$0")/clusterctl.yaml")

clusterctl generate cluster ${CLUSTER_NAME} \
  --flavor clusterclass-kubeadm \
  --config $(realpath "$(dirname "$0")/clusterctl.yaml") \
  | kubectl apply --wait -f -

c=8
until kubectl get secret ${CLUSTER_NAME}-kubeconfig; do
  sleep $(((c--)))
done

kubectl get secret ${CLUSTER_NAME}-kubeconfig -o jsonpath="{.data.value}" \
  | base64 --decode \
  > "$(pwd)/${CLUSTER_NAME}.conf"

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"

c=16
until kubectl version; do
  sleep $(((c--)))
done

kubectl apply -f $(realpath "$(dirname "$0")/../manifest/linode-blockstorage-csi-driver.yaml")

# For backward compatibility
export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"
