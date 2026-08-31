#!/bin/bash

set -euf -o pipefail

# Default Values
DEFAULT_NAMESPACE="monitoring"
DEFAULT_DASHBOARD_FILE="observability/metrics/dashboard.json"
DEFAULT_LB_FILE="observability/metrics/loadBalancer.yaml"

# Function to display usage
usage() {
  echo "Usage: $0 --namespace=<namespace> --dashboard-file=<path_to_dashboard_json> --lb-file=<path_to_loadbalancer_yaml>"
  exit 1
}

# Parse command-line arguments
for arg in "$@"
do
  case $arg in
    --namespace=*)
      NAMESPACE="${arg#*=}"
      shift
      ;;
    --dashboard-file=*)
      DASHBOARD_FILE="${arg#*=}"
      shift
      ;;
    --lb-file=*)
      LB_FILE="${arg#*=}"
      shift
      ;;
    *)
      usage
      ;;
  esac
done

# Set default values if not provided
NAMESPACE=${NAMESPACE:-$DEFAULT_NAMESPACE}
DASHBOARD_FILE=${DASHBOARD_FILE:-$DEFAULT_DASHBOARD_FILE}
LB_FILE=${LB_FILE:-$DEFAULT_LB_FILE}

# Function to retrieve Grafana LoadBalancer External IP
get_grafana_lb_ip() {
  kubectl get svc grafana-lb -n ${NAMESPACE} -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
}

# Ensure the namespace exists
if ! kubectl get namespace "${NAMESPACE}" > /dev/null 2>&1; then
  echo "Namespace '${NAMESPACE}' does not exist. Creating..."
  kubectl create namespace "${NAMESPACE}"
else
  echo "Namespace '${NAMESPACE}' already exists."
fi

# Validate that the dashboard file exists
if [[ ! -f "${DASHBOARD_FILE}" ]]; then
  echo "Error: Dashboard file '${DASHBOARD_FILE}' does not exist."
  exit 1
fi

# Validate that the LoadBalancer YAML file exists
if [[ ! -f "${LB_FILE}" ]]; then
  echo "Error: LoadBalancer file '${LB_FILE}' does not exist."
  exit 1
fi

# Delete the existing ConfigMap if it exists
echo "Deleting existing Grafana dashboard ConfigMap if it exists..."
kubectl delete configmap grafana-dashboard \
  --namespace ${NAMESPACE} \
  --ignore-not-found

# Create or update the ConfigMap containing the dashboard JSON from the local file
echo "Creating or updating Grafana dashboard ConfigMap from local file..."
kubectl create configmap grafana-dashboard \
  --from-file=dashboard.json=${DASHBOARD_FILE} \
  --namespace ${NAMESPACE} \
  --dry-run=client -o yaml | kubectl apply -f -

# Add the label to the ConfigMap
kubectl label configmap grafana-dashboard \
  --namespace ${NAMESPACE} \
  grafana_dashboard=1 --overwrite

# Apply the LoadBalancer YAML file to create the LoadBalancer service
echo "Applying LoadBalancer service from file..."
kubectl apply -f ${LB_FILE} --namespace ${NAMESPACE}

# Wait for the LoadBalancer to get an external IP
echo "Waiting for LoadBalancer to get an external IP..."
EXTERNAL_IP=""
while [[ -z "$EXTERNAL_IP" ]]; do
  EXTERNAL_IP=$(get_grafana_lb_ip)
  if [[ -z "$EXTERNAL_IP" ]]; then
    echo "Waiting for LoadBalancer external IP..."
    sleep 10
  fi
done

# Output the Grafana dashboard access URL
echo "------------------------------------------------------------"
echo "Grafana Dashboard Setup Complete!"
echo "Access Grafana using the following URL:"
echo "  - http://${EXTERNAL_IP}"
echo ""
echo "Grafana Admin Credentials:"
echo "  - Username: ${GRAFANA_USERNAME:-admin}"
echo "  - Password: ${GRAFANA_PASSWORD:-admin}"
echo "------------------------------------------------------------"
