package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"github.com/linode/linodego"
)

// MinVolumeSizeBytes is the smallest allowed size for a Linode block storage
// Volume, in bytes.
//
// The CSI RPC scheme deal with bytes, whereas the Linode API's block storage
// volume endpoints deal with "GB".
// Internally, the driver will deal with sizes and capacities in bytes, but
// convert to and from "GB" when interacting with the Linode API.
const MinVolumeSizeBytes = 10 << 30 // 10GiB

// bytesToGB is a convenience function that converts the given number of bytes
// to gigabytes.
// This function should be used when converting a CSI RPC type's capacity range
// to a value that the Linode API will understand.
func bytesToGB(numBytes int64) int { return int(numBytes >> 30) }

// gbToBytes is a convenience function that converts gigabytes to bytes.
// This function is typically going to be used when converting
// [github.com/linode/linodego.Volume.Size] to a value that works with the CSI
// RPC types.
func gbToBytes(gb int) int64 { return int64(gb << 30) }

const (
	// WaitTimeout is the default timeout duration used for polling the Linode
	// API, when waiting for a volume to enter an "active" state.
	WaitTimeout = 5 * time.Minute

	// CloneTimeout is the duration to wait when cloning a volume through the
	// Linode API.
	CloneTimeout = 15 * time.Minute
)

// waitTimeout is a convenience function to get the number of seconds in
// [WaitTimeout].
func waitTimeout() int {
	return int(WaitTimeout.Truncate(time.Second).Seconds())
}

// cloneTimeout is a convenience function to get the number of seconds in
// [CloneTimeout].
func cloneTimeout() int {
	return int(CloneTimeout.Truncate(time.Second).Seconds())
}

const (
	// VolumeTags is the parameter key used for passing a comma-separated list
	// of tags to the Linode API.
	VolumeTags = Name + "/volumeTags"

	// PublishInfoVolumeName is used to pass the name of the volume as it exists
	// in the Linode API (the "label") to [NodeStageVolume] and
	// [NodePublishVolume].
	PublishInfoVolumeName = Name + "/volume-name"

	// VolumeTopologyRegion is the parameter key used to indicate the region
	// the volume exists in.
	VolumeTopologyRegion string = "topology.linode.com/region"

	// devicePathKey is the key used in the publish context map when a volume is
	// published/attached to an instance.
	devicePathKey = "devicePath"
)

// canAttach indicates whether or not another volume can be attached to the
// Linode with the given ID.
//
// Whether or not another volume can be attached is based on how many instance
// disks and block storage volumes are currently attached to the instance.
func (s *ControllerServer) canAttach(ctx context.Context, instance *linodego.Instance) (canAttach bool, err error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Checking if volume can be attached", "instance_id", instance.ID)

	// Get the maximum number of volume attachments allowed for the instance
	limit, err := s.maxAllowedVolumeAttachments(ctx, instance)
	if err != nil {
		return false, err
	}

	// List the volumes currently attached to the instance
	volumes, err := s.client.ListInstanceVolumes(ctx, instance.ID, nil)
	if err != nil {
		return false, errInternal("list instance volumes: %v", err)
	}

	// Return true if the number of attached volumes is less than the limit
	return len(volumes) < limit, nil
}

// maxAllowedVolumeAttachments calculates the maximum number of volumes that can be attached to a Linode instance,
// taking into account the instance's memory and currently attached disks.
func (s *ControllerServer) maxAllowedVolumeAttachments(ctx context.Context, instance *linodego.Instance) (int, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Calculating max volume attachments")

	// Check if the instance or its specs are nil
	if instance == nil || instance.Specs == nil {
		return 0, errNilInstance
	}

	// Retrieve the list of disks currently attached to the instance
	disks, err := s.client.ListInstanceDisks(ctx, instance.ID, nil)
	if err != nil {
		return 0, errInternal("list instance disks: %v", err)
	}

	// Convert the reported memory from MB to bytes
	memBytes := uint(instance.Specs.Memory) << 20
	return maxVolumeAttachments(memBytes) - len(disks), nil
}

// getContentSourceVolume retrieves information about the Linode volume to clone from.
// It returns a LinodeVolumeKey if a valid source volume is found, or an error if the source is invalid.
func (cs *ControllerServer) getContentSourceVolume(ctx context.Context, contentSource *csi.VolumeContentSource) (*linodevolumes.LinodeVolumeKey, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to get content source volume")

	if contentSource == nil {
		return nil, nil // Return nil if no content source is provided
	}

	// Check if the content source type is a volume
	if _, ok := contentSource.GetType().(*csi.VolumeContentSource_Volume); !ok {
		return nil, errUnsupportedVolumeContentSource
	}

	sourceVolume := contentSource.GetVolume()
	if sourceVolume == nil {
		return nil, errNoSourceVolume // Return error if no source volume is specified
	}

	// Parse the volume ID from the content source
	volumeInfo, err := linodevolumes.ParseLinodeVolumeKey(sourceVolume.GetVolumeId())
	if err != nil {
		return nil, errInternal("parse volume info from content source: %v", err)
	}
	if volumeInfo == nil {
		return nil, errInternal("processed *LinodeVolumeKey is nil") // Throw an internal error if the processed LinodeVolumeKey is nil
	}

	// Retrieve the volume data using the parsed volume ID
	volumeData, err := cs.client.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		return nil, errInternal("get volume %d: %v", volumeInfo.VolumeID, err)
	}
	if volumeData == nil {
		return nil, errInternal("source volume *linodego.Volume is nil") // Throw an internal error if the processed linodego.Volume is nil
	}

	// Check if the volume's region matches the server's metadata region
	if volumeData.Region != cs.metadata.Region {
		return nil, errRegionMismatch(volumeData.Region, cs.metadata.Region)
	}

	return volumeInfo, nil // Return the parsed volume information
}

// attemptCreateLinodeVolume creates a Linode volume while ensuring idempotency.
// It checks for existing volumes with the same label and either returns the existing
// volume or creates a new one, optionally cloning from a source volume.
func (cs *ControllerServer) attemptCreateLinodeVolume(ctx context.Context, label string, sizeGB int, tags string, sourceVolume *linodevolumes.LinodeVolumeKey) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to create Linode volume", "label", label, "sizeGB", sizeGB, "tags", tags)

	// List existing volumes with the specified label
	jsonFilter, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		return nil, errInternal("marshal json filter: %v", err)
	}

	volumes, err := cs.client.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, errInternal("list volumes: %v", err)
	}

	// Raise an error if more than one volume with the same label exists
	if len(volumes) > 1 {
		return nil, errAlreadyExists("more than one volume with the label %q exists", label)
	}

	// Return the existing volume if found
	if len(volumes) == 1 {
		return &volumes[0], nil
	}

	// Clone the source volume if provided, otherwise create a new volume
	if sourceVolume != nil {
		return cs.cloneLinodeVolume(ctx, label, sourceVolume.VolumeID)
	}

	return cs.createLinodeVolume(ctx, label, sizeGB, tags)
}

// createLinodeVolume creates a new Linode volume with the specified label, size, and tags.
// It returns the created volume or an error if the creation fails.
func (cs *ControllerServer) createLinodeVolume(ctx context.Context, label string, sizeGB int, tags string) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Creating Linode volume", "label", label, "sizeGB", sizeGB, "tags", tags)

	// Prepare the volume creation request with region, label, and size.
	volumeReq := linodego.VolumeCreateOptions{
		Region: cs.metadata.Region,
		Label:  label,
		Size:   sizeGB,
	}

	// If tags are provided, split them into a slice for the request.
	if tags != "" {
		volumeReq.Tags = strings.Split(tags, ",")
	}

	// Attempt to create the volume using the client and handle any errors.
	result, err := cs.client.CreateVolume(ctx, volumeReq)
	if err != nil {
		return nil, errInternal("create volume: %v", err)
	}

	log.V(4).Info("Linode volume created", "volume", result)
	return result, nil
}

// cloneLinodeVolume clones a Linode volume using the specified source ID and label.
// It returns the cloned volume or an error if the cloning fails.
func (cs *ControllerServer) cloneLinodeVolume(ctx context.Context, label string, sourceID int) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Cloning Linode volume", "label", label, "source_vol_id", sourceID)

	result, err := cs.client.CloneVolume(ctx, sourceID, label)
	if err != nil {
		return nil, errInternal("clone volume %d: %v", sourceID, err)
	}

	log.V(4).Info("Linode volume cloned", "volume", result)
	return result, nil
}

// getRequestCapacitySize validates the CapacityRange and determines the optimal volume size.
// It returns the minimum size if no range is provided, or the required size if specified.
// It ensures that the size is not negative and does not exceed the maximum limit.
func getRequestCapacitySize(capRange *csi.CapacityRange) (int64, error) {
    // If no capacity range is provided, return the minimum volume size
    if capRange == nil {
        return MinVolumeSizeBytes, nil
    }

	// Volume MUST be at least this big. This field is OPTIONAL.
	reqSize := capRange.GetRequiredBytes()

	// Volume MUST not be bigger than this. This field is OPTIONAL.
	maxSize := capRange.GetLimitBytes()

    // Validate that at least one size is specified
    if reqSize == 0 && maxSize == 0 {
        return 0, errors.New("either RequiredBytes or LimitBytes must be set")
    }

    // Check for negative values
    if reqSize < 0 || maxSize < 0 {
        return 0, errors.New("RequiredBytes and LimitBytes must not be negative")
    }

    // Handle case where only required size is specified
    if maxSize == 0 {
        return adjustToMinimumSize(reqSize), nil
    }

    // Handle case where max size is less than minimum allowed
    if maxSize < MinVolumeSizeBytes {
        return 0, fmt.Errorf("limit bytes %v is less than minimum allowed bytes %v", maxSize, MinVolumeSizeBytes)
    }

    // Determine the final size
    return determineOptimalSize(reqSize, maxSize), nil
}

// adjustToMinimumSize ensures that the provided size is at least the minimum volume size.
// If the size is less than MinVolumeSizeBytes, it returns MinVolumeSizeBytes; otherwise, it returns the original size.
func adjustToMinimumSize(size int64) int64 {
    if size < MinVolumeSizeBytes {
        return MinVolumeSizeBytes
    }
    return size
}

// determineOptimalSize calculates the optimal size for a volume based on the required size and maximum size.
// If the required size is zero or less than the maximum size, it returns the maximum size.
// Otherwise, it returns the required size.
func determineOptimalSize(reqSize, maxSize int64) int64 {
    if reqSize == 0 || reqSize < maxSize {
        return maxSize
    }
    return reqSize
}

// validVolumeCapabilities checks if the provided volume capabilities are valid.
// It ensures that each capability is non-nil and that the access mode is set to
// SINGLE_NODE_WRITER.
func validVolumeCapabilities(caps []*csi.VolumeCapability) bool {
	// Iterate through each capability in the provided slice
	for _, cap := range caps {
		// Check if the capability is nil; if so, return false
		if cap == nil {
			return false
		}
		// Retrieve the access mode for the capability
		accMode := cap.GetAccessMode()

		// Check if the access mode is nil; if so, return false
		if accMode == nil {
			return false
		}

		// Ensure the access mode is SINGLE_NODE_WRITER; if not, return false
		if accMode.GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return false
		}
	}
	// All capabilities are valid; return true
	return true
}