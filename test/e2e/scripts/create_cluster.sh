#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

export LINODE_API_TOKEN="$1"
export CLUSTER_NAME="$2"


cat > main.tf <<EOF
module "k8s" {
  source  = "linode/k8s/linode"
  version = "0.1.0"

  linode_token = "${LINODE_API_TOKEN}"
}
EOF

terraform workspace new ${CLUSTER_NAME}

terraform init

terraform apply \
 -var region=eu-west \
 -var server_type_master=g6-standard-2 \
 -var nodes=2 \
 -var server_type_node=g6-standard-2 \
 -auto-approve

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"