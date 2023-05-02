#!/bin/bash

## Must be run from the root of the repo

UDS="/tmp/e2e-csi-sanity.sock"
CSI_ENDPOINT="unix://${UDS}"
CSI_MOUNTPOINT="/mnt"
APP=linode

SKIP="WithCapacity"

# Get csi-sanity
./hack/get-sanity.sh

# Build
make linode

# Cleanup
rm -f $UDS

# Start the application in the background
sudo _output/$APP --endpoint=$CSI_ENDPOINT --nodeid=1 &
pid=$!

# Need to skip Capacity testing since linode does not support it
sudo $GOPATH/bin/csi-sanity $@ \
    --ginkgo.skip=${SKIP} \
    --csi.mountdir=$CSI_MOUNTPOINT \
    --csi.endpoint=$CSI_ENDPOINT ; ret=$?
sudo kill -9 $pid
sudo rm -f $UDS

if [ $ret -ne 0 ] ; then
	exit $ret
fi

exit 0
