package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/metrics"
)

// MinVolumeSizeBytes is the smallest allowed size for a Linode block storage
// Volume, in bytes.
//
// The CSI RPC scheme deal with bytes, whereas the Linode API's block storage
// volume endpoints deal with "GB".
// Internally, the driver will deal with sizes and capacities in bytes, but
// convert to and from "GB" when interacting with the Linode API.
const (
	MinVolumeSizeBytes = 10 << 30 // 10GiB
	True               = "true"
)

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

	// volumeEncryption is the key used in the context map for encryption
	VolumeEncryption = Name + "/encrypted"
)

// Struct to return volume parameters when prepareVolumeParams is called

type VolumeParams struct {
	VolumeName       string
	TargetSizeGB     int
	Size             int64
	EncryptionStatus string
	Region           string
}

// canAttach indicates whether or not another volume can be attached to the
// Linode with the given ID.
//
// Whether or not another volume can be attached is based on how many instance
// disks and block storage volumes are currently attached to the instance.
func (cs *ControllerServer) canAttach(ctx context.Context, instance *linodego.Instance) (canAttach bool, err error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Checking if volume can be attached", "instance_id", instance.ID)

	// Get the maximum number of volume attachments allowed for the instance
	limit, err := cs.maxAllowedVolumeAttachments(ctx, instance)
	if err != nil {
		return false, err
	}

	// List the volumes currently attached to the instance
	volumes, err := cs.client.ListInstanceVolumes(ctx, instance.ID, nil)
	if err != nil {
		return false, errInternal("list instance volumes: %v", err)
	}

	// Return true if the number of attached volumes is less than the limit
	return len(volumes) < limit, nil
}

// maxAllowedVolumeAttachments calculates the maximum number of volumes that can be attached to a Linode instance,
// taking into account the instance's memory and currently attached disks.
func (cs *ControllerServer) maxAllowedVolumeAttachments(ctx context.Context, instance *linodego.Instance) (int, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Calculating max volume attachments")

	// Check if the instance or its specs are nil
	if instance == nil || instance.Specs == nil {
		return 0, errNilInstance
	}

	// Retrieve the list of disks currently attached to the instance
	disks, err := cs.client.ListInstanceDisks(ctx, instance.ID, nil)
	if err != nil {
		return 0, errInternal("list instance disks: %v", err)
	}

	// Convert the reported memory from MB to bytes
	memBytes := uint(instance.Specs.Memory) << 20
	return maxVolumeAttachments(memBytes) - len(disks), nil
}

// getContentSourceVolume retrieves information about the Linode volume to clone from.
// It returns a LinodeVolumeKey if a valid source volume is found, or an error if the source is invalid.
func (cs *ControllerServer) getContentSourceVolume(ctx context.Context, contentSource *csi.VolumeContentSource, accessibilityRequirements *csi.TopologyRequirement) (volKey *linodevolumes.LinodeVolumeKey, err error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to get content source volume")

	if contentSource == nil {
		return volKey, nil // Return nil if no content source is provided
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
	volKey, err = linodevolumes.ParseLinodeVolumeKey(sourceVolume.GetVolumeId())
	if err != nil {
		return nil, errInternal("parse volume info from content source: %v", err)
	}
	if volKey == nil {
		return nil, errInternal("processed *LinodeVolumeKey is nil") // Throw an internal error if the processed LinodeVolumeKey is nil
	}

	// Retrieve the volume data using the parsed volume ID
	volumeData, err := cs.client.GetVolume(ctx, volKey.VolumeID)
	if err != nil {
		return nil, errInternal("get volume %d: %v", volKey.VolumeID, err)
	}
	if volumeData == nil {
		return nil, errInternal("source volume *linodego.Volume is nil") // Throw an internal error if the processed linodego.Volume is nil
	}

	// Check if the source volume's region matches the required region
	requiredRegion := cs.metadata.Region
	if accessibilityRequirements != nil {
		if topologyRegion := getRegionFromTopology(accessibilityRequirements); topologyRegion != "" {
			requiredRegion = topologyRegion
		}
	}

	if volumeData.Region != requiredRegion {
		return nil, errRegionMismatch(volumeData.Region, requiredRegion)
	}

	log.V(4).Info("Content source volume", "volumeData", volumeData)
	return volKey, nil
}

// attemptCreateLinodeVolume creates a Linode volume while ensuring idempotency.
// It checks for existing volumes with the same label and either returns the existing
// volume or creates a new one, optionally cloning from a source volume.
func (cs *ControllerServer) attemptCreateLinodeVolume(ctx context.Context, label, tags, volumeEncryption string, sizeGB int, sourceVolume *linodevolumes.LinodeVolumeKey, region string) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to create Linode volume", "label", label, "sizeGB", sizeGB, "tags", tags, "encryptionStatus", volumeEncryption, "region", region)

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

	return cs.createLinodeVolume(ctx, label, tags, volumeEncryption, sizeGB, region)
}

// Helper function to extract region from topology
func getRegionFromTopology(requirements *csi.TopologyRequirement) string {
	topologies := requirements.GetPreferred()
	if len(topologies) == 0 {
		topologies = requirements.GetRequisite()
	}

	if len(topologies) > 0 {
		if value, ok := topologies[0].GetSegments()[VolumeTopologyRegion]; ok {
			return value
		}
	}

	return ""
}

// createLinodeVolume creates a new Linode volume with the specified label, size, and tags.
// It returns the created volume or an error if the creation fails.
func (cs *ControllerServer) createLinodeVolume(ctx context.Context, label, tags, encryptionStatus string, sizeGB int, region string) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Creating Linode volume", "label", label, "sizeGB", sizeGB, "tags", tags, "encryptionStatus", encryptionStatus, "region", region)

	// Prepare the volume creation request with region, label, and size.
	volumeReq := linodego.VolumeCreateOptions{
		Region:     region,
		Label:      label,
		Size:       sizeGB,
		Encryption: encryptionStatus,
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

// isEncryptionSupported is a helper function that checks if the specified region supports volume encryption.
// It returns true or false based on the support for encryption in that region.
func (cs *ControllerServer) isEncryptionSupported(ctx context.Context, region string) (bool, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Checking if encryption is supported for region", "region", region)

	// Get the specifications of specified region from Linode API
	regionDetails, err := cs.client.GetRegion(ctx, region)
	if err != nil {
		return false, errInternal("failed to fetch region %s: %v", region, err)
	}

	// Check if encryption is supported in the specified region
	for _, capability := range regionDetails.Capabilities {
		if capability == "Block Storage Encryption" {
			return true, nil
		}
	}

	// If the region was found but does not support encryption, return false
	log.V(4).Info("Encryption not supported in the specified region", "region", region)
	return false, nil
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
func getRequestCapacitySize(ctx context.Context, capRange *csi.CapacityRange) (int64, error) {
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

	metrics.TraceFunctionData(ctx, "CheckRequestedSize", map[string]string{
		"requestSize": strconv.Itoa(int(reqSize)),
		"maxSize":     strconv.Itoa(int(maxSize)),
	}, metrics.TracingSubfunction, nil)

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

// validateCreateVolumeRequest checks if the provided CreateVolumeRequest is valid.
// It ensures that the volume name is not empty, that volume capabilities are provided,
// and that the capabilities are valid. Returns an error if any validation fails.
func (cs *ControllerServer) validateCreateVolumeRequest(ctx context.Context, req *csi.CreateVolumeRequest) error {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering validateCreateVolumeRequest()", "req", req)
	defer log.V(4).Info("Exiting validateCreateVolumeRequest()")

	// Check if the volume name is empty; if so, return an error indicating no volume name was provided.
	if req.GetName() == "" {
		return errNoVolumeName
	}

	// Retrieve the volume capabilities from the request.
	volCaps := req.GetVolumeCapabilities()
	// Check if no volume capabilities are provided; if so, return an error.
	if len(volCaps) == 0 {
		return errNoVolumeCapabilities
	}
	// Validate the provided volume capabilities; if they are invalid, return an error.
	if !validVolumeCapabilities(volCaps) {
		return errInvalidVolumeCapability(volCaps)
	}

	metrics.TraceFunctionData(ctx, "ValidateCreateVolumeRequest", map[string]string{"volume_name": req.GetName(), "requestBody": metrics.SerializeRequest(req)}, metrics.TracingSubfunction, nil)
	// If all checks pass, return nil indicating the request is valid.
	return nil
}

// prepareVolumeParams prepares the volume parameters for creation.
// It extracts the capacity range from the request, calculates the size,
// and generates a normalized volume name. Returns the volume name and size in GB.
func (cs *ControllerServer) prepareVolumeParams(ctx context.Context, req *csi.CreateVolumeRequest) (*VolumeParams, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering prepareVolumeParams()", "req", req)
	defer log.V(4).Info("Exiting prepareVolumeParams()")
	// By default, encryption is disabled
	encryptionStatus := "disabled"
	// Retrieve the capacity range from the request to determine the size limits for the volume.
	capRange := req.GetCapacityRange()
	// Get the requested size in bytes, handling any potential errors.
	size, err := getRequestCapacitySize(ctx, capRange)
	if err != nil {
		return nil, err
	}

	// Get the region from req.AccessibilityRequirements if it exists. Fall back to the controller's metadata region if not specified.
	accessibilityRequirements := req.GetAccessibilityRequirements()
	region := cs.metadata.Region
	if accessibilityRequirements != nil {
		if topologyRegion := getRegionFromTopology(accessibilityRequirements); topologyRegion != "" {
			log.V(4).Info("Using region from topology", "region", topologyRegion)
			region = topologyRegion
		}
	}

	preKey := linodevolumes.CreateLinodeVolumeKey(0, req.GetName())
	volumeName := preKey.GetNormalizedLabelWithPrefix(cs.driver.volumeLabelPrefix)
	targetSizeGB := bytesToGB(size)

	// Check if encryption should be enabled
	if req.GetParameters()[VolumeEncryption] == True {
		supported, err := cs.isEncryptionSupported(ctx, region)
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, errInternal("Volume encryption is not supported in the %s region", region)
		}
		encryptionStatus = "enabled"
	}

	log.V(4).Info("Volume parameters prepared", "parameters", &VolumeParams{
		VolumeName:       volumeName,
		TargetSizeGB:     targetSizeGB,
		Size:             size,
		EncryptionStatus: encryptionStatus,
		Region:           region,
	})

	params := &VolumeParams{
		VolumeName:       volumeName,
		TargetSizeGB:     targetSizeGB,
		Size:             size,
		EncryptionStatus: encryptionStatus,
		Region:           region,
	}

	metrics.TraceFunctionData(ctx, "PrepareVolumeParams", map[string]string{
		"volume_name":      req.GetName(),
		"requestBody":      metrics.SerializeRequest(req),
		"volumeParameters": metrics.SerializeRequest(params)}, metrics.TracingSubfunction, nil)

	return params, nil
}

// createVolumeContext creates a context map for the volume based on the request parameters.
// If the volume is encrypted, it adds relevant encryption attributes to the context.
func (cs *ControllerServer) createVolumeContext(ctx context.Context, req *csi.CreateVolumeRequest, vol *linodego.Volume) map[string]string {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering createVolumeContext()", "req", req)
	defer log.V(4).Info("Exiting createVolumeContext()")

	volumeContext := make(map[string]string)

	if req.GetParameters()[LuksEncryptedAttribute] == True {
		volumeContext[LuksEncryptedAttribute] = True
		volumeContext[PublishInfoVolumeName] = req.GetName()
		volumeContext[LuksCipherAttribute] = req.GetParameters()[LuksCipherAttribute]
		volumeContext[LuksKeySizeAttribute] = req.GetParameters()[LuksKeySizeAttribute]
	}

	volumeContext[VolumeTopologyRegion] = vol.Region

	metrics.TraceFunctionData(ctx, "createVolumeContext", map[string]string{
		"requestBody":   metrics.SerializeRequest(req),
		"volumeContext": metrics.SerializeRequest(volumeContext),
	}, metrics.TracingSubfunction, nil)

	log.V(4).Info("Volume context created", "volumeContext", volumeContext)
	return volumeContext
}

// createAndWaitForVolume attempts to create a new volume and waits for it to become active.
// It logs the process and handles any errors that occur during creation or waiting.
func (cs *ControllerServer) createAndWaitForVolume(ctx context.Context, name string, parameters map[string]string, encryptionStatus string, sizeGB int, sourceInfo *linodevolumes.LinodeVolumeKey, region string) (*linodego.Volume, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering createAndWaitForVolume()", "name", name, "sizeGB", sizeGB, "tags", parameters[VolumeTags], "encryptionStatus", encryptionStatus, "region", region)
	defer log.V(4).Info("Exiting createAndWaitForVolume()")

	vol, err := cs.attemptCreateLinodeVolume(ctx, name, parameters[VolumeTags], encryptionStatus, sizeGB, sourceInfo, region)
	if err != nil {
		return nil, err
	}

	// Check if the created volume's size matches the requested size.
	// if not, and sourceInfo is nil, it indicates that the volume was not created from a source.
	if vol.Size != sizeGB && sourceInfo == nil {
		return nil, errAlreadyExists("volume %d already exists with size %d", vol.ID, vol.Size)
	}

	// Set the timeout for polling the volume status based on whether it's a clone or not.
	statusPollTimeout := waitTimeout()
	if sourceInfo != nil {
		statusPollTimeout = cloneTimeout()
	}

	log.V(4).Info("Waiting for volume to be active", "volumeID", vol.ID)
	vol, err = cs.client.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, statusPollTimeout)
	if err != nil {
		return nil, errInternal("Timed out waiting for volume %d to be active: %v", vol.ID, err)
	}

	metrics.TraceFunctionData(ctx, "CreateAndWaitForVolume",
		map[string]string{"name": name, "encryption": encryptionStatus, "size": strconv.Itoa(sizeGB), "region": region},
		metrics.TracingSubfunction, nil)

	log.V(4).Info("Volume is active", "volumeID", vol.ID)
	return vol, nil
}

// prepareCreateVolumeResponse constructs a CreateVolumeResponse from the created volume details.
// It includes the volume ID, capacity, accessible topology, and any relevant context or content source.
func (cs *ControllerServer) prepareCreateVolumeResponse(ctx context.Context, vol *linodego.Volume, size int64, volContext map[string]string, sourceInfo *linodevolumes.LinodeVolumeKey, contentSource *csi.VolumeContentSource) *csi.CreateVolumeResponse {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering prepareCreateVolumeResponse()", "vol", vol)
	defer log.V(4).Info("Exiting prepareCreateVolumeResponse()")

	key := linodevolumes.CreateLinodeVolumeKey(vol.ID, vol.Label)
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      key.GetVolumeKey(),
			CapacityBytes: size,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						VolumeTopologyRegion: vol.Region,
					},
				},
			},
			VolumeContext: volContext,
		},
	}

	if sourceInfo != nil {
		resp.Volume.ContentSource = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: contentSource.GetVolume().GetVolumeId(),
				},
			},
		}
	}

	return resp
}

// validateControllerPublishVolumeRequest validates the incoming ControllerPublishVolumeRequest.
// It extracts the Linode ID and Volume ID from the request and checks if the
// volume capability is provided and valid. If any validation fails, it returns
// an appropriate error.
func (cs *ControllerServer) validateControllerPublishVolumeRequest(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (linodeID, volumeID int, err error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering validateControllerPublishVolumeRequest()", "req", req)
	defer log.V(4).Info("Exiting validateControllerPublishVolumeRequest()")

	// extract the linode ID from the request
	linodeID, err = linodevolumes.NodeIdAsInt("ControllerPublishVolume", req)
	if err != nil {
		return 0, 0, err
	}

	// extract the volume ID from the request
	volumeID, err = linodevolumes.VolumeIdAsInt("ControllerPublishVolume", req)
	if err != nil {
		return 0, 0, err
	}

	// retrieve the volume capability from the request
	volCap := req.GetVolumeCapability()
	// return an error if no volume capability is provided
	if volCap == nil {
		return 0, 0, errNoVolumeCapability
	}
	// return an error if the volume capability is invalid
	if !validVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return 0, 0, errInvalidVolumeCapability([]*csi.VolumeCapability{volCap})
	}

	metrics.TraceFunctionData(ctx, "validateControllerPublishVolumeRequest", map[string]string{
		"requestBody":      metrics.SerializeRequest(req),
		"volumeCapability": metrics.SerializeRequest(volCap),
		"linodeId":         strconv.Itoa(linodeID),
		"volumeId":         strconv.Itoa(volumeID),
	}, metrics.TracingSubfunction, nil)

	log.V(4).Info("Validation passed", "linodeID", linodeID, "volumeID", volumeID)
	return linodeID, volumeID, nil
}

// getAndValidateVolume retrieves the volume by its ID and run checks.
//
// It performs the following checks:
//  1. If the volume is found and already attached to the specified Linode instance,
//     it returns the device path of the volume.
//  2. If the volume is not found, it returns an error indicating that the volume does not exist.
//  3. If the volume is attached to a different instance, it returns an error indicating
//     that the volume is already attached elsewhere.
//
// Additionally, it checks if the volume and instance are in the same region based on
// the provided volume context. If they are not in the same region, it returns an internal error.
func (cs *ControllerServer) getAndValidateVolume(ctx context.Context, volumeID int, instance *linodego.Instance) (string, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering getAndValidateVolume()", "volumeID", volumeID, "linodeID", instance.ID)
	defer log.V(4).Info("Exiting getAndValidateVolume()")

	volume, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		return "", errVolumeNotFound(volumeID)
	} else if err != nil {
		return "", errInternal("get volume %d: %v", volumeID, err)
	}

	if volume.LinodeID != nil {
		if *volume.LinodeID == instance.ID {
			log.V(4).Info("Volume already attached to instance", "volume_id", volume.ID, "node_id", *volume.LinodeID, "device_path", volume.FilesystemPath)
			return volume.FilesystemPath, nil
		}
		return "", errVolumeAttached(volumeID, instance.ID)
	}

	// check if the volume and instance are in the same region
	if instance.Region != volume.Region {
		return "", errRegionMismatch(volume.Region, instance.Region)
	}

	metrics.TraceFunctionData(ctx, "GetAndValidateVolume", map[string]string{
		"volumeSpecs": metrics.SerializeRequest(volume),
	}, metrics.TracingSubfunction, nil)

	log.V(4).Info("Volume validated and is not attached to instance", "volume_id", volume.ID, "node_id", instance.ID)
	return "", nil
}

// getInstance retrieves the Linode instance by its ID. If the
// instance is not found, it returns an error indicating that the instance
// does not exist. If any other error occurs during retrieval, it returns
// an internal error.
func (cs *ControllerServer) getInstance(ctx context.Context, linodeID int) (*linodego.Instance, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering getInstance()", "linodeID", linodeID)
	defer log.V(4).Info("Exiting getInstance()")

	instance, err := cs.client.GetInstance(ctx, linodeID)
	if linodego.IsNotFound(err) {
		return nil, errInstanceNotFound(linodeID)
	} else if err != nil {
		// If any other error occurs, return an internal error.
		return nil, errInternal("get linode instance %d: %v", linodeID, err)
	}

	metrics.TraceFunctionData(ctx, "GetInstance", map[string]string{
		"instanceSpecs": metrics.SerializeRequest(instance),
	}, metrics.TracingSubfunction, nil)

	log.V(4).Info("Instance retrieved", "instance", instance)
	return instance, nil
}

// checkAttachmentCapacity checks if the specified instance can accommodate
// additional volume attachments. It retrieves the maximum number of allowed
// attachments and compares it with the currently attached volumes. If the
// limit is exceeded, it returns an error indicating the maximum volume
// attachments allowed.
func (cs *ControllerServer) checkAttachmentCapacity(ctx context.Context, instance *linodego.Instance) error {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering checkAttachmentCapacity()", "linodeID", instance.ID)
	defer log.V(4).Info("Exiting checkAttachmentCapacity()")

	canAttach, err := cs.canAttach(ctx, instance)
	if err != nil {
		return err
	}
	if !canAttach {
		// If the instance cannot accommodate more attachments, retrieve the maximum allowed attachments.
		limit, err := cs.maxAllowedVolumeAttachments(ctx, instance)
		if errors.Is(err, errNilInstance) {
			return errInternal("cannot calculate max volume attachments for a nil instance")
		} else if err != nil {
			return errMaxAttachments // Return an error indicating the maximum attachments limit has been reached.
		}
		return errMaxVolumeAttachments(limit) // Return an error indicating the maximum volume attachments allowed.
	}

	metrics.TraceFunctionData(ctx, "CheckAttachmentCapacity", map[string]string{
		"instanceSpecs": metrics.SerializeRequest(instance),
	}, metrics.TracingSubfunction, nil)

	return nil // Return nil if the instance can accommodate more attachments.
}

// attachVolume attaches the specified volume to the given Linode instance.
// It logs the action and handles any errors that may occur during the
// attachment process. If the volume is already attached, it allows for a
// retry by returning an Unavailable error.
func (cs *ControllerServer) attachVolume(ctx context.Context, volumeID, linodeID int) error {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Entering attachVolume()", "volume_id", volumeID, "node_id", linodeID)
	defer log.V(4).Info("Exiting attachVolume()")

	persist := false
	_, err := cs.client.AttachVolume(ctx, volumeID, &linodego.VolumeAttachOptions{
		LinodeID:           linodeID,
		PersistAcrossBoots: &persist,
	})
	if err != nil {
		code := codes.Internal // Default error code is Internal.
		// Check if the error indicates that the volume is already attached.
		var apiErr *linodego.Error
		if errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "is already attached") {
			code = codes.Unavailable // Allow a retry if the volume is already attached: race condition can occur here
		}
		return status.Errorf(code, "attach volume: %v", err)
	}

	metrics.TraceFunctionData(ctx, "AttachVolume", map[string]string{
		"volumeID": strconv.Itoa(volumeID),
		"linodeID": strconv.Itoa(linodeID),
	}, metrics.TracingError, nil)

	return nil // Return nil if the volume is successfully attached.
}
