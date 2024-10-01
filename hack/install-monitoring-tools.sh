#!/bin/bash

set -euf -o pipefail

GRAFANA_PORT=3000
GRAFANA_POD=""
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
  echo "No worker nodes found. Untainting control-plane node(s) to allow scheduling of Prometheus and Grafana pods..."
  for NODE in ${CONTROL_PLANE_NODES}; do
    echo "Untainting node: ${NODE}"
    kubectl taint nodes "${NODE}" node-role.kubernetes.io/control-plane- || true
  done
else
  echo "Worker nodes detected. Skipping untainting of control-plane nodes."
fi

# Add Helm repositories if not already added
echo "Adding Helm repositories for Prometheus and Grafana..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts || true
helm repo add grafana https://grafana.github.io/helm-charts || true

# Update Helm repositories
echo "Updating Helm repositories..."
helm repo update

# Create service to export the metrics for Prometheus to scrape from the sidecars
kubectl apply -f observability/metrics/csi-linode-controller-metrics-service.yaml

# Create a namespace for monitoring tools
echo "Creating namespace '${NAMESPACE}'..."
kubectl create namespace ${NAMESPACE} || true

# Install or Upgrade Prometheus
echo "Installing or upgrading Prometheus..."
helm upgrade --install prometheus prometheus-community/prometheus \
  --namespace ${NAMESPACE} \
  --set server.persistentVolume.enabled=false \
  --wait \
  --timeout=10m0s \
  --debug

# Install or Upgrade Grafana with Prometheus data source
echo "Installing or upgrading Grafana with Prometheus data source..."
helm upgrade --install grafana grafana/grafana \
  --namespace ${NAMESPACE} \
  --set adminPassword='admin' \
  --set service.type=ClusterIP \
  --set service.port=${GRAFANA_PORT} \
  --set sidecar.dashboards.enabled=true \
  --set sidecar.dashboards.label=grafana_dashboard \
  --set datasources."datasources\.yaml".apiVersion=1 \
  --set datasources."datasources\.yaml".datasources[0].name=Prometheus \
  --set datasources."datasources\.yaml".datasources[0].type=prometheus \
  --set datasources."datasources\.yaml".datasources[0].url=http://prometheus-server.${NAMESPACE}.svc.cluster.local \
  --set datasources."datasources\.yaml".datasources[0].access=proxy \
  --set datasources."datasources\.yaml".datasources[0].isDefault=true \
  --wait \
  --timeout=10m0s \
  --debug

# Delete the existing ConfigMap if it exists
kubectl delete configmap grafana-dashboard \
  --namespace ${NAMESPACE} \
  --ignore-not-found

# Create or update the ConfigMap containing the dashboard JSON from the local file
echo "Creating or updating Grafana dashboard ConfigMap from local file..."

kubectl create configmap grafana-dashboard \
  --from-file=dashboard.json=docs/dashboard.json \
  --namespace ${NAMESPACE} \
  --dry-run=client -o yaml | kubectl apply -f -

# Add the label to the ConfigMap
kubectl label configmap grafana-dashboard \
  --namespace ${NAMESPACE} \
  grafana_dashboard=1 --overwrite

# Retrieve Grafana pod name
echo "Retrieving Grafana pod name..."
GRAFANA_POD=$(kubectl get pods --namespace ${NAMESPACE} -l "app.kubernetes.io/name=grafana" -o jsonpath="{.items[0].metadata.name}")

# Port-forward Grafana service
echo "Port-forwarding Grafana service on port ${GRAFANA_PORT}..."
nohup kubectl port-forward --namespace ${NAMESPACE} svc/grafana ${GRAFANA_PORT}:${GRAFANA_PORT} > port-forward.log 2>&1 &
PORT_FORWARD_PID=$!

# Give port-forward some time to start
sleep 5

# Provide instructions to access Grafana
echo "Please open http://localhost:${GRAFANA_PORT} in your web browser."

# Grafana login details
echo "Grafana admin username: admin"
echo "Grafana admin password: admin"

echo "To stop the port-forwarding, run: kill ${PORT_FORWARD_PID}"

exit 0