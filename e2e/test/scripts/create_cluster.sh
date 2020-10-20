#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export LINODE_API_TOKEN="$1"
export CLUSTER_NAME="$2"
export K8S_VERSION="$3"

TEST_MANIFEST=$(realpath "$(dirname "$0")/../manifest/linode-blockstorage-csi-driver.yaml")

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
  default = "eu-west"
}
variable "ssh_public_key" {
  default = "${HOME}/.ssh/id_rsa.pub"
}
module "k8s" {
  source  = "git::https://github.com/linode/terraform-linode-k8s.git?ref=master"
  linode_token = "$LINODE_API_TOKEN"
  cluster_name = "$CLUSTER_NAME"
  csi_manifest = "file://${TEST_MANIFEST}"
  server_type_node = "\${var.server_type_node}"
  nodes = "\${var.nodes}"
  server_type_master = "\${var.server_type_master}"
  region = "\${var.region}"
  ssh_public_key = "\${var.ssh_public_key}"
  k8s_version = "$K8S_VERSION"
}
EOF

terraform workspace new ${CLUSTER_NAME}

terraform init

terraform apply \
 -auto-approve

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"
