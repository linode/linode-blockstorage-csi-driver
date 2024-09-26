#!/bin/sh
kubectl exec csi-linode-controller-0 -n kube-system -c csi-linode-plugin -- rmdir "$@"
