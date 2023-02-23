#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

cluster_name="$1"
terraform apply -destroy -auto-approve

rm cluster.tf

rm -rf test/.terraform

if [[ -d ".terraform" ]]
then
    rm -rf .terraform
fi

if [[ -d "terraform.tfstate.d" ]]
then
    rm -rf terraform.tfstate.d
fi

