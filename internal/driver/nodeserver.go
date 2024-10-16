package driver

/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"golang.org/x/net/context"
	"k8s.io/mount-utils"

	devicemanager "github.com/linode/linode-blockstorage-csi-driver/pkg/device-manager"
	filesystem "github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/metrics"
)

type NodeServer struct {
	driver      *LinodeDriver
	mounter     *mount.SafeFormatAndMount
	deviceutils devicemanager.DeviceUtils
	client      linodeclient.LinodeClient
	metadata    Metadata
	encrypt     Encryption
	// TODO: Only lock mutually exclusive calls and make locking more fine grained
	mux sync.Mutex

	csi.UnimplementedNodeServer
}

var _ csi.NodeServer = &NodeServer{}

func NewNodeServer(ctx context.Context, linodeDriver *LinodeDriver, mounter *mount.SafeFormatAndMount, deviceUtils devicemanager.DeviceUtils, client linodeclient.LinodeClient, metadata Metadata, encrypt Encryption) (*NodeServer, error) {
	log := logger.GetLogger(ctx)

	log.V(4).Info("Creating new NodeServer")

	if linodeDriver == nil {
		log.Error(nil, "LinodeDriver is nil")
		return nil, fmt.Errorf("linodeDriver is nil")
	}
	if mounter == nil {
		log.Error(nil, "Mounter is nil")
		return nil, fmt.Errorf("mounter is nil")
	}
	if deviceUtils == nil {
		log.Error(nil, "DeviceUtils is nil")
		return nil, fmt.Errorf("deviceUtils is nil")
	}
	if client == nil {
		log.Error(nil, "Linode client is nil")
		return nil, fmt.Errorf("linode client is nil")
	}

	ns := &NodeServer{
		driver:      linodeDriver,
		mounter:     mounter,
		deviceutils: deviceUtils,
		client:      client,
		metadata:    metadata,
		encrypt:     encrypt,
	}

	log.V(4).Info("NodeServer created successfully")
	return ns, nil
}

func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodePublishVolume")
	defer done()

	functionStartTime := time.Now()
	volumeID := req.GetVolumeId()
	log.V(2).Info("Processing request", "volumeID", volumeID)

	ns.mux.Lock()
	defer ns.mux.Unlock()

	functionStatus := metrics.Completed

	// Validate the request object
	log.V(4).Info("Validating request", "volumeID", volumeID)
	if err := validateNodePublishVolumeRequest(ctx, req); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodePublishTotal, metrics.NodePublishDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Set mount options
	options := []string{"bind"}
	if req.GetReadonly() {
		options = append(options, "ro")
		log.V(4).Info("Volume will be mounted as read-only", "volumeID", volumeID)
	}

	fs := filesystem.NewFileSystem()
	// publish block volume
	if req.GetVolumeCapability().GetBlock() != nil {
		log.V(4).Info("Publishing volume as block volume", "volumeID", volumeID)
		response, err := ns.nodePublishVolumeBlock(ctx, req, options, fs)
		if err != nil {
			functionStatus = metrics.Failed
		}
		metrics.RecordMetrics(metrics.NodePublishTotal, metrics.NodePublishDuration, functionStatus, functionStartTime)
		return response, err
	}

	targetPath := req.GetTargetPath()

	// Check if target path is a valid mount point
	log.V(4).Info("Ensuring target path is a valid mount point", "volumeID", volumeID, "targetPath", targetPath)
	notMnt, err := ns.ensureMountPoint(ctx, targetPath, fs)
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodePublishTotal, metrics.NodePublishDuration, functionStatus, functionStartTime)
		return nil, err
	}
	if !notMnt {
		log.V(4).Info("Target path is already a mount point", "volumeID", volumeID, "targetPath", targetPath)
		metrics.RecordMetrics(metrics.NodePublishTotal, metrics.NodePublishDuration, functionStatus, functionStartTime)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	stagingTargetPath := req.GetStagingTargetPath()

	// Mount stagingTargetPath to targetPath
	log.V(4).Info("Mounting volume", "volumeID", volumeID, "stagingTargetPath", stagingTargetPath, "targetPath", targetPath, "options", options)
	err = ns.mounter.Mount(stagingTargetPath, targetPath, "ext4", options)

	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodePublishTotal, metrics.NodePublishDuration, functionStatus, functionStartTime)
		return nil, errInternal("NodePublishVolume could not mount %s at %s: %v", stagingTargetPath, targetPath, err)
	}

	// Record functionStatus metrics
	metrics.RecordMetrics(metrics.NodePublishTotal, metrics.NodePublishDuration, functionStatus, functionStartTime)

	log.V(4).Info("Successfully completed", "volumeID", volumeID)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeUnpublishVolume")
	defer done()

	functionStartTime := time.Now()
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()
	log.V(2).Info("Processing request", "volumeID", volumeID, "targetPath", targetPath)

	ns.mux.Lock()
	defer ns.mux.Unlock()
	functionStatus := metrics.Completed

	// Validate request object
	log.V(4).Info("Validating request", "volumeID", volumeID, "targetPath", targetPath)

	if err := validateNodeUnpublishVolumeRequest(ctx, req); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeUnpublishTotal, metrics.NodeUnpublishDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Unmount the target path and delete the remaining directory
	log.V(4).Info("Unmounting and deleting target path", "volumeID", volumeID, "targetPath", targetPath)
	if err := mount.CleanupMountPoint(targetPath, ns.mounter.Interface, true /* bind mount */); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeUnpublishTotal, metrics.NodeUnpublishDuration, functionStatus, functionStartTime)
		return nil, errInternal("NodeUnpublishVolume could not unmount %s: %v", targetPath, err)
	}

	// Record functionStatus metric
	metrics.RecordMetrics(metrics.NodeUnpublishTotal, metrics.NodeUnpublishDuration, functionStatus, functionStartTime)

	log.V(2).Info("Successfully completed", "volumeID", volumeID, "targetPath", targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeStageVolume")
	defer done()

	functionStartTime := time.Now()
	volumeID := req.GetVolumeId()
	log.V(2).Info("Processing request", "volumeID", volumeID)

	ns.mux.Lock()
	defer ns.mux.Unlock()

	functionStatus := metrics.Completed

	// Before to functionStartTime, validate the request object (NodeStageVolumeRequest)
	log.V(4).Info("Validating request", "volumeID", volumeID)
	if err := validateNodeStageVolumeRequest(ctx, req); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Get the LinodeVolumeKey which we need to find the device path
	LinodeVolumeKey, err := linodevolumes.ParseLinodeVolumeKey(volumeID)
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Get device path of attached device
	partition := ""

	if part, ok := req.GetVolumeContext()["partition"]; ok {
		partition = part
	}

	log.V(4).Info("Finding device path", "volumeID", volumeID)
	devicePath, err := ns.findDevicePath(ctx, *LinodeVolumeKey, partition)
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Check if staging target path is a valid mount point.
	log.V(4).Info("Ensuring staging target path is a valid mount point", "volumeID", volumeID, "stagingTargetPath", req.GetStagingTargetPath())
	notMnt, err := ns.ensureMountPoint(ctx, req.GetStagingTargetPath(), filesystem.NewFileSystem())
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		return nil, err
	}

	if !notMnt {
		// TODO(#95): Check who is mounted here. No error if its us
		/*
		   1) Target Path MUST be the vol referenced by vol ID
		   2) VolumeCapability MUST match
		   3) Readonly MUST match

		*/
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		log.V(4).Info("Staging target path is already a mount point", "volumeID", volumeID, "stagingTargetPath", req.GetStagingTargetPath())
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Check if the volume mode is set to 'Block'
	// Do nothing else with the mount point for stage
	if blk := req.GetVolumeCapability().GetBlock(); blk != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		log.V(4).Info("Volume is a block volume", "volumeID", volumeID)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Mount device to stagingTargetPath
	// If LUKS is enabled, format the device accordingly
	log.V(4).Info("Mounting device", "volumeID", volumeID, "devicePath", devicePath, "stagingTargetPath", req.GetStagingTargetPath())
	if err := ns.mountVolume(ctx, devicePath, req); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Record functionStatus metric
	metrics.RecordMetrics(metrics.NodeStageVolumeTotal, metrics.NodeStageVolumeDuration, functionStatus, functionStartTime)

	log.V(2).Info("Successfully completed", "volumeID", volumeID)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeUnstageVolume")
	defer done()

	functionStartTime := time.Now()
	stagingTargetPath := req.GetStagingTargetPath()
	volumeID := req.GetVolumeId()
	log.V(2).Info("Processing request", "volumeID", volumeID, "stagingTargetPath", stagingTargetPath)

	ns.mux.Lock()
	defer ns.mux.Unlock()

	functionStatus := metrics.Completed

	// Validate req (NodeUnstageVolumeRequest)
	log.V(4).Info("Validating request", "volumeID", volumeID, "stagingTargetPath", stagingTargetPath)
	err := validateNodeUnstageVolumeRequest(ctx, req)
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeUnstageVolumeTotal, metrics.NodeUnstageVolumeDuration, functionStatus, functionStartTime)
		return nil, err
	}

	log.V(4).Info("Unmounting staging target path", "volumeID", volumeID, "stagingTargetPath", stagingTargetPath)
	err = mount.CleanupMountPoint(stagingTargetPath, ns.mounter.Interface, true /* bind mount */)
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeUnstageVolumeTotal, metrics.NodeUnstageVolumeDuration, functionStatus, functionStartTime)
		return nil, errInternal("NodeUnstageVolume failed to unmount at path %s: %v", stagingTargetPath, err)
	}

	// If LUKS volume is used, close the LUKS device
	log.V(4).Info("Closing LUKS device", "volumeID", volumeID, "stagingTargetPath", stagingTargetPath)
	if err := ns.closeLuksMountSource(ctx, volumeID); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeUnstageVolumeTotal, metrics.NodeUnstageVolumeDuration, functionStatus, functionStartTime)
		return nil, fmt.Errorf("closing luks to unstage volume %s: %w", volumeID, err)
	}

	// Record functionStatus metric
	metrics.RecordMetrics(metrics.NodeUnstageVolumeTotal, metrics.NodeUnstageVolumeDuration, functionStatus, functionStartTime)

	log.V(2).Info("Successfully completed", "volumeID", volumeID)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeExpandVolume")
	defer done()

	functionStartTime := time.Now()
	volumeID := req.GetVolumeId()
	log.V(2).Info("Processing request", "volumeID", volumeID)

	functionStatus := metrics.Completed

	// Validate req (NodeExpandVolumeRequest)
	log.V(4).Info("Validating request", "volumeID", volumeID)
	if err := validateNodeExpandVolumeRequest(ctx, req); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeExpandTotal, metrics.NodeExpandDuration, functionStatus, functionStartTime)
		return nil, err
	}

	// Check linode to see if a give volume exists by volume ID
	// Make call to linode api using the linode api client
	LinodeVolumeKey, err := linodevolumes.ParseLinodeVolumeKey(volumeID)
	log.V(4).Info("Processed LinodeVolumeKey", "LinodeVolumeKey", LinodeVolumeKey)
	if err != nil {
		// Node volume expansion is not supported yet. To meet the spec, we need to implement this.
		// For now, we'll return a not found error.
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeExpandTotal, metrics.NodeExpandDuration, functionStatus, functionStartTime)
		return nil, errNotFound("volume not found: %v", err)
	}
	jsonFilter, err := json.Marshal(map[string]string{"label": LinodeVolumeKey.Label})
	if err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeExpandTotal, metrics.NodeExpandDuration, functionStatus, functionStartTime)
		return nil, errInternal("marshal json filter: %v", err)
	}

	log.V(4).Info("Listing volumes", "volumeID", volumeID)
	if _, err = ns.client.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter))); err != nil {
		functionStatus = metrics.Failed
		metrics.RecordMetrics(metrics.NodeExpandTotal, metrics.NodeExpandDuration, functionStatus, functionStartTime)
		return nil, errVolumeNotFound(LinodeVolumeKey.VolumeID)
	}

	// Record functionStatus metric
	metrics.RecordMetrics(metrics.NodeExpandTotal, metrics.NodeExpandDuration, functionStatus, functionStartTime)

	log.V(2).Info("Successfully completed", "volumeID", volumeID)
	return &csi.NodeExpandVolumeResponse{
		CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
	}, nil
}

func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeGetCapabilities")
	defer done()

	log.V(2).Info("Processing request")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.driver.nscap,
	}, nil
}

func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeGetInfo")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	// Get the number of currently attached instance disks, and subtract it
	// from the limit of block devices that can be attached to the instance,
	// which will effectively give us the number of block storage volumes
	// that can be attached to this node/instance.
	//
	// This is what the spec wants us to report: the actual number of volumes
	// that can be attached, and not the theoretical maximum number of
	// devices that can be attached.
	log.V(4).Info("Listing instance disks", "nodeID", ns.metadata.ID)
	disks, err := ns.client.ListInstanceDisks(ctx, ns.metadata.ID, nil)
	if err != nil {
		return &csi.NodeGetInfoResponse{}, errInternal("list instance disks: %v", err)
	}
	maxVolumes := maxVolumeAttachments(ns.metadata.Memory) - len(disks)

	log.V(2).Info("functionStatusfully completed")
	return &csi.NodeGetInfoResponse{
		NodeId:            strconv.Itoa(ns.metadata.ID),
		MaxVolumesPerNode: int64(maxVolumes),
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				"topology.linode.com/region": ns.metadata.Region,
			},
		},
	}, nil
}

func (ns *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("NodeGetVolumeStats")
	defer done()

	log.V(2).Info("Processing request", "req", req)

	return nodeGetVolumeStats(ctx, req)
}
