#!/bin/bash

# Check if the correct number of arguments are provided
if [ "$#" -ne 3 ]; then
  echo "Usage: $0 <namespace> <pvc-name> <filter>"
  exit 1
fi

NAMESPACE="$1"
PVC_NAME="$2"
INTERVAL=3  # Time in seconds between checks

# Extract the volume name from the PVC and remove the 'pvc-' prefix
VOLUME_NAME=$(kubectl get pvc $PVC_NAME -n $NAMESPACE -o jsonpath='{.spec.volumeName}' | sed 's/^pvc-//')

echo "Checking for volume: $VOLUME_NAME"

while true; do
  # Check if any node has both volumesInUse and volumesAttached fields
  NODE_HAS_FIELDS=$(kubectl get nodes -o json | jq '
    .items | map(
      .status | 
      (has("volumesInUse") and has("volumesAttached"))
    ) | any
  ')

  if [ "$NODE_HAS_FIELDS" = "false" ]; then
    echo "No nodes have both volumesInUse and volumesAttached fields."
    break
  else
    # Check if the volume is in volumesInUse or volumesAttached of any node
    VOLUME_PRESENT=$(kubectl get nodes -o json | jq --arg vol "$VOLUME_NAME" '
      .items[] | 
      select(.status | (has("volumesInUse") and has("volumesAttached"))) |
      (.status.volumesAttached | map(.name) | any(contains($vol))) or
      (.status.volumesInUse | any(contains($vol)))
    ' | grep -q true && echo "true" || echo "false")

    if [ "$VOLUME_PRESENT" = "true" ]; then
      echo "Volume $VOLUME_NAME is still attached or in use. Waiting..."
      sleep $INTERVAL
    else
      echo "Volume $VOLUME_NAME is not attached or in use by any node."
      break
    fi
  fi
done

echo "Check completed successfully. Volume was successfully detached from all nodes!"

# Set environment variables for the second part
TARGET_API=${TARGET_API:-api.linode.com}
TARGET_API_VERSION=${TARGET_API_VERSION:-v4}
URI=${URI:-volumes}
FILTER=$3
MAX_RETRIES=5
RETRY_DELAY=5

curl_command() {
    curl -s \
        -H "Authorization: Bearer $LINODE_TOKEN" \
        -H "X-Filter: $FILTER" \
        -H "Content-Type: application/json" \
        "https://$TARGET_API/$TARGET_API_VERSION/$URI"
}

echo "Checking Linode API for volume status..."

for ((i=1; i<=$MAX_RETRIES; i++)); do
    response=$(curl_command)
    
    if [ $? -eq 0 ]; then
        # Check if the response is valid JSON
        if jq -e . >/dev/null 2>&1 <<< "$response"; then
            # Extract linode_id and check if it's null
            linode_id=$(echo "$response" | jq -r '.data[0].linode_id')
            
            if [ "$linode_id" = "null" ]; then
                echo "Volume detached in Linode"
                exit 0
            else
                echo "Volume still attached in Linode. Response:"
                echo "$response"
                if [ $i -lt $MAX_RETRIES ]; then
                    echo "Retrying in $RETRY_DELAY seconds..."
                    sleep $RETRY_DELAY
                else
                    echo "Max retries reached. Volume is still attached in Linode."
                    exit 1
                fi
            fi
        else
            echo "Invalid JSON response. Retrying..."
        fi
    else
        echo "Curl command failed. Retrying..."
    fi
    
    if [ $i -lt $MAX_RETRIES ]; then
        sleep $RETRY_DELAY
    else
        echo "Max retries reached. Exiting."
        exit 1
    fi
done
