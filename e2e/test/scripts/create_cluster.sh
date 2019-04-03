#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export LINODE_API_TOKEN=d62a569581c2a74ba0a386fe682fcba88cae40c5043f4f8611d843655dbd5931
export CLUSTER_NAME="$2"


cat > cluster.tf <<EOF
variable "server_type_node" {
  default = "g6-standard-2"
}
variable "nodes" {
  default = 2
}
variable "server_type_master" {
  default = "g6-standard-2"
}
variable "region" {
  default = ""
}
variable "ssh_public_key" {
  default = "${HOME}/.ssh/id_rsa.pub"
}
module "k8s" {
  source  = "git::https://github.com/linode/terraform-linode-k8s.git?ref=for-cli"
  linode_token = "$LINODE_API_TOKEN"
  linode_group = "$CLUSTER_NAME"
  server_type_node = "\${var.server_type_node}"
  nodes = "\${var.nodes}"
  server_type_master = "\${var.server_type_master}"
  region = "\${var.region}"
  ssh_public_key = "\${var.ssh_public_key}"
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