package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linodego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	gigabyte               = 1024 * 1024 * 1024
	devicePathKey          = "devicePath"
	waitTimeout            = 300
	cloneReadinessTimeout  = 900
	minProviderVolumeBytes = 10 * gigabyte
)

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
func NewControllerServer(driver *LinodeDriver, client linodeclient.LinodeClient, metadata Metadata) (*ControllerServer, error) {
	if driver == nil {
		return nil, errors.New("nil driver")
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
	name := req.GetName()

	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume name is required")
	}

	volCapabilities := req.GetVolumeCapabilities()
	if len(volCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume capabilities are required")
	}

	/*
		// early validation: If ANY of the specified volume capabilities are not supported
		if validVolumeCapabilities(req) {
			return nil, status.Error(codes.InvalidArgument, "CreateVolume capabilities are limitted to SINGLE_NODE_WRITER")
		}
	*/

	capRange := req.GetCapacityRange()
	size, err := getRequestCapacitySize(capRange)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// to avoid mangled requests for existing volumes with hyphen,
	// we only strip them out on creation when k8s invented the name
	// this is still problematic because we strip "-" from volume-name-prefixes
	// that specifically requested "-".
	// Don't strip this when volume labels support sufficient length
	condensedName := strings.Replace(name, "-", "", -1)

	preKey := common.CreateLinodeVolumeKey(0, condensedName)

	volumeName := preKey.GetNormalizedLabelWithPrefix(cs.driver.volumeLabelPrefix)

	klog.V(4).Infoln("create volume called", map[string]interface{}{
		"method":                  "create_volume",
		"storage_size_giga_bytes": size / gigabyte,
		"volume_name":             volumeName,
	})

	volumeContext := make(map[string]string)
	if req.Parameters[LuksEncryptedAttribute] == "true" {
		// if luks encryption is enabled add a volume context
		volumeContext[LuksEncryptedAttribute] = "true"
		volumeContext[PublishInfoVolumeName] = volumeName
		volumeContext[LuksCipherAttribute] = req.Parameters[LuksCipherAttribute]
		volumeContext[LuksKeySizeAttribute] = req.Parameters[LuksKeySizeAttribute]
	}

	targetSizeGB := int(size / gigabyte)

	// Attempt to get info about the source volume.
	// sourceVolumeInfo will be null if no content source is defined.
	contentSource := req.GetVolumeContentSource()
	sourceVolumeInfo, err := cs.attemptGetContentSourceVolume(
		ctx,
		contentSource,
	)
	if err != nil {
		return nil, err
	}

	// Attempt to create the volume while respecting idempotency
	vol, err := cs.attemptCreateLinodeVolume(
		ctx,
		volumeName,
		targetSizeGB,
		req.Parameters[VolumeTags],
		sourceVolumeInfo,
	)
	if err != nil {
		return nil, err
	}

	// Attempt to resize the volume if necessary
	if vol.Size != targetSizeGB {
		klog.V(4).Infoln("resizing volume", map[string]interface{}{
			"volume_id": vol.ID,
			"old_size":  vol.Size,
			"new_size":  targetSizeGB,
		})

		if err := cs.client.ResizeVolume(ctx, vol.ID, targetSizeGB); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to resize cloned volume (%d): %s", targetSizeGB, err)
		}
	}

	statusPollTimeout := waitTimeout

	// If we're cloning the volume we should extend the timeout
	if sourceVolumeInfo != nil {
		statusPollTimeout = cloneReadinessTimeout
	}

	if _, err := cs.client.WaitForVolumeStatus(
		ctx, vol.ID, linodego.VolumeActive, statusPollTimeout); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to wait for volume (%d) active: %s", vol.ID, err)
	}

	klog.V(4).Infoln("volume active", map[string]interface{}{"vol": vol})

	key := common.CreateLinodeVolumeKey(vol.ID, vol.Label)
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

	klog.V(4).Infoln("volume finished creation", map[string]interface{}{"response": resp})
	return resp, nil
}

// DeleteVolume deletes the given volume. The function is idempotent.
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volID, statusErr := common.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	klog.V(4).Infoln("delete volume called", map[string]interface{}{
		"volume_id": volID,
		"method":    "delete_volume",
	})

	if vol, err := cs.client.GetVolume(ctx, volID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	} else if vol.LinodeID != nil {
		return nil, status.Error(codes.FailedPrecondition, "DeleteVolume Volume in use")
	}

	if err := cs.client.DeleteVolume(ctx, volID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Info("volume is deleted")
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches the given volume to the node
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	linodeID, statusErr := common.NodeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	volumeID, statusErr := common.VolumeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	cap := req.GetVolumeCapability()
	if cap == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume capability must be provided")
	}

	if !validVolumeCapabilities([]*csi.VolumeCapability{req.GetVolumeCapability()}) {
		return nil, status.Errorf(codes.InvalidArgument, "ControllerPublishVolume Volume capability is not compatible: %v", req)
	}

	klog.V(4).Infof("controller publish volume called with %v", map[string]interface{}{
		"volume_id": volumeID,
		"node_id":   linodeID,
		"cap":       cap,
		"method":    "controller_publish_volume",
	})

	if volume, err := cs.client.GetVolume(ctx, volumeID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume with id %d not found", volumeID))
		}
		return nil, status.Error(codes.Internal, err.Error())
	} else if volume.LinodeID != nil {
		if *volume.LinodeID == linodeID {
			return &csi.ControllerPublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("Volume with id %d already attached to node %d", volumeID, *volume.LinodeID))
	}

	instance, err := cs.client.GetInstance(ctx, linodeID)
	if err, ok := err.(*linodego.Error); ok && err.Code == 404 {
		return nil, status.Errorf(codes.NotFound, "Linode with id %d not found", linodeID)
	} else if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Check to see if there is room to attach this volume to the instance.
	if canAttach, err := cs.canAttach(ctx, instance); errors.Is(err, errNilInstance) {
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, "cannot determine volume attachments for a nil instance")
	} else if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	} else if !canAttach {
		// If we can, try and add a little more information to the error message
		// for the caller.
		limit, err := cs.maxVolumeAttachments(ctx, instance)
		if errors.Is(err, errNilInstance) {
			return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, "cannot calculate max volume attachments for a nil instance")
		} else if err != nil {
			return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.ResourceExhausted, "max number of volumes already attached to instance")
		}
		return &csi.ControllerPublishVolumeResponse{}, status.Errorf(codes.ResourceExhausted, "max number of volumes (%d) already attached to instance", limit)
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
		retCode := codes.Internal
		if apiErr, ok := err.(*linodego.Error); ok && strings.Contains(apiErr.Message, "is already attached") {
			retCode = codes.Unavailable // Allow a retry if the volume is already attached: race condition can occur here
		}
		return nil, status.Errorf(retCode, "error attaching volume: %s", err)
	}

	klog.V(4).Infoln("waiting for volume to attach")
	volume, err := cs.client.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("volume %d is attached to instance %d with path '%s'", volume.ID, *volume.LinodeID, volume.FilesystemPath)

	pvInfo := map[string]string{devicePathKey: volume.FilesystemPath}
	return &csi.ControllerPublishVolumeResponse{PublishContext: pvInfo}, nil
}

// canAttach indicates whether or not another volume can be attached to the
// Linode with the given ID.
//
// Whether or not another volume can be attached is based on how many instance
// disks and block storage volumes are currently attached to the instance.
func (s *ControllerServer) canAttach(ctx context.Context, instance *linodego.Instance) (canAttach bool, err error) {
	limit, err := s.maxVolumeAttachments(ctx, instance)
	if err != nil {
		return false, fmt.Errorf("max volume attachments: %w", err)
	}

	volumes, err := s.client.ListInstanceVolumes(ctx, instance.ID, nil)
	if err != nil {
		return false, fmt.Errorf("list instance volumes: %w", err)
	}

	return len(volumes) < limit, nil
}

var (
	// errNilInstance is a general-purpose error used to indicate a nil
	// [github.com/linode/linodego.Instance] was passed as an argument to a
	// function.
	errNilInstance = errors.New("nil instance")
)

// maxVolumeAttachments returns the maximum number of volumes that can be
// attached to a single Linode instance, minus any currently-attached instance
// disks.
func (s *ControllerServer) maxVolumeAttachments(ctx context.Context, instance *linodego.Instance) (int, error) {
	if instance == nil || instance.Specs == nil {
		return 0, errNilInstance
	}

	disks, err := s.client.ListInstanceDisks(ctx, instance.ID, nil)
	if err != nil {
		return 0, fmt.Errorf("list instance disks: %w", err)
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
		return nil, statusErr
	}

	linodeID, statusErr := common.NodeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	klog.V(4).Infoln("controller unpublish volume called", map[string]interface{}{
		"volume_id": volumeID,
		"node_id":   linodeID,
		"method":    "controller_unpublish_volume",
	})

	volume, err := cs.client.GetVolume(ctx, volumeID)
	if err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if volume.LinodeID != nil && *volume.LinodeID != linodeID {
		klog.V(4).Infof("volume is attached to %d, not to %d, skipping", *volume.LinodeID, linodeID)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	if err := cs.client.DetachVolume(ctx, volumeID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "Error detaching volume: %s", err)
	}

	klog.V(4).Infoln("waiting for detaching volume")
	if _, err := cs.client.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Info("volume is detached")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerValidateVolumeCapabilities", req)
	if statusErr != nil {
		return nil, statusErr
	}

	volumeCapabilities := req.GetVolumeCapabilities()
	if len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	volume, err := cs.client.GetVolume(ctx, volumeID)
	if volume == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume with id %v not found", volumeID))
	}
	if err != nil {
		return nil, err
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
			return nil, status.Error(codes.Aborted, fmt.Sprintf("Invalid starting token %v", startingToken))
		}

		listOpts.Page = int(startingPage)
		nextToken = strconv.Itoa(listOpts.Page + 1)
	}

	klog.V(4).Infoln("list volumes called", map[string]interface{}{
		"list_opts":          listOpts,
		"req_starting_token": req.StartingToken,
		"method":             "list_volumes",
	})

	var volumes []linodego.Volume

	volumes, err = cs.client.ListVolumes(ctx, listOpts)
	if err != nil {
		return nil, err
	}
	var entries []*csi.ListVolumesResponse_Entry
	for _, vol := range volumes {
		key := common.CreateLinodeVolumeKey(vol.ID, vol.Label)

		var publishInfoVolumeName []string = make([]string, 0, 1)
		if vol.LinodeID != nil {
			publishInfoVolumeName = append(publishInfoVolumeName, fmt.Sprintf("%d", *vol.LinodeID))
		}

		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      key.GetVolumeKey(),
				CapacityBytes: int64(vol.Size * gigabyte),
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.linode.com/region": vol.Region,
						},
					},
				},
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: publishInfoVolumeName,
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

	capRange := req.GetCapacityRange()
	size, err := getRequestCapacitySize(capRange)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infoln("expand volume called", map[string]interface{}{
		"volume_id": volumeID,
		"method":    "controller_expand_volume",
	})

	var vol *linodego.Volume

	if vol, err = cs.client.GetVolume(ctx, volumeID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if vol.Size > int(size/gigabyte) {
		return nil, status.Error(codes.Internal, "Volumes can only be resized up")
	}

	if err := cs.client.ResizeVolume(ctx, volumeID, int(size/gigabyte)); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	vol, err = cs.client.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infoln("volume active", map[string]interface{}{"vol": vol})

	resp := &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         size,
		NodeExpansionRequired: false,
	}
	klog.V(4).Info("volume is resized")
	return resp, nil
}

// attemptGetContentSourceVolume attempts to get information about the Linode volume to clone from.
func (cs *ControllerServer) attemptGetContentSourceVolume(ctx context.Context, contentSource *csi.VolumeContentSource) (*common.LinodeVolumeKey, error) {
	// No content source was defined; no clone operation
	if contentSource == nil {
		return nil, nil
	}

	if _, ok := contentSource.GetType().(*csi.VolumeContentSource_Volume); !ok {
		return nil, status.Error(codes.InvalidArgument, "Unsupported volume content source type")
	}

	sourceVolume := contentSource.GetVolume()
	if sourceVolume == nil {
		return nil, status.Error(codes.InvalidArgument, "Error retrieving volume from the volume content source")
	}

	volumeInfo, err := common.ParseLinodeVolumeKey(sourceVolume.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.Internal, "Error parsing Linode volume info from volume content source")
	}

	volumeData, err := cs.client.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, "Error retrieving source volume from Linode API")
	}

	if volumeData.Region != cs.metadata.Region {
		return nil, status.Error(codes.InvalidArgument, "Source volume region cannot differ from destination volume region")
	}

	return volumeInfo, nil
}

// attemptCreateLinodeVolume attempts to create a volume while respecting
// idempotency.
func (cs *ControllerServer) attemptCreateLinodeVolume(ctx context.Context, label string, sizeGB int, tags string, sourceVolume *common.LinodeVolumeKey) (*linodego.Volume, error) {
	// List existing volumes
	jsonFilter, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		return nil, err
	}

	volumes, err := cs.client.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// This shouldn't happen, but raise an error just in case
	if len(volumes) > 1 {
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("duplicate volume %q exists", label))
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
		return nil, status.Errorf(
			codes.Internal,
			"failed to create linode volume: %s", err,
		)
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
		return nil, status.Errorf(
			codes.Internal,
			"failed to clone linode volume %d into new volume: %s", sourceID, err,
		)
	}

	return result, nil
}

// getRequestCapacity evaluates the CapacityRange parameters to validate and resolve the best volume size
func getRequestCapacitySize(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return minProviderVolumeBytes, nil
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
		if reqSize < minProviderVolumeBytes {
			// the Linode API would reject the request, opt to fulfill it by provisioning above
			// the requested size, but no more than the limit size
			reqSize = minProviderVolumeBytes
		}
		maxSize = reqSize
	} else if maxSize < minProviderVolumeBytes {
		return 0, fmt.Errorf("limit bytes %v is less than minimum bytes %v", maxSize, minProviderVolumeBytes)
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
