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
	"k8s.io/klog/v2"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
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

// MaxVolumeLabelLength is the maximum allowed length of a label on a block
// storage volume.
//
// NOTE: It is unclear if this limit is self-imposed, or from the Linode API.
const MaxVolumeLabelLength = 32

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
func NewControllerServer(driver *LinodeDriver, client linodeclient.LinodeClient, metadata Metadata) (*ControllerServer, error) {
	if driver == nil {
		return nil, errNilDriver
	}
	if client == nil {
		return nil, errors.New("nil client")
	}

	return &ControllerServer{
		driver:   driver,
		client:   client,
		metadata: metadata,
	}, nil
}

// CreateVolume will be called by the CO to provision a new volume on behalf of a user (to be consumed
// as either a block device or a mounted filesystem).  This operation is idempotent.
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Quick check to make sure the caller filled in the volume name.
	name := req.GetName()
	if name == "" {
		return &csi.CreateVolumeResponse{}, errNoVolumeName
	}

	// Make sure the caller provided some volume capabilities, and make sure
	// they only specified ones we can support.
	capabilities := req.GetVolumeCapabilities()
	if len(capabilities) == 0 {
		return &csi.CreateVolumeResponse{}, errNoVolumeCapabilities
	}
	for _, c := range capabilities {
		if c == nil {
			continue
		}
		switch mode := c.GetAccessMode().GetMode(); mode {
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER:
		// This is ok.
		default:
			return &csi.CreateVolumeResponse{}, errInvalidArgument("unsupported volume access mode: %q", mode)
		}
	}

	// Check the capacity range.
	//
	// The main things we want to check, are to make sure that the caller
	// doesn't specify a range that is smaller than the minimum volume size
	// [MinVolumeSizeBytes].
	//
	// A capacity range has two values: required, and limit.
	// Both values indicate the number of bytes.
	// The "required" bytes is the minimum size the caller will tolerate,
	// and the "limit" bytes is the upper bound.
	// The caller does not need to specify both values.
	// They can specify one value, and as long as it is larger than
	// [MinVolumeSizeBytes], we will use it.
	// If the caller does not specify a capacity, (specifying a capacity range
	// is optional, as per the CSI spec) we will assume [MinVolumeSizeBytes].
	// If both the required and limit sizes are specified, we will choose the
	// larger of the two values.
	//
	// Note that the capacity range is optional.
	var (
		capacity = req.GetCapacityRange()

		// This funny use of max() is a shortcut to ensure that we get the
		// larger of required or limit bytes, even if one of them is not
		// specified.
		// If neither are specified, the result will be 0 (zero).
		requiredSize = max(capacity.GetRequiredBytes(), capacity.GetLimitBytes())
		limitSize    = max(capacity.GetLimitBytes(), capacity.GetRequiredBytes())

		// Get the larger of the required and limit sizes.
		desiredSize = max(requiredSize, limitSize)
	)
	// Remember, the capacity range is optional.
	// If the caller did not specify a size, both requiredSize and limitSize
	// will be 0 (zero), and we will default to [MinVolumeSizeBytes].
	// The assumption is that the user wants a volume, they just don't
	// necessarily care what size it is.
	//
	// However, if the caller specified a size that is less-than
	// [MinVolumeSizeBytes], we will error out.
	if desiredSize == 0 {
		desiredSize = MinVolumeSizeBytes
	} else if desiredSize < MinVolumeSizeBytes {
		return &csi.CreateVolumeResponse{}, errSmallVolumeCapacity
	}

	// Make sure the caller does not want to create a new volume by cloning
	// a snapshot.
	// Linode does not support snapshotting block storage volumes.
	if req.GetVolumeContentSource().GetSnapshot() != nil {
		return &csi.CreateVolumeResponse{}, errSnapshot
	}

	// Strip out any hyphens "-" from the volume name provided in the request.
	// When we formulate a new volume name to return, it will be in the format
	//
	//    ID-[PREFIX]LABEL
	//
	// where "ID" is the Linode volume's ID, and "LABEL" is the name supplied
	// in the request.
	// If the driver was initialized with a volume label prefix, it will be
	// prepended to the LABEL part of the ID.
	// The full volume ID will be set later.
	// For now, we will prepare the "label" part of the ID.
	stripped := []rune(strings.ReplaceAll(name, "-", ""))
	stripped = append(stripped, []rune(cs.driver.volumeLabelPrefix)...)

	var volumeName string
	if len(stripped) > MaxVolumeLabelLength {
		volumeName = string(stripped[:MaxVolumeLabelLength])
	} else {
		volumeName = string(stripped)
	}

	// Build up the volume context.
	//
	// The volume context is a map of key-value pairs that carry information
	// about the volume through to later steps.
	//
	// In this case, we want to propagate the LUKS-related information.
	volumeContext := make(map[string]string)
	if req.Parameters[LuksEncryptedAttribute] == "true" {
		// if luks encryption is enabled add a volume context
		volumeContext[LuksEncryptedAttribute] = "true"
		volumeContext[PublishInfoVolumeName] = volumeName
		volumeContext[LuksCipherAttribute] = req.Parameters[LuksCipherAttribute]
		volumeContext[LuksKeySizeAttribute] = req.Parameters[LuksKeySizeAttribute]
	}

	// Now, it is time to see about creating the volume.
	//
	// There are two ways we support:
	//  - Creating the volume from scratch
	//  - Cloning an existing volume
	//
	// In either case, we are going to call another method to handle the
	// creation process, but this method is going to handle sending the
	// response to the caller.
	var newVolume newVolumeFunc = cs.createVolume
	if req.GetVolumeContentSource().GetVolume() != nil {
		newVolume = cs.cloneVolume
	}

	volume, err := newVolume(ctx, req)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}
}

// newVolumeFunc is a functional type that creates a new
// [github.com/linode/linodego.Volume] given the parameters in
// [github.com/container-storage-interface/spec/lib/go/csi.CreateVolumeRequest].
type newVolumeFunc func(context.Context, *csi.CreateVolumeRequest) (*linodego.Volume, error)

// createVolume creates a brand new Linode block storage volume.
// It is an implementation of a [newVolumeFunc].
func (cs *ControllerServer) createVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*linodego.Volume, error) {
	// Check to see if a volume with the name already exists.
	// If it is in the right region, and not currently attached to another node
	// we will use it.
	//
	// Since we do not have the ID of an existing volume handy, we will have to
	// search for it by label.
	stripped := []rune(strings.ReplaceAll(name, "-", ""))
	stripped = append(stripped, []rune(cs.driver.volumeLabelPrefix)...)

	var label string
	if len(stripped) > MaxVolumeLabelLength {
		label = string(stripped[:MaxVolumeLabelLength])
	} else {
		label = string(stripped)
	}

	volume, err := cs.getVolumeByLabel(ctx, label)
	if err != nil {
		return nil, err
	}
	if volume != nil {
		return cs.useExistingVolume(ctx, req, volume)
	}

	return nil, errNotImplemented
}

// getVolumeByLabel retrieves a single block storage volume from the Linode API
// by its label.
// If there are no volumes with the given label, getVolumeByLabel will return
// a nil [github.com/linode/linodego.Volume], and a nil error.
func (cs *ControllerServer) getVolumeByLabel(ctx context.Context, label string) (*linodego.Volume, error) {
	filter, err := json.Marshal(map[string]string{
		"label": label,
	})
	if err != nil {
		return nil, errInternal("marshal json filter: %v", err)
	}

	volumes, err := cs.client.ListVolumes(ctx, linodego.NewListOptions(0, string(filter)))
	if err != nil {
		return nil, errInternal("list volumes: %v", err)
	}

	if len(volumes) > 1 {
		return nil, status.Errorf(codes.AlreadyExists, "too many volumes with label %q", label)
	}

	if len(volumes) == 1 {
		return &volumes[0], nil
	}
	return nil, nil
}

// useExistingVolume checks to see if the given volume can be used to satisfy
// the request.
// This method should only be called if the volume name provided in the request
// already exists as a label on a block storage volume that exists in the
// Linode API.
func (cs *ControllerServer) useExistingVolume(ctx context.Context, req *csi.CreateVolumeRequest, volume *linodego.Volume) (*linodego.Volume, error) {
	if req == nil {
		return nil, errInternal("nil request")
	}
	if volume == nil {
		return nil, errInternal("cannot use existing nil volume")
	}

	// If the volume is currently attached to a node, we cannot use it.
	if volume.LinodeID != nil {
		return nil, errVolumeAttached(volume.ID, *volume.LinodeID)
	}

	// Check if the existing volume can satisfy the request.
	//
	// First, we will check to see if the block storage volume is in the right
	// region.
	// While we cannot know which node the volume will be attached to ahead of
	// time, the container orchestrator (CO) should have provided us some
	// topology information to indicate which region the volume needs to be in.
	// If the volume is not in the specified region, we will error out.
	// There is not much we can do about the volume being in the wrong region,
	// since volume labels are unique across an entire account.
	//
	// If no topology region is specified in the request, we will default to
	// the region the controller server is running in.
	desiredRegion := cs.metadata.Region
	if accessibility := req.GetAccessibilityRequirements(); accessibility != nil {
		// Build up a slice of the preferred topologies, followed by the
		// requisite ("required") topologies.
		//
		// NOTE: We put the requisite topologies second, so that they are
		// evaluated last, and have a chance to override any preferred
		// topology.
		var topologies []*csi.Topology = append(accessibility.GetPreferred(), accessibility.GetRequisite()...)
		for _, topology := range topologies {
			for key, value := range topology.GetSegments() {
				switch key {
				case "topology.kubernetes.io/region", VolumeTopologyRegion:
					// NOTE: [VolumeTopologyRegion] is the current label key
					// used to specify the region to the CSI driver.
					// The addition of "topology.kubernetes.io/region" is the
					// [well-known label] that *should* be used going forward.
					//
					// [well-known label]: https://kubernetes.io/docs/reference/labels-annotations-taints/#topologykubernetesioregion
					if value != "" {
						desiredRegion = value
					}
				}
			}
		}
	}
	if volume.Region != desiredRegion {
		return nil, errRegionMismatch(volume.Region, desiredRegion)
	}

	// Now check the to make sure the existing volume's capacity is at-least
	// as large as the required capacity, if the capacity range has been set.
	//
	// If the capacity range does not set a required capacity, but does set
	// a limit, the volume must be no more than the limit.
	//
	// If the existing volume's capacity is larger than the limit (if set),
	// we will error out, because we cannot resize the volume to make it
	// smaller.
	//
	// If a capacity range was not set in the request, we will allow the
	// volume.
	var (
		requiredSize = req.GetCapacityRange().GetRequiredBytes()
		limitSize    = req.GetCapacityRange().GetLimitBytes()
		currentSize  = gbToBytes(volume.Size)
	)
	if requiredSize > 0 && currentSize < requiredSize {
		return nil, status.Error(codes.AlreadyExists, "volume is smaller than the required size")
	}
	if limitSize > 0 && currentSize > limitSize {
		return nil, status.Error(codes.AlreadyExists, "volume is larger than the requested capacity limit")
	}

	return nil, errNotImplemented
}

// desiredSize returns the desired capacity (in bytes) specified in the request.
// Of the "request" and "limit" bytes, the larger of the two values will be
// returned.
//
// desiredSize returns a non-nil error if the capacity range was set in the
// request, and either the "request" or "limit" bytes are < 0.
func desiredSize(req *csi.CreateVolumeRequest) (int64, error) {
	capacity := req.GetCapacityRange()
	if !isCapacitySet(capacity) {
		return 0, nil
	}

	// This funny use of max() is a shortcut to ensure that we get the
	// larger of required or limit bytes, even if one of them is not
	// specified.
	// If neither are specified, the result will be 0 (zero).
	required := max(capacity.GetRequiredBytes(), capacity.GetLimitBytes())
	limit := max(capacity.GetLimitBytes(), capacity.GetRequiredBytes())

	return max(required, limit), nil
}

// isCapacitySet indicates whether or not the capacity range was set.
// A nil capacity range, or a non-nil capacity range where the required and
// limit bytes are 0 (zero), are treated as though the capacity range was not
// set.
func isCapacitySet(capacity *csi.CapacityRange) bool {
	// Short-circuit the rest of this function if we were given a nil capacity
	// range.
	if capacity == nil {
		return false
	}

	// If the capacity range is non-nil, but both the "required" and "limit"
	// bytes are 0 (zero), we are going to assume it was not set.
	if capacity.GetRequiredBytes() == 0 && capacity.GetLimitBytes() == 0 {
		return false
	}

	return true
}

// cloneVolume creates a new Linode block storage volume by cloning an existing
// block storage volume.
//
// If a larger size/capacity is requested, cloneVolume will resize the volume
// after it is created.
// Note that a volume cannot be resized to be smaller, so if a capacity range
// is specified that is smaller than the source block storage volume,
// cloneVolume will return a non-nil error before attempting to create the new
// block storage volume.
func (cs *ControllerServer) cloneVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*linodego.Volume, error) {
	return nil, errNotImplemented
}

// 	// Attempt to get info about the source volume.
// 	// sourceVolumeInfo will be null if no content source is defined.
// 	contentSource := req.GetVolumeContentSource()
// 	sourceVolumeInfo, err := cs.attemptGetContentSourceVolume(ctx, contentSource)
// 	if err != nil {
// 		return &csi.CreateVolumeResponse{}, err
// 	}

// 	// Attempt to create the volume while respecting idempotency
// 	vol, err := cs.attemptCreateLinodeVolume(
// 		ctx,
// 		volumeName,
// 		targetSizeGB,
// 		req.Parameters[VolumeTags],
// 		sourceVolumeInfo,
// 	)
// 	if err != nil {
// 		return &csi.CreateVolumeResponse{}, err
// 	}

// 	// Attempt to resize the volume if necessary
// 	if vol.Size != targetSizeGB {
// 		klog.V(4).Infoln("resizing volume", map[string]interface{}{
// 			"volume_id": vol.ID,
// 			"old_size":  vol.Size,
// 			"new_size":  targetSizeGB,
// 		})

// 		if err := cs.client.ResizeVolume(ctx, vol.ID, targetSizeGB); err != nil {
// 			return &csi.CreateVolumeResponse{}, errInternal("resize cloned volume (%d): %v", targetSizeGB, err)
// 		}
// 	}

// 	statusPollTimeout := waitTimeout()

// 	// If we're cloning the volume we should extend the timeout
// 	if sourceVolumeInfo != nil {
// 		statusPollTimeout = cloneTimeout()
// 	}

// 	if _, err := cs.client.WaitForVolumeStatus(
// 		ctx, vol.ID, linodego.VolumeActive, statusPollTimeout); err != nil {
// 		return &csi.CreateVolumeResponse{}, errInternal("wait for volume %d to be active: %v", vol.ID, err)
// 	}

// 	klog.V(4).Infoln("volume active", map[string]interface{}{"vol": vol})

// 	key := common.CreateLinodeVolumeKey(vol.ID, vol.Label)
// 	resp := &csi.CreateVolumeResponse{
// 		Volume: &csi.Volume{
// 			VolumeId:      key.GetVolumeKey(),
// 			CapacityBytes: size,
// 			AccessibleTopology: []*csi.Topology{
// 				{
// 					Segments: map[string]string{
// 						VolumeTopologyRegion: vol.Region,
// 					},
// 				},
// 			},
// 			VolumeContext: volumeContext,
// 		},
// 	}

// 	// Append the content source to the response
// 	if sourceVolumeInfo != nil {
// 		resp.Volume.ContentSource = &csi.VolumeContentSource{
// 			Type: &csi.VolumeContentSource_Volume{
// 				Volume: &csi.VolumeContentSource_VolumeSource{
// 					VolumeId: contentSource.GetVolume().GetVolumeId(),
// 				},
// 			},
// 		}
// 	}

// 	klog.V(4).Infoln("volume finished creation", map[string]interface{}{"response": resp})
// 	return resp, nil
// }

// DeleteVolume deletes the given volume. The function is idempotent.
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volID, statusErr := common.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		return &csi.DeleteVolumeResponse{}, statusErr
	}

	klog.V(4).Infoln("delete volume called", map[string]interface{}{
		"volume_id": volID,
		"method":    "delete_volume",
	})

	vol, err := cs.client.GetVolume(ctx, volID)
	if linodego.IsNotFound(err) {
		return &csi.DeleteVolumeResponse{}, nil
	} else if err != nil {
		return &csi.DeleteVolumeResponse{}, errInternal("get volume %d: %v", volID, err)
	}
	if vol.LinodeID != nil {
		return &csi.DeleteVolumeResponse{}, errVolumeInUse
	}

	if err := cs.client.DeleteVolume(ctx, volID); err != nil {
		return &csi.DeleteVolumeResponse{}, errInternal("delete volume %d: %v", volID, err)
	}

	klog.V(4).Info("volume is deleted")
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches the given volume to the node
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	linodeID, statusErr := common.NodeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerPublishVolumeResponse{}, statusErr
	}

	volumeID, statusErr := common.VolumeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerPublishVolumeResponse{}, statusErr
	}

	cap := req.GetVolumeCapability()
	if cap == nil {
		return &csi.ControllerPublishVolumeResponse{}, errNoVolumeCapability
	}

	if vc := req.GetVolumeCapability(); !validVolumeCapabilities([]*csi.VolumeCapability{vc}) {
		return &csi.ControllerPublishVolumeResponse{}, errInvalidVolumeCapability(vc)
	}

	klog.V(4).Infof("controller publish volume called with %v", map[string]interface{}{
		"volume_id": volumeID,
		"node_id":   linodeID,
		"cap":       cap,
		"method":    "controller_publish_volume",
	})

	volume, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		return &csi.ControllerPublishVolumeResponse{}, errVolumeNotFound(volumeID)
	} else if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, errInternal("get volume %d: %v", volumeID, err)
	}
	if volume.LinodeID != nil {
		if *volume.LinodeID == linodeID {
			return &csi.ControllerPublishVolumeResponse{}, nil
		}
		return &csi.ControllerPublishVolumeResponse{}, errVolumeAttached(volumeID, linodeID)
	}

	instance, err := cs.client.GetInstance(ctx, linodeID)
	if linodego.IsNotFound(err) {
		return &csi.ControllerPublishVolumeResponse{}, errInstanceNotFound(linodeID)
	} else if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, errInternal("get linode instance %d: %v", linodeID, err)
	}

	// Check to see if there is room to attach this volume to the instance.
	if canAttach, err := cs.canAttach(ctx, instance); err != nil {
		return &csi.ControllerPublishVolumeResponse{}, err
	} else if !canAttach {
		// If we can, try and add a little more information to the error message
		// for the caller.
		limit, err := cs.maxVolumeAttachments(ctx, instance)
		if errors.Is(err, errNilInstance) {
			return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, "cannot calculate max volume attachments for a nil instance")
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

	klog.V(4).Infoln("waiting for volume to attach")
	volume, err = cs.client.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout())
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, errInternal("wait for volume to attach: %v", err)
	}
	klog.V(4).Infof("volume %d is attached to instance %d with path '%s'",
		volume.ID,
		*volume.LinodeID,
		volume.FilesystemPath,
	)

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
	limit, err := s.maxVolumeAttachments(ctx, instance)
	if err != nil {
		return false, err
	}

	volumes, err := s.client.ListInstanceVolumes(ctx, instance.ID, nil)
	if err != nil {
		return false, status.Errorf(codes.Internal, "list instance volumes: %v", err)
	}

	return len(volumes) < limit, nil
}

// maxVolumeAttachments returns the maximum number of volumes that can be
// attached to a single Linode instance, minus any currently-attached instance
// disks.
func (s *ControllerServer) maxVolumeAttachments(ctx context.Context, instance *linodego.Instance) (int, error) {
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
	volumeID, statusErr := common.VolumeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, statusErr
	}

	linodeID, statusErr := common.NodeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, statusErr
	}

	klog.V(4).Infoln("controller unpublish volume called", map[string]interface{}{
		"volume_id": volumeID,
		"node_id":   linodeID,
		"method":    "controller_unpublish_volume",
	})

	volume, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("get volume %d: %v", volumeID, err)
	}
	if volume.LinodeID != nil && *volume.LinodeID != linodeID {
		klog.V(4).Infof("volume is attached to %d, not to %d, skipping", *volume.LinodeID, linodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	if err := cs.client.DetachVolume(ctx, volumeID); linodego.IsNotFound(err) {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("detach volume %d: %v", volumeID, err)
	}

	klog.V(4).Infoln("waiting for detaching volume")
	if _, err := cs.client.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout()); err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("wait for volume %d to detach: %v", volumeID, err)
	}

	klog.V(4).Info("volume is detached")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerValidateVolumeCapabilities", req)
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

	klog.V(4).Infoln("validate volume capabilities called", map[string]interface{}{
		"volume_id":           req.VolumeId,
		"volume_capabilities": req.VolumeCapabilities,
		"method":              "validate_volume_capabilities",
	})

	resp := &csi.ValidateVolumeCapabilitiesResponse{}
	if validVolumeCapabilities(volumeCapabilities) {
		resp.Confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volumeCapabilities}
	}
	klog.V(4).Infoln("supported capabilities", map[string]interface{}{"response": resp})

	return resp, nil
}

// ListVolumes shall return information about all the volumes the provider knows about
func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	var err error

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

	klog.V(4).Infoln("list volumes called", map[string]interface{}{
		"list_opts":          listOpts,
		"req_starting_token": req.StartingToken,
		"method":             "list_volumes",
	})

	volumes, err := cs.client.ListVolumes(ctx, listOpts)
	if err != nil {
		return &csi.ListVolumesResponse{}, errInternal("list volumes: %v", err)
	}

	entries := make([]*csi.ListVolumesResponse_Entry, 0, len(volumes))
	for _, vol := range volumes {
		key := common.CreateLinodeVolumeKey(vol.ID, vol.Label)

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

	klog.V(4).Infoln("volumes listed", map[string]interface{}{"response": resp})
	return resp, nil
}

// ControllerGetCapabilities returns the supported capabilities of controller service provided by this Plugin
func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.driver.cscap,
	}

	klog.V(4).Infoln("controller get capabilities called", map[string]interface{}{
		"response": resp,
		"method":   "controller_get_capabilities",
	})
	return resp, nil
}

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerExpandVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	size, err := getRequestCapacitySize(req.GetCapacityRange())
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("get requested size from capacity range: %v", err)
	}

	klog.V(4).Infoln("expand volume called", map[string]interface{}{
		"volume_id": volumeID,
		"method":    "controller_expand_volume",
	})

	vol, err := cs.client.GetVolume(ctx, volumeID)
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("get volume: %v", err)
	}

	// Is the caller trying to resize the volume to be smaller than it currently is?
	if vol.Size > bytesToGB(size) {
		return &csi.ControllerExpandVolumeResponse{}, errResizeDown
	}

	if err := cs.client.ResizeVolume(ctx, volumeID, bytesToGB(size)); err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("resize volume %d: %v", volumeID, err)
	}

	vol, err = cs.client.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout())
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, errInternal("wait for volume %d to become active: %v", volumeID, err)
	}

	klog.V(4).Infoln("volume active", map[string]interface{}{"vol": vol})

	klog.V(4).Info("volume is resized")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         size,
		NodeExpansionRequired: false,
	}, nil
}

// attemptGetContentSourceVolume attempts to get information about the Linode volume to clone from.
func (cs *ControllerServer) attemptGetContentSourceVolume(ctx context.Context, contentSource *csi.VolumeContentSource) (*common.LinodeVolumeKey, error) {
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

	volumeInfo, err := common.ParseLinodeVolumeKey(sourceVolume.GetVolumeId())
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
func (cs *ControllerServer) attemptCreateLinodeVolume(ctx context.Context, label string, sizeGB int, tags string, sourceVolume *common.LinodeVolumeKey) (*linodego.Volume, error) {
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
	volumeReq := linodego.VolumeCreateOptions{
		Region: cs.metadata.Region,
		Label:  label,
		Size:   sizeGB,
	}

	if tags != "" {
		volumeReq.Tags = strings.Split(tags, ",")
	}

	klog.V(4).Infoln("creating volume", map[string]interface{}{"volume_req": volumeReq})

	result, err := cs.client.CreateVolume(ctx, volumeReq)
	if err != nil {
		return nil, errInternal("create volume: %v", err)
	}

	return result, nil
}

// cloneLinodeVolume clones a Linode volume and returns the result
func (cs *ControllerServer) cloneLinodeVolume(ctx context.Context, label string, sourceID int) (*linodego.Volume, error) {
	klog.V(4).Infoln("cloning volume", map[string]interface{}{
		"source_vol_id": sourceID,
	})

	result, err := cs.client.CloneVolume(ctx, sourceID, label)
	if err != nil {
		return nil, errInternal("clone volume %d: %v", sourceID, err)
	}

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
