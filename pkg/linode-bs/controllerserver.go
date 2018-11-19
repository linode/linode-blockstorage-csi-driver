package driver

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"encoding/json"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	"github.com/linode/linodego"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const gigabyte = 1024 * 1024 * 1024
const minProviderVolumeBytes = 10 * gigabyte
const waitTimeout = 300

// CreateVolume will be called by the CO to provision a new volume on behalf of a user (to be consumed
// as either a block device or a mounted filesystem).  This operation is idempotent.
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	name := req.GetName()

	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume name is required")
	}

	volCapabilities := req.GetVolumeCapabilities()
	if volCapabilities == nil || len(volCapabilities) == 0 {
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

	volumeName := strings.Replace(req.Name, "-", "", -1)
	if len(volumeName) > 32 {
		volumeName = volumeName[:32]
	}

	ll := d.log.WithFields(logrus.Fields{
		"method":                  "create_volume",
		"storage_size_giga_bytes": size / gigabyte,
		"volume_name":             volumeName,
	})
	ll.Info("create volume called")

	jsonFilter, err := json.Marshal(map[string]string{"label": volumeName})
	if err != nil {
		return nil, err
	}

	volumes, err := d.linodeClient.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter)))
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

		ll.Info("volume already created")
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      strconv.Itoa(volume.ID),
				CapacityBytes: int64(volume.Size * gigabyte),
			},
		}, nil
	}

	volumeReq := linodego.VolumeCreateOptions{
		Region: d.region,
		Label:  volumeName,
		Size:   int(size / gigabyte),
	}

	ll.WithField("volume_req", volumeReq).Info("creating volume")

	vol, err := d.linodeClient.CreateVolume(ctx, volumeReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	vol, err = d.linodeClient.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ll.WithField("vol", vol).Info("volume active")

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      strconv.Itoa(vol.ID),
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

	ll.WithField("response", resp).Info("volume created")
	return resp, nil
}

// DeleteVolume deletes the given volume. The function is idempotent.
func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volID, statusErr := common.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	ll := d.log.WithFields(logrus.Fields{
		"volume_id": volID,
		"method":    "delete_volume",
	})
	ll.Info("delete volume called")

	if vol, err := d.linodeClient.GetVolume(ctx, volID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	} else if vol.LinodeID != nil {
		return nil, status.Error(codes.FailedPrecondition, "DeleteVolume Volume in use")
	}

	if err := d.linodeClient.DeleteVolume(ctx, volID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ll.Info("volume is deleted")
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches the given volume to the node
func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerPublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	linodeID, statusErr := common.NodeIdAsInt("ControllerPublishVolume", req)
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

	ll := d.log.WithFields(logrus.Fields{
		"volume_id": volumeID,
		"node_id":   linodeID,
		"cap":       cap,
		"method":    "controller_publish_volume",
	})
	ll.Info("controller publish volume called")

	if volume, err := d.linodeClient.GetVolume(ctx, volumeID); err != nil {
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

	if _, err := d.linodeClient.GetInstance(ctx, linodeID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("Linode with id %d not found", linodeID))
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	opts := &linodego.VolumeAttachOptions{
		LinodeID: linodeID,
		ConfigID: 0,
	}

	if _, err := d.linodeClient.AttachVolume(ctx, volumeID, opts); err != nil {
		return nil, status.Errorf(codes.Internal, "error attaching volume: %s", err)
	}

	ll.Infoln("waiting for volume to attach")
	volume, err := d.linodeClient.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout)
	if err != nil {
		return nil, err
	}
	ll.Infof("volume %d is attached to instance %d", volume.ID, *volume.LinodeID)

	return &csi.ControllerPublishVolumeResponse{}, nil
}

// ControllerUnpublishVolume deattaches the given volume from the node
func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	linodeID, statusErr := common.NodeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	ll := d.log.WithFields(logrus.Fields{
		"volume_id": volumeID,
		"node_id":   linodeID,
		"method":    "controller_unpublish_volume",
	})
	ll.Info("controller unpublish volume called")

	if err := d.linodeClient.DetachVolume(ctx, volumeID); err != nil {
		if apiErr, ok := err.(*linodego.Error); ok && apiErr.Code == 404 {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "Error detaching volume: %s", err)
	}

	ll.Infoln("waiting for detaching volume")
	if _, err := d.linodeClient.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ll.Info("volume is detached")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks whether the volume capabilities requested are supported.
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID, statusErr := common.VolumeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	volume, err := d.linodeClient.GetVolume(ctx, volumeID)
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

	ll := d.log.WithFields(logrus.Fields{
		"volume_id":              req.VolumeId,
		"volume_capabilities":    req.VolumeCapabilities,
		"supported_capabilities": vcaps,
		"method":                 "validate_volume_capabilities",
	})
	ll.Info("validate volume capabilities called")

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

	ll.WithField("response", resp).Info("supported capabilities")
	return resp, nil
}

// ListVolumes shall return information about all the volumes the provider knows about
func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
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

	ll := d.log.WithFields(logrus.Fields{
		"list_opts":          listOpts,
		"req_starting_token": req.StartingToken,
		"method":             "list_volumes",
	})
	ll.Info("list volumes called")

	var volumes []linodego.Volume

	volumes, err = d.linodeClient.ListVolumes(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	var entries []*csi.ListVolumesResponse_Entry
	for _, vol := range volumes {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      strconv.Itoa(vol.ID),
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

	ll.WithField("response", resp).Info("volumes listed")
	return resp, nil
}

// ControllerGetCapabilities returns the supported capabilities of controller service provided by this Plugin
func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
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

	d.log.WithFields(logrus.Fields{
		"response": resp,
		"method":   "controller_get_capabilities",
	}).Info("controller get capabilities called")
	return resp, nil
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

type withVolumeCapability interface {
	GetVolumeCapabilities() []*csi.VolumeCapability
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
