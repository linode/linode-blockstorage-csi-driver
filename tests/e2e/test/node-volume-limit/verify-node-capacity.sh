#!/bin/bash

# Script to verify CSI node capacity matches expected calculation
# This script performs the complete verification logic used in the chainsaw test
# Usage: ./verify-node-capacity.sh <node-name> [context-suffix]

set -e

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Usage: $0 <node-name> [context-suffix]" >&2
    echo "  node-name: Name of the Kubernetes node to verify" >&2
    echo "  context-suffix: Optional suffix for log messages (e.g., 'after restart')" >&2
    exit 1
fi

NODE_NAME="$1"
CONTEXT_SUFFIX="${2:-}"

# Default maximum attachments for instances with < 16GiB RAM (see internal/driver/limits.go)
DEFAULT_MAX_ATTACHMENTS=8

echo "Verifying node capacity for node: $NODE_NAME${CONTEXT_SUFFIX:+ $CONTEXT_SUFFIX}"

# Get CSI allocatable count
MAX_VOL=$(kubectl get csinode "$NODE_NAME" -o jsonpath='{.spec.drivers[?(@.name=="linodebs.csi.linode.com")].allocatable.count}')

# Validate MAX_VOL
if [ -z "$MAX_VOL" ] || ! [[ "$MAX_VOL" =~ ^[0-9]+$ ]]; then
    echo "Error: Invalid or empty CSI allocatable count for node $NODE_NAME" >&2
    exit 1
fi

# Get the CSI node pod running on the target node
CSI_POD=$(kubectl get pods -n kube-system -l app=csi-linode-node --field-selector spec.nodeName="$NODE_NAME" -o jsonpath='{.items[0].metadata.name}')
if [ -z "$CSI_POD" ]; then
    echo "Error: Could not find CSI node pod on node $NODE_NAME" >&2
    exit 1
fi
echo "Found CSI node pod: $CSI_POD"

# Count QEMU disks inside the CSI node pod using /sys/block
# This mimics the logic used by the CSI driver itself (see internal/driver/limits.go)
DISK_COUNT=$(kubectl exec -n kube-system "$CSI_POD" -c csi-linode-plugin -- sh -c '
  count=0
  # Check each block device in /sys/block
  for device in /sys/block/*; do
    if [ -d "$device" ]; then
      device_name=$(basename "$device")
      # Skip loop, ram, and other non-disk devices
      case "$device_name" in
        loop*|ram*|sr*|fd*) continue ;;
      esac
      # Check if device has vendor info
      if [ -f "$device/device/vendor" ]; then
        vendor=$(cat "$device/device/vendor" 2>/dev/null | tr -d " \t\n\r")
        if [ "$vendor" = "QEMU" ] || [ "$vendor" = "qemu" ]; then
          count=$((count + 1))
        fi
      fi
    fi
  done
  echo $count
' 2>/dev/null)

# Validate DISK_COUNT
if [ -z "$DISK_COUNT" ] || ! [[ "$DISK_COUNT" =~ ^[0-9]+$ ]]; then
    echo "Error: Invalid or empty QEMU disk count for node $NODE_NAME" >&2
    exit 1
fi

# Calculate expected max volumes using the defined constant
# The CSI driver uses maxVolumeAttachments(memory) - diskCount
# For test instances (< 16GiB RAM), this defaults to DEFAULT_MAX_ATTACHMENTS - diskCount
EXPECTED_MAX_VOL=$((DEFAULT_MAX_ATTACHMENTS - DISK_COUNT))

echo "CSI allocatable count: $MAX_VOL"
echo "QEMU disk count: $DISK_COUNT"
echo "Expected max volumes ($DEFAULT_MAX_ATTACHMENTS - $DISK_COUNT): $EXPECTED_MAX_VOL"

if [ "$MAX_VOL" != "$EXPECTED_MAX_VOL" ]; then
    echo "Mismatch${CONTEXT_SUFFIX:+ $CONTEXT_SUFFIX}: CSI count ($MAX_VOL) != Expected count ($EXPECTED_MAX_VOL)" >&2
    exit 1
fi

echo "Node capacity verified successfully${CONTEXT_SUFFIX:+ $CONTEXT_SUFFIX}."
