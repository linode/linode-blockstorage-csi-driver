#!/bin/bash

# Check if namespace is provided
if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <filter>" >&2
  exit 1
fi

# Set environment variables (if not already set)
TARGET_API=${TARGET_API:-https://api.linode.com}
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
        "$TARGET_API/$TARGET_API_VERSION/$URI"
}

echo "Checking Linode API for volume status..."

for ((i=1; i<=$MAX_RETRIES; i++)); do
    response=$(curl_command)
    
    if [ $? -eq 0 ]; then
        # Check if the response is valid JSON
        if jq -e . >/dev/null 2>&1 <<< "$response"; then
            # Extract results and check if it's null
            results=$(echo "$response" | jq -r '.results')
            
            if [ "$results" = "0" ]; then
                echo "Volume deleted in Linode"
                exit 0
            else
                echo "Volume still available in Linode. Response:"
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

