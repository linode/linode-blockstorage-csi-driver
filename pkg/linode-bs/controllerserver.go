package linodebs

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"encoding/json"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	metadataservice "github.com/linode/linode-blockstorage-csi-driver/pkg/metadata"
	"github.com/linode/linodego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const gigabyte = 1024 * 1024 * 1024
const minProviderVolumeBytes = 10 * gigabyte
const waitTimeout = 300

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

	preKey := common.CreateLinodeVolumeKey(0, name)
	volumeName := preKey.GetNormalizedLabelWithPrefix(linodeCS.Driver.bsPrefix)

	glog.V(4).Infoln("create volume called", map[string]interface{}{
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

	if len(volumes) != 0 {
		if len(volumes) > 1 {
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("duplicate volume %q exists", volumeName))
		}
		volume := volumes[0]
		if int64(volume.Size*gigabyte) != size {
			return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("invalid option requested size: %d", size))
		}

		key := common.CreateLinodeVolumeKey(volume.ID, volume.Label)

		glog.V(4).Info("volume already created")
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      key.GetVolumeKey(),
				CapacityBytes: int64(volume.Size * gigabyte),
			},
		}, nil
	}

	volumeReq := linodego.VolumeCreateOptions{
		Region: linodeCS.MetadataService.GetZone(),
		Label:  volumeName,
		Size:   int(size / gigabyte),
	}

	glog.V(4).Infoln("creating volume", map[string]interface{}{"volume_req": volumeReq})

	vol, err := linodeCS.CloudProvider.CreateVolume(ctx, volumeReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	vol, err = linodeCS.CloudProvider.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infoln("volume active", map[string]interface{}{"vol": vol})

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
		},
	}

	glog.V(4).Infoln("volume created", map[string]interface{}{"response": resp})
	return resp, nil
}

// DeleteVolume deletes the given volume. The function is idempotent.
func (linodeCS *LinodeControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volID, statusErr := common.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	glog.V(4).Infoln("delete volume called", map[string]interface{}{
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

	glog.V(4).Info("volume is deleted")
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

	glog.V(4).Infof("controller publish volume called with %v", map[string]interface{}{
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
		/**
		TODO(displague) existing volume on node is not ok unless checking publish caps are identical
		if *volume.LinodeID == linodeID {
			return &csi.ControllerPublishVolumeResponse{}, nil
		}
		**/
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
		return nil, status.Errorf(codes.Internal, "error attaching volume: %s", err)
	}

	glog.V(4).Infoln("waiting for volume to attach")
	volume, err := linodeCS.CloudProvider.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout)
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("volume %d is attached to instance %d", volume.ID, *volume.LinodeID)

	return &csi.ControllerPublishVolumeResponse{}, nil
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

	glog.V(4).Infoln("controller unpublish volume called", map[string]interface{}{
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

	glog.V(4).Infoln("waiting for detaching volume")
	if _, err := linodeCS.CloudProvider.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Info("volume is detached")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (linodeCS *LinodeControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	volume, err := linodeCS.CloudProvider.GetVolume(ctx, volumeID)
	if volume == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume with id %v not found", volumeID))
	}
	if err != nil {
		return nil, err
	}

	var vcaps []*csi.VolumeCapability_AccessMode
	for _, mode := range []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	} {
		vcaps = append(vcaps, &csi.VolumeCapability_AccessMode{Mode: mode})
	}

	glog.V(4).Infoln("validate volume capabilities called", map[string]interface{}{
		"volume_id":              req.VolumeId,
		"volume_capabilities":    req.VolumeCapabilities,
		"supported_capabilities": vcaps,
		"method":                 "validate_volume_capabilities",
	})

	/*
		hasSupport := func(mode csi.VolumeCapability_AccessMode_Mode) bool {
			for _, m := range vcaps {
				if mode == m.Mode {
					return true
				}
			}
			return false
		}
	*/

	resp := &csi.ValidateVolumeCapabilitiesResponse{
		Message: "ValidateVolumeCapabilities is currently unimplemented for CSI v1.0.0",
	}
	/*
		for _, capabilities := range req.VolumeCapabilities {
			if hasSupport(capabilities.AccessMode.Mode) {
				resp.Supported = true
			} else {
				// we need to make sure all capabilities are supported. Revert back
				// in case we have a cap that is supported, but is invalidated now
				resp.Supported = false
			}
		}
	*/

	glog.V(4).Infoln("supported capabilities", map[string]interface{}{"response": resp})
	return resp, nil
}

// ListVolumes shall return information about all the volumes the provider knows about
func (linodeCS *LinodeControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	var err error

	startingToken := req.GetStartingToken()
	nextToken := ""

	listOpts := linodego.NewListOptions(0, "")
	if req.GetMaxEntries() > 0 {
		if startingToken == "" {
			listOpts.Page = 1
		} else {
			startingPage, errParse := strconv.ParseInt(startingToken, 10, 64)
			if errParse != nil {
				return nil, status.Error(codes.Aborted, fmt.Sprintf("Invalid starting token %v", startingToken))
			}
			listOpts.Page = int(startingPage)
		}
		nextToken = strconv.Itoa(listOpts.Page + 1)
	}

	glog.V(4).Infoln("list volumes called", map[string]interface{}{
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

	glog.V(4).Infoln("volumes listed", map[string]interface{}{"response": resp})
	return resp, nil
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
	} {
		caps = append(caps, newCap(capability))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}

	glog.V(4).Infoln("controller get capabilities called", map[string]interface{}{
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

// getRequestCapacity evaluates the CapacityRange parameters to validate and resolve the best volume size
func getRequestCapacitySize(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return minProviderVolumeBytes, nil
	}

	minSize := capRange.GetRequiredBytes()
	maxSize := capRange.GetLimitBytes()

	// At least one of the these fields MUST be specified.
	if minSize == 0 && maxSize == 0 {
		return 0, errors.New("RequiredBytes or LimitBytes must be set")
	}

	// The value of this field MUST NOT be negative.
	if minSize < 0 || maxSize < 0 {
		return 0, errors.New("RequiredBytes and LimitBytes may not be negative")
	}

	if maxSize == 0 {
		// Only received a required size
		if minSize < minProviderVolumeBytes {
			return 0, fmt.Errorf("Required bytes %v is less than minimum bytes %v", minSize, minProviderVolumeBytes)
		}
		maxSize = minSize
	}

	if maxSize < minProviderVolumeBytes {
		return 0, fmt.Errorf("Limit bytes %v is less than minimum bytes %v", maxSize, minProviderVolumeBytes)
	}

	if minSize == 0 {
		// Only received a limit size
		minSize = maxSize
	}

	return minSize, nil
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
