package driver

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
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

// CreateVolume provisions a new volume on behalf of a user, which can be used as a block device or mounted filesystem. 
// This operation is idempotent, meaning multiple calls with the same parameters will not create duplicate volumes. 
// For more details, refer to the CSI Driver Spec documentation.
func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("CreateVolume")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	// Validate the incoming request to ensure it meets the necessary criteria.
	// This includes checking for required fields and valid volume capabilities.
	if err := cs.validateCreateVolumeRequest(ctx, req); err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// Prepare the volume parameters such as name and SizeGB from the request.
	// This step may involve calculations or adjustments based on the request's content.
	volName, sizeGB, size, err := cs.prepareVolumeParams(ctx, req)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// Create volume context
	volContext := cs.createVolumeContext(ctx, req)

	// Attempt to retrieve information about a source volume if the request includes a content source.
	// This is important for scenarios where the volume is being cloned from an existing one.
	sourceVolInfo, err := cs.getContentSourceVolume(ctx, req.GetVolumeContentSource())
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// Create the volume
	vol, err := cs.createAndWaitForVolume(ctx, volName, sizeGB, req.Parameters[VolumeTags], sourceVolInfo)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}

	// Prepare and return response
	resp := cs.prepareCreateVolumeResponse(ctx, vol, size, volContext, sourceVolInfo, req.GetVolumeContentSource())

	log.V(2).Info("CreateVolume response", "response", resp)
	return resp, nil
}

// DeleteVolume removes the specified volume. This operation is idempotent,
// meaning that calling it multiple times with the same volume ID will have
// the same effect as calling it once. If the volume does not exist, the
// function will return a success response without any error.
// For more details, refer to the CSI Driver Spec documentation.
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

// ControllerPublishVolume attaches a volume to a specified node. 
// It ensures the volume is not already attached to another node 
// and that the node can accommodate the attachment. Returns 
// the device path if successful.
// For more details, refer to the CSI Driver Spec documentation.
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
		limit, err := cs.maxAllowedVolumeAttachments(ctx, instance)
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

// ControllerUnpublishVolume detaches a specified volume from a node.
// It checks if the volume is currently attached to the node and,
// if so, proceeds to detach it. The operation is idempotent,
// meaning it can be called multiple times without adverse effects.
// If the volume is not found or is already detached, it will
// return a successful response without error.
// For more details, refer to the CSI Driver Spec documentation.
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

// ValidateVolumeCapabilities checks if the requested volume capabilities are supported by the volume.
// It returns a confirmation response if the capabilities are valid, or an error if the volume does not exist
// or if no capabilities were provided. Refer to the @CSI Driver Spec for more details.
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

// ListVolumes returns a list of all volumes known to the provider,
// including their IDs, sizes, and accessibility information. It
// supports pagination through the starting token and maximum entries
// parameters as specified in the CSI Driver Spec.
// For more details, refer to the CSI Driver Spec documentation.
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
			return &csi.ListVolumesResponse{}, status.Errorf(codes.Aborted, 
				"invalid starting token: %q", startingToken)
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

		// If the volume is attached to a Linode instance, add it to the
		// list. Note that in the Linode API, volumes can only be
		// attached to a single Linode at a time. We are storing it in
		// a []string here, since that is what the response struct
		// returns. We do not need to pre-allocate the slice with
		// make(), since the CSI specification says this response field
		// is optional, and thus it should tolerate a nil slice.
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

// ControllerGetCapabilities retrieves the capabilities supported by the 
// controller service implemented by this Plugin. It returns a response 
// containing the capabilities available for the CSI driver.
// For more details, refer to the CSI Driver Spec documentation.
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

// ControllerExpandVolume resizes a volume to the specified capacity.
// It checks if the requested size is valid, ensures the volume exists,
// and performs the resize operation. If the volume is successfully resized,
// it returns the new capacity and indicates that no node expansion is required.
// For more details, refer to the CSI Driver Spec documentation.
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
