#!/bin/bash

# Check if the correct number of arguments are provided
if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <filter>" >&2
  exit 1
fi

INTERVAL=3  # Time in seconds between checks

# Set environment variables for the second part
TARGET_API=${TARGET_API:-api.linode.com}
TARGET_API_VERSION=${TARGET_API_VERSION:-v4}
URI=${URI:-volumes}
FILTER=$1
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
            # Extract linode_id & volume_name
            linode_id=$(echo "$response" | jq -r '.data[0].linode_id')
            volume_name=$(echo "$response" | jq -r '.data[0].label' | sed 's/^pvc//')
            
            if [ "$linode_id" = "null" ]; then
                echo "Volume detached in Linode"
                break
            else
                echo "Volume still attached in Linode. Response:"
                echo "$response"
                if [ $i -lt $MAX_RETRIES ]; then
                    echo "Retrying in $RETRY_DELAY seconds..."
                    sleep $RETRY_DELAY
                else
                    echo "Max retries reached. Volume is still attached in Linode." >&2
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
        echo "Max retries reached. Exiting." >&2
        exit 1
    fi
done


echo "Checking for volume pvc-$volume_name in Kubernetes..."

for ((i=1; i<=$MAX_RETRIES; i++)); do
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
    VOLUME_PRESENT=$(kubectl get nodes -o json | jq --arg vol "$volume_name" '
      .items[] | 
      select(.status | (has("volumesInUse") and has("volumesAttached"))) |
      (.status.volumesAttached | map(.name) | any(contains($vol))) or
      (.status.volumesInUse | any(contains($vol)))
    ' | grep -q true && echo "true" || echo "false")

    if [ "$VOLUME_PRESENT" = "true" ]; then
      echo "Volume $volume_name is still attached or in use. Waiting..."
      if [ $i -lt $MAX_RETRIES ]; then
          echo "Retrying in $RETRY_DELAY seconds..."
          sleep $RETRY_DELAY
      else
          echo "Max retries reached. Volume is still attached to the Node." >&2
          exit 1
      fi
    else
      echo "Volume $volume_name is not attached or in use by any node."
      break
    fi
  fi

  if [ $i -lt $MAX_RETRIES ]; then
      sleep $RETRY_DELAY
  else
      echo "Max retries reached. Exiting." >&2
      exit 1
  fi
done

echo "Check completed successfully. Volume was successfully detached from all nodes!"
