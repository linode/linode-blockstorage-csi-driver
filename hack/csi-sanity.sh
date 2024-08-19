#!/bin/bash

# Define the CSI endpoint for the sanity tests
CSI_ENDPOINT="dns:///127.0.0.1:10000"

# Define the scripts for creating and deleting directories in the pod
CREATE_DIRECTORY="./tests/csi-sanity/mkdir_in_pod.sh"
DELETE_DIRECTORY="./tests/csi-sanity/rmdir_in_pod.sh"

# Define the list of tests to skip as an array
SKIP_TESTS=(
  "WithCapacity"
  # Need to skip it because we do not support volume snapshots
  "should fail when the volume source volume is not found" 
  # This case fails because we currently do not support read only volume creation on the linode side
  # but we are supporting it in the CSI driver by mounting the volume as read only
  "should fail when the volume is already published but is incompatible"
)

# Join the array into a single string with '|' as the separator
SKIP_TESTS_STRING=$(IFS='|'; echo "${SKIP_TESTS[*]}")

# Install the latest version of csi-sanity
go install github.com/kubernetes-csi/csi-test/v5/cmd/csi-sanity@latest

# Create socat statefulset
kubectl apply -f tests/csi-sanity/socat.yaml

# Wait for pod to be ready
kubectl wait --for=condition=ready --timeout=60s pods/csi-socat-0

# Start the port forwarding in the background and log output to a file
nohup kubectl port-forward pods/csi-socat-0 10000:10000 > port-forward.log 2>&1 &

# Run the csi-sanity tests with the specified parameters
csi-sanity --ginkgo.vv --ginkgo.trace --ginkgo.skip "$SKIP_TESTS_STRING" --csi.endpoint="$CSI_ENDPOINT" --csi.createstagingpathcmd="$CREATE_DIRECTORY" --csi.createmountpathcmd="$CREATE_DIRECTORY" --csi.removestagingpathcmd="$DELETE_DIRECTORY" --csi.removemountpathcmd="$DELETE_DIRECTORY"

# Find the process ID (PID) of the kubectl port-forward command using the specified port
PID=$(lsof -t -i :10000 -sTCP:LISTEN)

# Check if a PID was found and kill the process if it exists
if [ -z "$PID" ]; then
  echo "No process found on port 10000."
else
  kill -9 "$PID"
  echo "Process on port 10000 with PID $PID has been killed."
fi

# Remove the socat statefulset
kubectl delete -f tests/csi-sanity/socat.yaml
