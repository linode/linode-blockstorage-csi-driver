#!/bin/bash
set -euf -o pipefail

manifest_directory=$(cd "${0%/*}/../deploy/kubernetes/sidecars"; pwd)

function fetch_manifest {
    local source_file=$1
    local target_file=$2
    printf "# xref: %s\n\n" $source_file > $target_file
    wget "$source_file" -O - >> $target_file
}

function external_provisioner {
    local version=$1
    local source_directory="https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/release-$version/deploy/kubernetes"
    local target_directory="$manifest_directory/external-provisioner"
    fetch_manifest "$source_directory/rbac.yaml" "$target_directory/rbac.yaml"
}
function external_attacher {
    local version=$1
    local source_directory="https://raw.githubusercontent.com/kubernetes-csi/external-attacher/release-$version/deploy/kubernetes"
    local target_directory="$manifest_directory/external-attacher"
    fetch_manifest "$source_directory/rbac.yaml" "$target_directory/rbac.yaml"
}

function external_resizer {
    local version=$1
    local source_directory="https://raw.githubusercontent.com/kubernetes-csi/external-resizer/v$version/deploy/kubernetes"
    local target_directory="$manifest_directory/external-resizer"
    fetch_manifest "$source_directory/rbac.yaml" "$target_directory/rbac.yaml"
}

external_provisioner "1.6"
external_attacher "2.2"
# external_snapshotter "" We don't use it?
external_resizer "0.5.0"
# node_driver_registrar "1.3.0" No manifests to fetch
