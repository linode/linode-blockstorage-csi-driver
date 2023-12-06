package linodebs

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
	metadataservice "github.com/linode/linode-blockstorage-csi-driver/pkg/metadata"
	"github.com/linode/linodego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type VolumeLifecycle string

const (
	gigabyte               = 1024 * 1024 * 1024
	driverName             = "linodebs.csi.linode.com"
	devicePathKey          = "devicePath"
	waitTimeout            = 300
	minProviderVolumeBytes = 10 * gigabyte

	// VolumeTags is a comma seperated string used to pass information to the linode APIs to tag the
	// created volumes
	VolumeTags = driverName + "/volumeTags"

	// PublishInfoVolumeName is used to pass the volume name from
	// `ControllerPublishVolume` to `NodeStageVolume or `NodePublishVolume`
	PublishInfoVolumeName = driverName + "/volume-name"

	VolumeLifecycleNodeStageVolume     VolumeLifecycle = "NodeStageVolume"
	VolumeLifecycleNodePublishVolume   VolumeLifecycle = "NodePublishVolume"
	VolumeLifecycleNodeUnstageVolume   VolumeLifecycle = "NodeUnstageVolume"
	VolumeLifecycleNodeUnpublishVolume VolumeLifecycle = "NodeUnpublishVolume"
)

type LinodeControllerServer struct {
	Driver          *LinodeDriver
	CloudProvider   linodeclient.LinodeClient
	MetadataService metadataservice.MetadataService
}

var _ csi.ControllerServer = &LinodeControllerServer{}

// CreateVolume will be called by the CO to provision a new volume on behalf of a user (to be consumed
// as either a block device or a mounted filesystem).  This operation is idempotent.
func (linodeCS *LinodeControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
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

	// Attempt to get info about the source volume.
	// sourceVolumeInfo will be null if no content source is defined.
	contentSource := req.GetVolumeContentSource()
	sourceVolumeInfo, err := linodeCS.attemptGetContentSourceVolume(ctx, contentSource)
	if err != nil {
		return nil, err
	}

	// to avoid mangled requests for existing volumes with hyphen,
	// we only strip them out on creation when k8s invented the name
	// this is still problematic because we strip "-" from volume-name-prefixes
	// that specifically requested "-".
	// Don't strip this when volume labels support sufficient length
	condensedName := strings.Replace(name, "-", "", -1)

	preKey := common.CreateLinodeVolumeKey(0, condensedName)

	volumeName := preKey.GetNormalizedLabelWithPrefix(linodeCS.Driver.bsPrefix)

	klog.V(4).Infoln("create volume called", map[string]interface{}{
		"method":                  "create_volume",
		"storage_size_giga_bytes": size / gigabyte,
		"volume_name":             volumeName,
	})

	jsonFilter, err := json.Marshal(map[string]string{"label": volumeName})
	if err != nil {
		return nil, err
	}

	volumes, err := linodeCS.CloudProvider.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volumeContext := make(map[string]string)
	if req.Parameters[LuksEncryptedAttribute] == "true" {
		// if luks encryption is enabled add a volume context
		volumeContext[LuksEncryptedAttribute] = "true"
		volumeContext[PublishInfoVolumeName] = volumeName
		volumeContext[LuksCipherAttribute] = req.Parameters[LuksCipherAttribute]
		volumeContext[LuksKeySizeAttribute] = req.Parameters[LuksKeySizeAttribute]
	}

	tags := req.Parameters[VolumeTags]

	if len(volumes) != 0 {
		if len(volumes) > 1 {
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("duplicate volume %q exists", volumeName))
		}
		volume := volumes[0]
		if int64(volume.Size*gigabyte) != size {
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("invalid option requested size: %d", size))
		}

		key := common.CreateLinodeVolumeKey(volume.ID, volume.Label)

		klog.V(4).Info("volume already created")
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      key.GetVolumeKey(),
				CapacityBytes: int64(volume.Size * gigabyte),
				VolumeContext: volumeContext,
			},
		}, nil
	}

	var vol *linodego.Volume
	volumeSizeGB := int(size / gigabyte)

	if sourceVolumeInfo != nil {
		// Clone the volume
		vol, err = linodeCS.cloneLinodeVolume(ctx, volumeName, volumeSizeGB, sourceVolumeInfo.VolumeID)
	} else {
		// Create the volume from scratch
		vol, err = linodeCS.createLinodeVolume(ctx, volumeName, volumeSizeGB, tags)
	}

	// Error handling for the above function calls
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
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
						"topology.linode.com/region": vol.Region,
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

	klog.V(4).Infoln("volume created", map[string]interface{}{"response": resp})
	return resp, nil
}

// DeleteVolume deletes the given volume. The function is idempotent.
func (linodeCS *LinodeControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volID, statusErr := common.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	klog.V(4).Infoln("delete volume called", map[string]interface{}{
		"volume_id": volID,
		"method":    "delete_volume",
	})

	if vol, err := linodeCS.CloudProvider.GetVolume(ctx, volID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	} else if vol.LinodeID != nil {
		return nil, status.Error(codes.FailedPrecondition, "DeleteVolume Volume in use")
	}

	if err := linodeCS.CloudProvider.DeleteVolume(ctx, volID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Info("volume is deleted")
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches the given volume to the node
func (linodeCS *LinodeControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
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

	if volume, err := linodeCS.CloudProvider.GetVolume(ctx, volumeID); err != nil {
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

	if _, err := linodeCS.CloudProvider.GetInstance(ctx, linodeID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Linode with id %d not found", linodeID))
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	opts := &linodego.VolumeAttachOptions{
		LinodeID: linodeID,
		ConfigID: 0,
	}

	if _, err := linodeCS.CloudProvider.AttachVolume(ctx, volumeID, opts); err != nil {
		retCode := codes.Internal
		if apiErr, ok := err.(*linodego.Error); ok && strings.Contains(apiErr.Message, "is already attached") {
			retCode = codes.Unavailable // Allow a retry if the volume is already attached: race condition can occur here
		}
		return nil, status.Errorf(retCode, "error attaching volume: %s", err)
	}

	klog.V(4).Infoln("waiting for volume to attach")
	volume, err := linodeCS.CloudProvider.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("volume %d is attached to instance %d with path '%s'", volume.ID, *volume.LinodeID, volume.FilesystemPath)

	pvInfo := map[string]string{devicePathKey: volume.FilesystemPath}
	return &csi.ControllerPublishVolumeResponse{PublishContext: pvInfo}, nil
}

// ControllerUnpublishVolume deattaches the given volume from the node
func (linodeCS *LinodeControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
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

	if err := linodeCS.CloudProvider.DetachVolume(ctx, volumeID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "Error detaching volume: %s", err)
	}

	klog.V(4).Infoln("waiting for detaching volume")
	if _, err := linodeCS.CloudProvider.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Info("volume is detached")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (linodeCS *LinodeControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerValidateVolumeCapabilities", req)
	if statusErr != nil {
		return nil, statusErr
	}

	volumeCapabilities := req.GetVolumeCapabilities()
	if len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	volume, err := linodeCS.CloudProvider.GetVolume(ctx, volumeID)
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
func (linodeCS *LinodeControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
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

	volumes, err = linodeCS.CloudProvider.ListVolumes(ctx, listOpts)
	if err != nil {
		return nil, err
	}
	var entries []*csi.ListVolumesResponse_Entry
	for _, vol := range volumes {
		key := common.CreateLinodeVolumeKey(vol.ID, vol.Label)

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
		})
	}

	resp := &csi.ListVolumesResponse{
		Entries:   entries,
		NextToken: nextToken,
	}

	klog.V(4).Infoln("volumes listed", map[string]interface{}{"response": resp})
	return resp, nil
}

// ControllerGetVolume allows probing for health status
func (linodeCS *LinodeControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities returns the supported capabilities of controller service provided by this Plugin
func (linodeCS *LinodeControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	var caps []*csi.ControllerServiceCapability
	for _, capability := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	} {
		caps = append(caps, newCap(capability))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}

	klog.V(4).Infoln("controller get capabilities called", map[string]interface{}{
		"response": resp,
		"method":   "controller_get_capabilities",
	})
	return resp, nil
}

func (linodeCS *LinodeControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (linodeCS *LinodeControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (linodeCS *LinodeControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (linodeCS *LinodeControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (linodeCS *LinodeControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
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

	if vol, err = linodeCS.CloudProvider.GetVolume(ctx, volumeID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if vol.Size > int(size/gigabyte) {
		return nil, status.Error(codes.Internal, "Volumes can only be resized up")
	}

	if err := linodeCS.CloudProvider.ResizeVolume(ctx, volumeID, int(size/gigabyte)); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	vol, err = linodeCS.CloudProvider.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout)

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
func (linodeCS *LinodeControllerServer) attemptGetContentSourceVolume(
	ctx context.Context, contentSource *csi.VolumeContentSource) (*common.LinodeVolumeKey, error) {
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

	volumeData, err := linodeCS.CloudProvider.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, "Error retrieving source volume from Linode API")
	}

	if volumeData.Region != linodeCS.MetadataService.GetZone() {
		return nil, status.Error(codes.InvalidArgument, "Source volume region cannot differ from destination volume region")
	}

	return volumeInfo, nil
}

// createLinodeVolume creates a Linode volume and returns the result
func (linodeCS *LinodeControllerServer) createLinodeVolume(
	ctx context.Context, label string, sizeGB int, tags string) (*linodego.Volume, error) {

	volumeReq := linodego.VolumeCreateOptions{
		Region: linodeCS.MetadataService.GetZone(),
		Label:  label,
		Size:   sizeGB,
		Tags:   strings.Split(tags, ","),
	}

	klog.V(4).Infoln("creating volume", map[string]interface{}{"volume_req": volumeReq})

	result, err := linodeCS.CloudProvider.CreateVolume(ctx, volumeReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if _, err := linodeCS.CloudProvider.WaitForVolumeStatus(
		ctx, result.ID, linodego.VolumeActive, waitTimeout); err != nil {
		return nil, status.Error(
			codes.Internal, fmt.Sprintf("failed to wait for fresh volume to be active: %s", err))
	}

	return result, nil
}

// cloneLinodeVolume clones a Linode volume and returns the result
func (linodeCS *LinodeControllerServer) cloneLinodeVolume(
	ctx context.Context, label string, sizeGB, sourceID int) (*linodego.Volume, error) {
	klog.V(4).Infoln("cloning volume", map[string]interface{}{
		"source_vol_id": sourceID,
	})

	result, err := linodeCS.CloudProvider.CloneVolume(ctx, sourceID, label)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if _, err := linodeCS.CloudProvider.WaitForVolumeStatus(
		ctx, result.ID, linodego.VolumeActive, waitTimeout); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to wait for cloned volume (%d) active: %s", result.ID, err)
	}

	if result.Size != sizeGB {
		klog.V(4).Infoln("resizing volume", map[string]interface{}{
			"volume_id": result.ID,
			"old_size":  result.Size,
			"new_size":  sizeGB,
		})

		if err := linodeCS.CloudProvider.ResizeVolume(ctx, result.ID, sizeGB); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to resize cloned volume (%d): %s", result.ID, err)
		}
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
		return 0, fmt.Errorf("Limit bytes %v is less than minimum bytes %v", maxSize, minProviderVolumeBytes)
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
