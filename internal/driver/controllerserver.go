package driver

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/observability"
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
	log, _ := logger.GetLogger(ctx)

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
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "CreateVolume")
	defer done()

	functionStartTime := time.Now()
	log.V(2).Info("Processing request", "req", req)

	// Validate the incoming request to ensure it meets the necessary criteria.
	// This includes checking for required fields and valid volume capabilities.
	if err := cs.validateCreateVolumeRequest(ctx, req); err != nil {
		observability.RecordMetrics(observability.ControllerCreateVolumeTotal, observability.ControllerCreateVolumeDuration, observability.Failed, functionStartTime)
		return &csi.CreateVolumeResponse{}, err
	}

	// Prepare the volume parameters such as name and SizeGB from the request.
	// This step may involve calculations or adjustments based on the request's content.
	params, err := cs.prepareVolumeParams(ctx, req)
	if err != nil {
		observability.RecordMetrics(observability.ControllerCreateVolumeTotal, observability.ControllerCreateVolumeDuration, observability.Failed, functionStartTime)
		return &csi.CreateVolumeResponse{}, err
	}

	contentSource := req.GetVolumeContentSource()
	accessibilityRequirements := req.GetAccessibilityRequirements()

	// Attempt to retrieve information about a source volume if the request includes a content source.
	// This is important for scenarios where the volume is being cloned from an existing one.
	sourceVolInfo, err := cs.getContentSourceVolume(ctx, contentSource, accessibilityRequirements)
	if err != nil {
		observability.RecordMetrics(observability.ControllerCreateVolumeTotal, observability.ControllerCreateVolumeDuration, observability.Failed, functionStartTime)
		return &csi.CreateVolumeResponse{}, err
	}

	// Create the volume
	vol, err := cs.createAndWaitForVolume(ctx, params.VolumeName, req.GetParameters(), params.EncryptionStatus, params.TargetSizeGB, sourceVolInfo, params.Region)
	if err != nil {
		observability.RecordMetrics(observability.ControllerCreateVolumeTotal, observability.ControllerCreateVolumeDuration, observability.Failed, functionStartTime)
		return &csi.CreateVolumeResponse{}, err
	}

	// Create volume context
	volContext := cs.createVolumeContext(ctx, req, vol)

	// Prepare and return response
	resp := cs.prepareCreateVolumeResponse(ctx, vol, params.Size, volContext, sourceVolInfo, contentSource)

	// Record function completion
	observability.RecordMetrics(observability.ControllerCreateVolumeTotal, observability.ControllerCreateVolumeDuration, observability.Completed, functionStartTime)

	log.V(2).Info("CreateVolume response", "response", resp)
	return resp, nil
}

// DeleteVolume removes the specified volume. This operation is idempotent,
// meaning that calling it multiple times with the same volume ID will have
// the same effect as calling it once. If the volume does not exist, the
// function will return a success response without any error.
// For more details, refer to the CSI Driver Spec documentation.
func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "DeleteVolume")
	defer done()

	functionStartTime := time.Now()
	volID, statusErr := linodevolumes.VolumeIdAsInt("DeleteVolume", req)
	if statusErr != nil {
		observability.RecordMetrics(observability.ControllerDeleteVolumeTotal, observability.ControllerDeleteVolumeDuration, observability.Failed, functionStartTime)
		return &csi.DeleteVolumeResponse{}, statusErr
	}

	log.V(2).Info("Processing request", "req", req)

	// Check if the volume exists
	log.V(4).Info("Checking if volume exists", "volume_id", volID)
	vol, err := cs.client.GetVolume(ctx, volID)
	if linodego.IsNotFound(err) {
		observability.RecordMetrics(observability.ControllerDeleteVolumeTotal, observability.ControllerDeleteVolumeDuration, observability.Failed, functionStartTime)
		return &csi.DeleteVolumeResponse{}, nil
	} else if err != nil {
		observability.RecordMetrics(observability.ControllerDeleteVolumeTotal, observability.ControllerDeleteVolumeDuration, observability.Failed, functionStartTime)
		return &csi.DeleteVolumeResponse{}, errInternal("get volume %d: %v", volID, err)
	}
	if vol.LinodeID != nil {
		observability.RecordMetrics(observability.ControllerDeleteVolumeTotal, observability.ControllerDeleteVolumeDuration, observability.Failed, functionStartTime)
		return &csi.DeleteVolumeResponse{}, errVolumeInUse
	}

	// Delete the volume
	log.V(4).Info("Deleting volume", "volume_id", volID)
	if err := cs.client.DeleteVolume(ctx, volID); err != nil {
		observability.RecordMetrics(observability.ControllerDeleteVolumeTotal, observability.ControllerDeleteVolumeDuration, observability.Failed, functionStartTime)
		return &csi.DeleteVolumeResponse{}, errInternal("delete volume %d: %v", volID, err)
	}

	// Record function completion
	observability.RecordMetrics(observability.ControllerDeleteVolumeTotal, observability.ControllerDeleteVolumeDuration, observability.Completed, functionStartTime)

	log.V(2).Info("Volume deleted successfully", "volume_id", volID)
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches a volume to a specified node.
// It ensures the volume is not already attached to another node
// and that the node can accommodate the attachment. Returns
// the device path if successful.
// For more details, refer to the CSI Driver Spec documentation.
func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ControllerPublishVolume")
	defer done()

	functionStartTime := time.Now()
	log.V(2).Info("Processing request", "req", req)

	// Validate the request and get Linode ID and Volume ID
	linodeID, volumeID, err := cs.validateControllerPublishVolumeRequest(ctx, req)
	if err != nil {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return resp, err
	}

	// Retrieve and validate the instance associated with the Linode ID
	instance, err := cs.getInstance(ctx, linodeID)
	if err != nil {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return resp, err
	}

	// Check if the volume exists and is valid.
	// If the volume is already attached to the specified instance, it returns its device path.
	devicePath, err := cs.getAndValidateVolume(ctx, volumeID, instance)
	if err != nil {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return resp, err
	}
	// If devicePath is not empty, the volume is already attached
	if devicePath != "" {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return &csi.ControllerPublishVolumeResponse{
			PublishContext: map[string]string{
				devicePathKey: devicePath,
			},
		}, nil
	}

	// Check if the instance can accommodate the volume attachment
	if capErr := cs.checkAttachmentCapacity(ctx, instance); capErr != nil {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return resp, capErr
	}

	// Attach the volume to the specified instance
	if attachErr := cs.attachVolume(ctx, volumeID, linodeID); attachErr != nil {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return resp, attachErr
	}

	log.V(4).Info("Waiting for volume to attach", "volume_id", volumeID)
	// Wait for the volume to be successfully attached to the instance
	volume, err := cs.client.WaitForVolumeLinodeID(ctx, volumeID, &linodeID, waitTimeout())
	if err != nil {
		observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Failed, functionStartTime)
		return resp, err
	}

	// Record function completion
	observability.RecordMetrics(observability.ControllerPublishVolumeTotal, observability.ControllerPublishVolumeDuration, observability.Completed, functionStartTime)

	log.V(2).Info("Volume attached successfully", "volume_id", volume.ID, "node_id", *volume.LinodeID, "device_path", volume.FilesystemPath)

	// Return the response with the device path of the attached volume
	resp = &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			devicePathKey: volume.FilesystemPath,
		},
	}
	return resp, nil
}

// ControllerUnpublishVolume detaches a specified volume from a node.
// It checks if the volume is currently attached to the node and,
// if so, proceeds to detach it. The operation is idempotent,
// meaning it can be called multiple times without adverse effects.
// If the volume is not found or is already detached, it will
// return a successful response without error.
// For more details, refer to the CSI Driver Spec documentation.
func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ControllerUnpublishVolume")
	defer done()

	functionStartTime := time.Now()
	log.V(2).Info("Processing request", "req", req)

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerUnpublishVolume", req)
	if statusErr != nil {
		observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
		return &csi.ControllerUnpublishVolumeResponse{}, statusErr
	}

	log = log.WithValues("volume_id", volumeID)

	log.V(4).Info("Checking if volume is attached")
	volume, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
		log.V(4).Info("Volume not found, skipping")
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else if err != nil {
		observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("get volume %d: %v", volumeID, err)
	}

	if req.GetNodeId() != "" {
		linodeID, statusErr := linodevolumes.NodeIdAsInt("ControllerUnpublishVolume", req)
		if statusErr != nil {
			observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
			return &csi.ControllerUnpublishVolumeResponse{}, statusErr
		}

		log = log.V(4).WithValues("node_id", linodeID)

		if volume.LinodeID != nil && *volume.LinodeID != linodeID {
			observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
			log.V(4).Info("Volume attached to different instance, skipping", "volume_id", volumeID, "attached_node_id", *volume.LinodeID, "requested_node_id", linodeID)
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
	}

	log.V(4).Info("Executing detach volume")
	if err := cs.client.DetachVolume(ctx, volumeID); linodego.IsNotFound(err) {
		observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else if err != nil {
		observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("detach volume %d: %v", volumeID, err)
	}

	log.V(4).Info("Waiting for volume to detach")
	if _, err := cs.client.WaitForVolumeLinodeID(ctx, volumeID, nil, waitTimeout()); err != nil {
		observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Failed, functionStartTime)
		return &csi.ControllerUnpublishVolumeResponse{}, errInternal("wait for volume %d to detach: %v", volumeID, err)
	}

	// Record function completion
	observability.RecordMetrics(observability.ControllerUnpublishVolumeTotal, observability.ControllerUnpublishVolumeDuration, observability.Completed, functionStartTime)

	log.V(2).Info("Volume detached successfully")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks if the requested volume capabilities are supported by the volume.
// It returns a confirmation response if the capabilities are valid, or an error if the volume does not exist
// or if no capabilities were provided. Refer to the @CSI Driver Spec for more details.
func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ValidateVolumeCapabilities")
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

	resp = &csi.ValidateVolumeCapabilitiesResponse{}
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
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ListVolumes")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	startingToken := req.GetStartingToken()
	nextToken := ""

	listOpts := linodego.NewListOptions(0, "")
	if req.GetMaxEntries() > 0 {
		listOpts.PageSize = int(req.GetMaxEntries())
	}

	if startingToken != "" {
		startingPage, errParse := strconv.ParseInt(startingToken, 10, 0)
		if errParse != nil {
			return &csi.ListVolumesResponse{}, status.Errorf(codes.Aborted,
				"invalid starting token: %q", startingToken)
		}

		if startingPage < math.MinInt || startingPage > math.MaxInt {
			return &csi.ListVolumesResponse{}, status.Errorf(codes.Aborted,
				"starting token out of bounds: %q", startingToken)
		}
		listOpts.Page = int(startingPage)
	}

	// List all volumes
	log.V(4).Info("Listing volumes", "list_opts", listOpts)
	volumes, err := cs.client.ListVolumes(ctx, listOpts)
	if err != nil {
		return &csi.ListVolumesResponse{}, errInternal("list volumes: %v", err)
	}

	entries := make([]*csi.ListVolumesResponse_Entry, 0, len(volumes))
	for volNum := range volumes {
		csiVol, publishedNodeIds, volumeCondition := getVolumeResponse(&volumes[volNum])
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: csiVol,
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: publishedNodeIds,
				VolumeCondition:  volumeCondition,
			},
		})
	}

	// Only set nextToken if we got a full page and there might be more
	if req.GetMaxEntries() > 0 && len(volumes) >= listOpts.PageSize {
		nextToken = strconv.Itoa(listOpts.Page + 1)
	}

	resp := &csi.ListVolumesResponse{
		Entries:   entries,
		NextToken: nextToken,
	}

	log.V(2).Info("Volumes listed")
	log.V(6).Info("Volumes listed", "response", resp)
	return resp, nil
}

// ControllerGetCapabilities retrieves the capabilities supported by the
// controller service implemented by this Plugin. It returns a response
// containing the capabilities available for the CSI driver.
// For more details, refer to the CSI Driver Spec documentation.
func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	log, _ := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ControllerGetCapabilities")
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
func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ControllerExpandVolume")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerExpandVolume", req)
	if statusErr != nil {
		return nil, statusErr
	}

	size, err := getRequestCapacitySize(req.GetCapacityRange())
	if err != nil {
		return resp, errInternal("get requested size from capacity range: %v", err)
	}

	// Get the volume
	log.V(4).Info("Checking if volume exists", "volume_id", volumeID)
	vol, err := cs.client.GetVolume(ctx, volumeID)
	if err != nil {
		return resp, errNotFound("get volume: %v", err)
	}

	// Is the caller trying to resize the volume to be smaller than it currently is?
	if vol.Size > bytesToGB(size) {
		return resp, errResizeDown
	}

	// Resize the volume
	log.V(4).Info("Calling API to resize volume", "volume_id", volumeID)
	if err = cs.client.ResizeVolume(ctx, volumeID, bytesToGB(size)); err != nil {
		return resp, errInternal("resize volume %d: %v", volumeID, err)
	}

	// Wait for the volume to become active
	log.V(4).Info("Waiting for volume to become active", "volume_id", volumeID)
	vol, err = cs.client.WaitForVolumeStatus(ctx, vol.ID, linodego.VolumeActive, waitTimeout())
	if err != nil {
		return resp, errInternal("timed out waiting for volume %d to become active: %v", volumeID, err)
	}
	log.V(4).Info("Volume active", "vol", vol)

	log.V(2).Info("Volume resized successfully", "volume_id", volumeID)
	resp = &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         size,
		NodeExpansionRequired: true,
	}
	return resp, nil
}

// ControllerGetVolume returns information about the specified volume.
// It checks if the volume exists and returns its ID, size, and status.
// For more details, refer to the CSI Driver Spec documentation.
func (cs *ControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	log, ctx := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "ControllerGetVolume")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	volumeID, statusErr := linodevolumes.VolumeIdAsInt("ControllerGetVolume", req)
	if statusErr != nil {
		return &csi.ControllerGetVolumeResponse{}, statusErr
	}

	// Get the volume
	log.V(4).Info("Checking if volume exists", "volume_id", volumeID)
	vol, err := cs.client.GetVolume(ctx, volumeID)
	if linodego.IsNotFound(err) {
		return &csi.ControllerGetVolumeResponse{}, errVolumeNotFound(volumeID)
	} else if err != nil {
		return &csi.ControllerGetVolumeResponse{}, errInternal("get volume: %v", err)
	}

	csiVol, publishedNodeIds, volumeCondition := getVolumeResponse(vol)
	resp := &csi.ControllerGetVolumeResponse{
		Volume: csiVol,
		Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
			PublishedNodeIds: publishedNodeIds,
			VolumeCondition:  volumeCondition,
		},
	}

	log.V(2).Info("Volume retrieved successfully", "volume_id", volumeID, "response", resp)
	return resp, nil
}

func getVolumeResponse(volume *linodego.Volume) (csiVolume *csi.Volume, publishedNodeIds []string, volumeCondition *csi.VolumeCondition) {
	key := linodevolumes.CreateLinodeVolumeKey(volume.ID, volume.Label)

	// If the volume is attached to a Linode instance, add it to the
	// list. Note that in the Linode API, volumes can only be
	// attached to a single Linode at a time. We are storing it in
	// a []string here, since that is what the response struct
	// returns. We do not need to pre-allocate the slice with
	// make(), since the CSI specification says this response field
	// is optional, and thus it should tolerate a nil slice.
	publishedNodeIds = []string{}
	if volume.LinodeID != nil {
		publishedNodeIds = append(publishedNodeIds, strconv.Itoa(*volume.LinodeID))
	}

	csiVolume = &csi.Volume{
		VolumeId:      key.GetVolumeKey(),
		CapacityBytes: gbToBytes(volume.Size),
	}

	if volume.Region != "" {
		csiVolume.AccessibleTopology = []*csi.Topology{
			{
				Segments: map[string]string{
					VolumeTopologyRegion: volume.Region,
				},
			},
		}
	}

	abnormal := false
	if volume.Status != linodego.VolumeActive {
		abnormal = true
	}

	volumeCondition = &csi.VolumeCondition{
		Abnormal: abnormal,
		Message:  string(volume.Status),
	}

	return
}
