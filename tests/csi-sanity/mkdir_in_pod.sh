#!/bin/sh
kubectl exec csi-linode-controller-0 -n kube-system -c linode-csi-plugin -- mktemp -d /tmp/csi-sanity.XXXXXX
