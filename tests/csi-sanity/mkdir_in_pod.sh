#!/bin/sh
kubectl exec csi-linode-controller-0 -n kube-system -c csi-linode-plugin -- mktemp -d /var/lib/csi/sockets/pluginproxy/csi-sanity.XXXXXX
