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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	limit, err := s.maxVolumeAttachments(ctx, instance)
	if err != nil {
		return false, err
	}

	volumes, err := s.client.ListInstanceVolumes(ctx, instance.ID, nil)
	if err != nil {
		return false, errInternal("list instance volumes: %v", err)
	}

	return len(volumes) < limit, nil
}

// maxVolumeAttachments returns the maximum number of volumes that can be
// attached to a single Linode instance, minus any currently-attached instance
// disks.
func (s *ControllerServer) maxVolumeAttachments(ctx context.Context, instance *linodego.Instance) (int, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Calculating max volume attachments")

	if instance == nil || instance.Specs == nil {
		return 0, errNilInstance
	}

	disks, err := s.client.ListInstanceDisks(ctx, instance.ID, nil)
	if err != nil {
		return 0, errInternal("list instance disks: %v", err)
	}

	// The reported amount of memory for an instance is in MB.
	// Convert it to bytes.
	memBytes := uint(instance.Specs.Memory) << 20
	return maxVolumeAttachments(memBytes) - len(disks), nil
}

// attemptGetContentSourceVolume retrieves information about the Linode volume to clone from.
// It returns a LinodeVolumeKey if a valid source volume is found, or an error if the source is invalid.
func (cs *ControllerServer) attemptGetContentSourceVolume(ctx context.Context, contentSource *csi.VolumeContentSource) (*linodevolumes.LinodeVolumeKey, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to get content source volume")

	if contentSource == nil {
		return nil, nil
	}

	if _, ok := contentSource.GetType().(*csi.VolumeContentSource_Volume); !ok {
		return nil, errUnsupportedVolumeContentSource
	}

	sourceVolume := contentSource.GetVolume()
	if sourceVolume == nil {
		return nil, errNoSourceVolume
	}

	volumeInfo, err := linodevolumes.ParseLinodeVolumeKey(sourceVolume.GetVolumeId())
	if err != nil {
		return nil, errInternal("parse volume info from content source: %v", err)
	}

	volumeData, err := cs.client.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		return nil, errInternal("get volume %d: %v", volumeInfo.VolumeID, err)
	}

	if volumeData.Region != cs.metadata.Region {
		return nil, errRegionMismatch(volumeData.Region, cs.metadata.Region)
	}

	return volumeInfo, nil
}

// attemptCreateLinodeVolume creates a Linode volume while ensuring idempotency.
// It checks for existing volumes with the same label and either returns the existing
// volume or creates a new one, optionally cloning from a source volume.
func (cs *ControllerServer) attemptCreateLinodeVolume(ctx context.Context, label string, sizeGB int, tags string, sourceVolume *linodevolumes.LinodeVolumeKey) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to create Linode volume", "label", label, "sizeGB", sizeGB, "tags", tags)

	// List existing volumes
	jsonFilter, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		return nil, errInternal("marshal json filter: %v", err)
	}

	volumes, err := cs.client.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, errInternal("list volumes: %v", err)
	}

	// This shouldn't happen, but raise an error just in case
	if len(volumes) > 1 {
		return nil, status.Errorf(codes.AlreadyExists, "volume %q already exists", label)
	}

	// Volume already exists
	if len(volumes) == 1 {
		return &volumes[0], nil
	}

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

	volumeReq := linodego.VolumeCreateOptions{
		Region: cs.metadata.Region,
		Label:  label,
		Size:   sizeGB,
	}

	if tags != "" {
		volumeReq.Tags = strings.Split(tags, ",")
	}

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
	if capRange == nil {
		return MinVolumeSizeBytes, nil
	}

	// Volume MUST be at least this big. This field is OPTIONAL.
	reqSize := capRange.GetRequiredBytes()

	// Volume MUST not be bigger than this. This field is OPTIONAL.
	maxSize := capRange.GetLimitBytes()

	// At least one of these fields MUST be specified.
	if reqSize == 0 && maxSize == 0 {
		return 0, errors.New("RequiredBytes or LimitBytes must be set")
	}

	// The value of this field MUST NOT be negative.
	if reqSize < 0 || maxSize < 0 {
		return 0, errors.New("RequiredBytes and LimitBytes may not be negative")
	}

	if maxSize == 0 {
		// Only received a required size
		if reqSize < MinVolumeSizeBytes {
			// the Linode API would reject the request, opt to fulfill it by provisioning above
			// the requested size, but no more than the limit size
			reqSize = MinVolumeSizeBytes
		}
		maxSize = reqSize
	} else if maxSize < MinVolumeSizeBytes {
		return 0, fmt.Errorf("limit bytes %v is less than minimum bytes %v", maxSize, MinVolumeSizeBytes)
	}

	// fulfill the upper bound of the request
	if reqSize == 0 || reqSize < maxSize {
		reqSize = maxSize
	}

	return reqSize, nil
}

// validVolumeCapabilities checks if the provided volume capabilities are valid.
// It ensures that each capability is non-nil and that the access mode is set to
// SINGLE_NODE_WRITER.
func validVolumeCapabilities(caps []*csi.VolumeCapability) bool {
	for _, cap := range caps {
		if cap == nil {
			return false
		}
		accMode := cap.GetAccessMode()

		if accMode == nil {
			return false
		}

		if accMode.GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return false
		}
	}
	return true
}
