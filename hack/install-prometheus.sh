#!/bin/bash

set -euf -o pipefail

# Default Values
DEFAULT_DATA_RETENTION_PERIOD="15d"

# Configuration Variables (Environment Variables)
DATA_RETENTION_PERIOD=${DATA_RETENTION_PERIOD:-$DEFAULT_DATA_RETENTION_PERIOD}
NAMESPACE="monitoring"

# Determine if worker nodes exist
echo "Checking for worker nodes..."

# Get all nodes
ALL_NODES=$(kubectl get nodes --no-headers -o custom-columns=NAME:.metadata.name)

# Get control-plane nodes (assuming 'control-plane' in the name)
CONTROL_PLANE_NODES=$(echo "${ALL_NODES}" | grep "control-plane" || true)

# Get worker nodes (nodes not containing 'control-plane' in the name)
WORKER_NODES=$(echo "${ALL_NODES}" | grep -v "control-plane" || true)

if [ -z "${WORKER_NODES}" ]; then
  echo "No worker nodes found. Untainting control-plane node(s) to allow scheduling of Prometheus pods..."
  for NODE in ${CONTROL_PLANE_NODES}; do
    echo "Untainting node: ${NODE}"
    kubectl taint nodes "${NODE}" node-role.kubernetes.io/control-plane:NoSchedule- || true
  done
else
  echo "Worker nodes detected. Prometheus will be installed on them."
fi

# Add Helm repositories if not already added
echo "Adding Helm repository for Prometheus..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts || true

# Update Helm repositories
echo "Updating Helm repositories..."
helm repo update

# Create a namespace for monitoring tools
echo "Creating namespace '${NAMESPACE}'..."
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

# Install or Upgrade Prometheus
echo "Installing or upgrading Prometheus..."
helm upgrade --install prometheus prometheus-community/prometheus \
  --namespace ${NAMESPACE} \
  --set server.persistentVolume.enabled=false \
  --set server.retention="${DATA_RETENTION_PERIOD}" \
  --wait \
  --timeout=10m0s \
  --debug

echo "Prometheus installation and configuration completed successfully."

exit 0
