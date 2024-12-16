#!/bin/bash

set -euf -o pipefail

# Default Values
NAMESPACE="kube-system"
RETRIES=5
TRACING_FILES=("observability/tracing/otel-configmap.yaml"
               "observability/tracing/otel-deployment.yaml"
               "observability/tracing/otel-service.yaml"
               "observability/tracing/jager-deployment.yaml"
               "observability/tracing/jager-service.yaml")

# Ensure namespace exists
if ! kubectl get namespace "${NAMESPACE}" > /dev/null 2>&1; then
  echo "Namespace '${NAMESPACE}' does not exist. Creating..."
  kubectl create namespace "${NAMESPACE}"
else
  echo "Namespace '${NAMESPACE}' already exists."
fi

# Apply each file
echo "Applying tracing YAML files..."
for file in "${TRACING_FILES[@]}"; do
  if [[ -f "$file" ]]; then
    echo "Applying $file..."
    kubectl apply -f "$file" --namespace ${NAMESPACE}
  else
    echo "Error: File $file not found. Exiting..."
    exit 1
  fi
done

# Retrieve and print the Jaeger LoadBalancer IP
get_jaeger_lb_ip() {
  kubectl get svc jaeger-collector -n ${NAMESPACE} -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo ""
}

echo "Waiting for Jaeger LoadBalancer to get an external IP..."
EXTERNAL_IP=""
attempt=0
while [[ -z "$EXTERNAL_IP" && $attempt -lt $RETRIES ]]; do
  EXTERNAL_IP=$(get_jaeger_lb_ip)
  if [[ -z "$EXTERNAL_IP" ]]; then
    echo "Attempt $((attempt + 1))/$RETRIES: Waiting for LoadBalancer external IP..."
    attempt=$((attempt + 1))
    sleep 10
  fi
done

if [[ -z "$EXTERNAL_IP" ]]; then
  echo "Error: Failed to retrieve Jaeger LoadBalancer external IP after $RETRIES attempts. Exiting..."
  exit 1
fi

echo "------------------------------------------------------------"
echo "Jaeger Dashboard Setup Complete!"
echo "Access Jaeger using the following URL:"
echo "  - http://${EXTERNAL_IP}:16686"
echo "------------------------------------------------------------"
