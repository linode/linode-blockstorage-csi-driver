#!/bin/bash

set -euf -o pipefail

# Default Values
DEFAULT_GRAFANA_PORT=3000
DEFAULT_GRAFANA_USERNAME="admin"
DEFAULT_GRAFANA_PASSWORD="admin"

# Configuration Variables (Environment Variables)
GRAFANA_PORT=${GRAFANA_PORT:-$DEFAULT_GRAFANA_PORT}
GRAFANA_USERNAME=${GRAFANA_USERNAME:-$DEFAULT_GRAFANA_USERNAME}
GRAFANA_PASSWORD=${GRAFANA_PASSWORD:-$DEFAULT_GRAFANA_PASSWORD}
NAMESPACE="monitoring"

# Validate Grafana Port
if ! [[ "$GRAFANA_PORT" =~ ^[0-9]+$ ]]; then
  echo "Error: Grafana port must be a number."
  exit 1
fi

# Determine if worker nodes exist
echo "Checking for worker nodes..."

# Get all nodes
ALL_NODES=$(kubectl get nodes --no-headers -o custom-columns=NAME:.metadata.name)

# Get control-plane nodes (assuming 'control-plane' in the name)
CONTROL_PLANE_NODES=$(echo "${ALL_NODES}" | grep "control-plane" || true)

# Get worker nodes (nodes not containing 'control-plane' in the name)
WORKER_NODES=$(echo "${ALL_NODES}" | grep -v "control-plane" || true)

if [ -z "${WORKER_NODES}" ]; then
  echo "No worker nodes found. Untainting control-plane node(s) to allow scheduling of Grafana pods..."
  for NODE in ${CONTROL_PLANE_NODES}; do
    echo "Untainting node: ${NODE}"
    kubectl taint nodes "${NODE}" node-role.kubernetes.io/control-plane:NoSchedule- || true
  done
else
  echo "Worker nodes detected. Grafana will be installed on them."
fi

# Add Helm repositories if not already added
echo "Adding Helm repository for Grafana..."
helm repo add grafana https://grafana.github.io/helm-charts || true

# Update Helm repositories
echo "Updating Helm repositories..."
helm repo update

# Install or Upgrade Grafana with Prometheus data source
echo "Installing or upgrading Grafana with Prometheus data source..."
helm upgrade --install grafana grafana/grafana \
  --namespace ${NAMESPACE} \
  --set adminUser="${GRAFANA_USERNAME}" \
  --set adminPassword="${GRAFANA_PASSWORD}" \
  --set service.type=NodePort \
  --set service.port="${GRAFANA_PORT}" \
  --set sidecar.dashboards.enabled=true \
  --set sidecar.dashboards.label=grafana_dashboard \
  --set datasources."datasources\.yaml".apiVersion=1 \
  --set datasources."datasources\.yaml".datasources[0].name=Prometheus \
  --set datasources."datasources\.yaml".datasources[0].type=prometheus \
  --set datasources."datasources\.yaml".datasources[0].url=http://prometheus-server.${NAMESPACE}.svc.cluster.local:80 \
  --set datasources."datasources\.yaml".datasources[0].access=proxy \
  --set datasources."datasources\.yaml".datasources[0].isDefault=true \
  --wait \
  --timeout=10m0s \
  --debug

echo "Grafana installation and configuration completed successfully."

exit 0
