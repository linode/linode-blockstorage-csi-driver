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

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
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
)

type ControllerServer struct {
	driver   *LinodeDriver
	client   linodeclient.LinodeClient
	metadata Metadata

	csi.UnimplementedControllerServer
}

// NewControllerServer instantiates a new RPC service that implements the
// CSI [Controller Service RPC] endpoints.
//
// If driver or client are nil, NewControllerServer returns a non-nil error.
//
// [Controller Service RPC]: https://github.com/container-storage-interface/spec/blob/master/spec.md#controller-service-rpc
func NewControllerServer(ctx context.Context, driver *LinodeDriver, client linodeclient.LinodeClient, metadata Metadata) (*ControllerServer, error) {
	log := logger.GetLogger(ctx)

	log.V(4).Info("Creating new ControllerServer")

	if driver == nil {
		log.Error(nil, "LinodeDriver is nil")
		return nil, errNilDriver
	}
	if client == nil {
		log.Error(nil, "Linode client is nil")
		return nil, errors.New("nil client")
	}

	cs := &ControllerServer{
		driver:   driver,
		client:   client,
		metadata: metadata,
	}

	log.V(4).Info("ControllerServer created successfully")
	return cs, nil
}

// CreateVolume will be called by the CO to provision a new volume on behalf of a user (to be consumed
// as either a block device or a mounted filesystem).  This operation is idempotent.
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("CreateVolume")
	defer done()

	name := req.GetName()
	log.V(2).Info("Processing request", "req", req)

	if len(name) == 0 {
		return &csi.CreateVolumeResponse{}, errNoVolumeName
	}

	// validate volume capabilities
	volCapabilities := req.GetVolumeCapabilities()
	if len(volCapabilities) == 0 {
		return &csi.CreateVolumeResponse{}, errNoVolumeCapabilities
	}
	if !validVolumeCapabilities(volCapabilities) {
		return &csi.CreateVolumeResponse{}, errInvalidVolumeCapability(volCapabilities)
	}

	capRange := req.GetCapacityRange()
	size, err := getRequestCapacitySize(capRange)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// to avoid mangled requests for existing volumes with hyphen,
	// we only strip them out on creation when k8s invented the name
	// this is still problematic because we strip "-" from volume-name-prefixes
	// that specifically requested "-".
	// Don't strip this when volume labels support sufficient length
	condensedName := strings.Replace(name, "-", "", -1)

	preKey := linodevolumes.CreateLinodeVolumeKey(0, condensedName)

	volumeName := preKey.GetNormalizedLabelWithPrefix(cs.driver.volumeLabelPrefix)
	targetSizeGB := bytesToGB(size)

	log.V(4).Info("CreateVolume details", "storage_size_giga_bytes", targetSizeGB, "volume_name", volumeName)

	volumeContext := make(map[string]string)
	if req.Parameters[LuksEncryptedAttribute] == "true" {
		// if luks encryption is enabled add a volume context
		volumeContext[LuksEncryptedAttribute] = "true"
		volumeContext[PublishInfoVolumeName] = volumeName
		volumeContext[LuksCipherAttribute] = req.Parameters[LuksCipherAttribute]
		volumeContext[LuksKeySizeAttribute] = req.Parameters[LuksKeySizeAttribute]
	}

	// Attempt to get info about the source volume for
	// volume cloning if the datasource is provided in the PVC.
	// sourceVolumeInfo will be null if no content source is defined.
	contentSource := req.GetVolumeContentSource()
	sourceVolumeInfo, err := cs.attemptGetContentSourceVolume(ctx, contentSource)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// Attempt to create the volume while respecting idempotency.
	// If the content source is defined, the source volume will be cloned to create a new volume.
	log.V(4).Info("Calling API to create volume", "volumeName", volumeName)
	vol, err := cs.attemptCreateLinodeVolume(
		ctx,
		volumeName,
		targetSizeGB,
		req.Parameters[VolumeTags],
		sourceVolumeInfo,
	)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// If the existing volume size differs from the requested size, we throw an error.
	if vol.Size != targetSizeGB {
		if sourceVolumeInfo == nil {
			return nil, errAlreadyExists("volume %d already exists of size %d", vol.ID, vol.Size)
		}
	}

	statusPollTimeout := waitTimeout()

	// If we're cloning the volume we should extend the timeout
	if sourceVolumeInfo != nil {
		statusPollTimeout = cloneTimeout()
	}

	log.V(4).Info("Waiting for volume to be active", "volumeID", vol.ID)
	if _, err := cs.client.WaitForVolumeStatus(
		ctx, vol.ID, linodego.VolumeActive, statusPollTimeout); err != nil {
		return &csi.CreateVolumeResponse{}, errInternal("Timed out waiting for volume %d to be active: %v", vol.ID, err)
	}

	log.V(4).Info("Volume is active", "volumeID", vol.ID)

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
			VolumeContext: volumeContext,
		},
	}

	// Append the content source to the response
	if sourceVolumeInfo != nil {
		resp.Volume.ContentSource = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: contentSource.GetVolume().GetVolumeId(),
				},
			},
		}
	}

	log.V(2).Info("Volume created successfully", "response", resp)
	return resp, nil
}

// DeleteVolume deletes the given volume. The function is idempotent.
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("DeleteVolume")
	defer done()

	volID, statusErr := linodevolumes.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		return &csi.DeleteVolumeResponse{}, statusErr
	}

	log.V(2).Info("Processing request", "req", req)

	// Check if the volume exists
	log.V(4).Info("Checking if volume exists", "volume_id", volID)
	vol, err := cs.client.GetVolume(ctx, volID)
	if linodego.IsNotFound(err) {
		return &csi.DeleteVolumeResponse{}, nil
	} else if err != nil {
		return &csi.DeleteVolumeResponse{}, errInternal("get volume %d: %v", volID, err)
	}
	if vol.LinodeID != nil {
		return &csi.DeleteVolumeResponse{}, errVolumeInUse
	}

	// Delete the volume
	log.V(4).Info("Deleting volume", "volume_id", volID)
	if err := cs.client.DeleteVolume(ctx, volID); err != nil {
		return &csi.DeleteVolumeResponse{}, errInternal("delete volume %d: %v", volID, err)
	}

	log.V(2).Info("Volume deleted successfully", "volume_id", volID)
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches the given volume to the node
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("ControllerPublishVolume")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	linodeID, statusErr := linodevolumes.NodeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerPublishVolumeResponse{}, statusErr
	}

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerPublishVolumeResponse{}, statusErr
	}

	cap := req.GetVolumeCapability()
	if cap == nil {
		return &csi.ControllerPublishVolumeResponse{}, errNoVolumeCapability
	}
	if !validVolumeCapabilities([]*csi.VolumeCapability{cap}) {
		return &csi.ControllerPublishVolumeResponse{}, errInvalidVolumeCapability([]*csi.VolumeCapability{cap})
	}

	volume, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		return &csi.ControllerPublishVolumeResponse{}, errVolumeNotFound(volumeID)
	} else if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, errInternal("get volume %d: %v", volumeID, err)
	}
	if volume.LinodeID != nil {
		if *volume.LinodeID == linodeID {
			log.V(4).Info("Volume already attached to instance", "volume_id", volume.ID, "node_id", *volume.LinodeID, "device_path", volume.FilesystemPath)
			pvInfo := map[string]string{
				devicePathKey: volume.FilesystemPath,
			}
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: pvInfo,
			}, nil
		}
		return &csi.ControllerPublishVolumeResponse{}, errVolumeAttached(volumeID, linodeID)
	}

	instance, err := cs.client.GetInstance(ctx, linodeID)
	if linodego.IsNotFound(err) {
		return &csi.ControllerPublishVolumeResponse{}, errInstanceNotFound(linodeID)
	} else if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, errInternal("get linode instance %d: %v", linodeID, err)
	}

	log.V(4).Info("Checking if volume can be attached", "volume_id", volumeID, "node_id", linodeID)
	// Check to see if there is room to attach this volume to the instance.
	if canAttach, err := cs.canAttach(ctx, instance); err != nil {
		return &csi.ControllerPublishVolumeResponse{}, err
	} else if !canAttach {
		// If we can, try and add a little more information to the error message
		// for the caller.
		limit, err := cs.maxVolumeAttachments(ctx, instance)
		if errors.Is(err, errNilInstance) {
			return &csi.ControllerPublishVolumeResponse{}, errInternal("cannot calculate max volume attachments for a nil instance")
		} else if err != nil {
			return &csi.ControllerPublishVolumeResponse{}, errMaxAttachments
		}
		return &csi.ControllerPublishVolumeResponse{}, errMaxVolumeAttachments(limit)
	}

	// Whether or not the volume attachment should be persisted across
	// boots.
	//
	// Setting this to true will limit the maximum number of attached
	// volumes to 8 (eight), minus any instance disks, since volume
	// attachments get persisted by adding them to the instance's boot
	// config.
	persist := false

	log.V(4).Info("Executing attach volume", "volume_id", volumeID, "node_id", linodeID)
	if _, err := cs.client.AttachVolume(ctx, volumeID, &linodego.VolumeAttachOptions{
		LinodeID:           linodeID,
		PersistAcrossBoots: &persist,
	}); err != nil {
		code := codes.Internal
		if apiErr, ok := err.(*linodego.Error); ok && strings.Contains(apiErr.Message, "is already attached") {
			code = codes.Unavailable // Allow a retry if the volume is already attached: race condition can occur here
		}
		return &csi.ControllerPublishVolumeResponse{}, status.Errorf(code, "attach volume: %v", err)
	}

	log.V(4).Info("Waiting for volume to attach", "volume_id", volumeID)
	volume, err = cs.client.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout())
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, errInternal("wait for volume to attach: %v", err)
	}

	log.V(2).Info("Volume attached successfully", "volume_id", volume.ID, "node_id", *volume.LinodeID, "device_path", volume.FilesystemPath)

	pvInfo := map[string]string{
		devicePathKey: volume.FilesystemPath,
	}
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: pvInfo,
	}, nil
}

// devicePathKey is the key used in the publish context map when a volume is
// published/attached to an instance.
const devicePathKey = "devicePath"

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

// ControllerUnpublishVolume deattaches the given volume from the node
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("ControllerUnpublishVolume")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, statusErr
	}

	linodeID, statusErr := linodevolumes.NodeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, statusErr
	}

	log.V(4).Info("Checking if volume is attached", "volume_id", volumeID, "node_id", linodeID)
	volume, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		log.V(4).Info("Volume not found, skipping", "volume_id", volumeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("get volume %d: %v", volumeID, err)
	}
	if volume.LinodeID != nil && *volume.LinodeID != linodeID {
		log.V(4).Info("Volume attached to different instance, skipping", "volume_id", volumeID, "attached_node_id", *volume.LinodeID, "requested_node_id", linodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	log.V(4).Info("Executing detach volume", "volume_id", volumeID, "node_id", linodeID)
	if err := cs.client.DetachVolume(ctx, volumeID); linodego.IsNotFound(err) {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("detach volume %d: %v", volumeID, err)
	}

	log.V(4).Info("Waiting for volume to detach", "volume_id", volumeID, "node_id", linodeID)
	if _, err := cs.client.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout()); err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("wait for volume %d to detach: %v", volumeID, err)
	}

	log.V(2).Info("Volume detached successfully", "volume_id", volumeID)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("ValidateVolumeCapabilities")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerValidateVolumeCapabilities", req)
	if statusErr != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{}, statusErr
	}

	volumeCapabilities := req.GetVolumeCapabilities()
	if len(volumeCapabilities) == 0 {
		return &csi.ValidateVolumeCapabilitiesResponse{}, errNoVolumeCapabilities
	}

	if _, err := cs.client.GetVolume(ctx, volumeID); linodego.IsNotFound(err) {
		return &csi.ValidateVolumeCapabilitiesResponse{}, errVolumeNotFound(volumeID)
	} else if err != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{}, errInternal("get volume: %v", err)
	}


	resp := &csi.ValidateVolumeCapabilitiesResponse{}
	if validVolumeCapabilities(volumeCapabilities) {
		resp.Confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volumeCapabilities}
	}
	log.V(2).Info("Supported capabilities", "response", resp)

	return resp, nil
}

// ListVolumes shall return information about all the volumes the provider knows about
func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("ListVolumes")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	startingToken := req.GetStartingToken()
	nextToken := ""

	listOpts := linodego.NewListOptions(0, "")
	if req.GetMaxEntries() > 0 {
		listOpts.PageSize = int(req.GetMaxEntries())
	}

	if startingToken != "" {
		startingPage, errParse := strconv.ParseInt(startingToken, 10, 64)
		if errParse != nil {
			return &csi.ListVolumesResponse{}, status.Errorf(codes.Aborted, "invalid starting token: %q", startingToken)
		}

		listOpts.Page = int(startingPage)
		nextToken = strconv.Itoa(listOpts.Page + 1)
	}


	// List all volumes
	log.V(4).Info("Listing volumes", "list_opts", listOpts)
	volumes, err := cs.client.ListVolumes(ctx, listOpts)
	if err != nil {
		return &csi.ListVolumesResponse{}, errInternal("list volumes: %v", err)
	}

	entries := make([]*csi.ListVolumesResponse_Entry, 0, len(volumes))
	for _, vol := range volumes {
		key := linodevolumes.CreateLinodeVolumeKey(vol.ID, vol.Label)

		// If the volume is attached to a Linode instance, add it to the list.
		//
		// Note that in the Linode API, volumes can only be attached to a single
		// Linode at a time.
		// We are storing it in a []string here, since that is what the
		// response struct returns.
		// We do not need to pre-allocate the slice with make(), since the CSI
		// specification says this response field is optional, and thus it
		// should tolerate a nil slice.
		var publishedNodeIDs []string
		if vol.LinodeID != nil {
			publishedNodeIDs = append(publishedNodeIDs, strconv.Itoa(*vol.LinodeID))
		}

		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      key.GetVolumeKey(),
				CapacityBytes: gbToBytes(vol.Size),
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{
							VolumeTopologyRegion: vol.Region,
						},
					},
				},
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: publishedNodeIDs,
				VolumeCondition: &csi.VolumeCondition{
					Abnormal: false,
				},
			},
		})
	}

	resp := &csi.ListVolumesResponse{
		Entries:   entries,
		NextToken: nextToken,
	}

	log.V(2).Info("Volumes listed", "response", resp)
	return resp, nil
}

// ControllerGetCapabilities returns the supported capabilities of controller service provided by this Plugin
func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("ControllerGetCapabilities")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.driver.cscap,
	}

	log.V(2).Info("ControllerGetCapabilities called", "response", resp)
	return resp, nil
}

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("ControllerExpandVolume")
	defer done()
	
	log.V(2).Info("Processing request", "req", req)

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerExpandVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	size, err := getRequestCapacitySize(req.GetCapacityRange())
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("get requested size from capacity range: %v", err)
	}

	// Get the volume
	log.V(4).Info("Checking if volume exists", "volume_id", volumeID)
	vol, err := cs.client.GetVolume(ctx, volumeID)
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("get volume: %v", err)
	}

	// Is the caller trying to resize the volume to be smaller than it currently is?
	if vol.Size > bytesToGB(size) {
		return &csi.ControllerExpandVolumeResponse{}, errResizeDown
	}

	// Resize the volume
	log.V(4).Info("Calling API to resize volume", "volume_id", volumeID)
	if err := cs.client.ResizeVolume(ctx, volumeID, bytesToGB(size)); err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("resize volume %d: %v", volumeID, err)
	}

	// Wait for the volume to become active
	log.V(4).Info("Waiting for volume to become active", "volume_id", volumeID)
	vol, err = cs.client.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout())
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("timed out waiting for volume %d to become active: %v", volumeID, err)
	}
	log.V(4).Info("Volume active", "vol", vol)

	log.V(2).Info("Volume resized successfully", "volume_id", volumeID)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         size,
		NodeExpansionRequired: false,
	}, nil
}

// attemptGetContentSourceVolume attempts to get information about the Linode volume to clone from.
func (cs *ControllerServer) attemptGetContentSourceVolume(ctx context.Context, contentSource *csi.VolumeContentSource) (*linodevolumes.LinodeVolumeKey, error) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Attempting to get content source volume")

	// No content source was defined; no clone operation
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

// attemptCreateLinodeVolume attempts to create a volume while respecting
// idempotency.
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

// createLinodeVolume creates a Linode volume and returns the result
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

// cloneLinodeVolume clones a Linode volume and returns the result
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

// getRequestCapacity evaluates the CapacityRange parameters to validate and resolve the best volume size
func getRequestCapacitySize(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return MinVolumeSizeBytes, nil
	}

	// Volume MUST be at least this big. This field is OPTIONAL.
	reqSize := capRange.GetRequiredBytes()

	// Volume MUST not be bigger than this. This field is OPTIONAL.
	maxSize := capRange.GetLimitBytes()

	// At least one of the these fields MUST be specified.
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
