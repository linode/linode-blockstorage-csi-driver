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

	"github.com/container-storage-interface/spec/lib/go/csi"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"github.com/linode/linodego"
	"golang.org/x/net/context"
	"k8s.io/mount-utils"
)

type NodeServer struct {
	driver        *LinodeDriver
	mounter       *mount.SafeFormatAndMount
	deviceutils   mountmanager.DeviceUtils
	client        linodeclient.LinodeClient
	metadata      Metadata
	encrypt       Encryption
	// TODO: Only lock mutually exclusive calls and make locking more fine grained
	mux sync.Mutex

	csi.UnimplementedNodeServer
}

var _ csi.NodeServer = &NodeServer{}

func NewNodeServer(linodeDriver *LinodeDriver, mounter *mount.SafeFormatAndMount, deviceUtils mountmanager.DeviceUtils, client linodeclient.LinodeClient, metadata Metadata, encrypt Encryption) (*NodeServer, error) {
	if linodeDriver == nil {
		return nil, fmt.Errorf("linodeDriver is nil")
	}
	if mounter == nil {
		return nil, fmt.Errorf("mounter is nil")
	}
	if deviceUtils == nil {
		return nil, fmt.Errorf("deviceUtils is nil")
	}
	if client == nil {
		return nil, fmt.Errorf("linode client is nil")
	}

	return &NodeServer{
		driver:        linodeDriver,
		mounter:       mounter,
		deviceutils:   deviceUtils,
		client:        client,
		metadata:      metadata,
		encrypt:       encrypt,
	}, nil
}

func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
    logger, ctx, done := GetLogger(ctx).WithMethod("NodePublishVolume")
    defer done()
    
    volumeID := req.GetVolumeId()
    logger.V(2).Info("Processing request", "volumeID", volumeID)
    
    ns.mux.Lock()
    defer ns.mux.Unlock()
    
    // Validate the request object
    logger.V(4).Info("Validating request", "volumeID", volumeID)
    if err := validateNodePublishVolumeRequest(ctx, req); err != nil {
        return nil, err
    }

    // Set mount options
    options := []string{"bind"}
    if req.GetReadonly() {
        options = append(options, "ro")
        logger.V(4).Info("Volume will be mounted as read-only", "volumeID", volumeID)
    }

    fs := mountmanager.NewFileSystem()
    // publish block volume
    if req.GetVolumeCapability().GetBlock() != nil {
        logger.V(4).Info("Publishing volume as block volume", "volumeID", volumeID)
        return ns.nodePublishVolumeBlock(ctx, req, options, fs)
    }

    targetPath := req.GetTargetPath()

    // Check if target path is a valid mount point
    logger.V(4).Info("Ensuring target path is a valid mount point", "volumeID", volumeID, "targetPath", targetPath)
    notMnt, err := ns.ensureMountPoint(ctx, targetPath, fs)
    if err != nil {
        return nil, err
    }
    if !notMnt {
        logger.V(4).Info("Target path is already a mount point", "volumeID", volumeID, "targetPath", targetPath)
        // TODO(#95): check if mount is compatible. Return OK if it is, or appropriate error.
        return &csi.NodePublishVolumeResponse{}, nil
    }

    stagingTargetPath := req.GetStagingTargetPath()

    // Mount stagingTargetPath to targetPath
    logger.V(4).Info("Mounting volume", "volumeID", volumeID, "stagingTargetPath", stagingTargetPath, "targetPath", targetPath, "options", options)
    err = ns.mounter.Mount(stagingTargetPath, targetPath, "ext4", options)
    if err != nil {
        return nil, errInternal("NodePublishVolume could not mount %s at %s: %v", stagingTargetPath, targetPath, err)
    }

    logger.V(2).Info("Successfully completed", "volumeID", volumeID)
    return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
    logger, ctx, done := GetLogger(ctx).WithMethod("NodeUnpublishVolume")
    defer done()

    volumeID := req.GetVolumeId()
    logger.V(2).Info("Processing request", "volumeID", volumeID)

    ns.mux.Lock()
    defer ns.mux.Unlock()

    // Validate request object
    logger.V(4).Info("Validating request", "volumeID", volumeID)
    err := validateNodeUnpublishVolumeRequest(ctx, req)
    if err != nil {
        return nil, err
    }

    // Unmount the target path and delete the remaining directory
    logger.V(4).Info("Unmounting and deleting target path", "volumeID", volumeID, "targetPath", req.GetTargetPath())
    err = mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter.Interface, true /* bind mount */)
    if err != nil {
        return nil, errInternal("NodeUnpublishVolume could not unmount %s: %v", req.GetTargetPath(), err)
    }

    // If LUKS volume is used, close the LUKS device
    logger.V(4).Info("Closing LUKS device", "volumeID", volumeID, "targetPath", req.GetTargetPath())
    if err := ns.closeLuksMountSources(ctx, req.GetTargetPath()); err != nil {
        return nil, err
    }

    logger.V(2).Info("Successfully completed", "volumeID", volumeID)
    return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
    logger, ctx, done := GetLogger(ctx).WithMethod("NodeStageVolume")
    defer done()

    volumeID := req.GetVolumeId()
    logger.V(2).Info("Processing request", "volumeID", volumeID)

    ns.mux.Lock()
    defer ns.mux.Unlock()

    // Before to start, validate the request object (NodeStageVolumeRequest)
    logger.V(4).Info("Validating request", "volumeID", volumeID)
    if err := validateNodeStageVolumeRequest(ctx, req); err != nil {
        return nil, err
    }

    // Get the LinodeVolumeKey which we need to find the device path
    LinodeVolumeKey, err := linodevolumes.ParseLinodeVolumeKey(volumeID)
    if err != nil {
        return nil, err
    }

    // Get device path of attached device
    partition := ""

    if part, ok := req.GetVolumeContext()["partition"]; ok {
        partition = part
    }

    logger.V(4).Info("Finding device path", "volumeID", volumeID)
    devicePath, err := ns.findDevicePath(ctx, *LinodeVolumeKey, partition)
    if err != nil {
        return nil, err
    }

    // Check if staging target path is a valid mount point.
    logger.V(4).Info("Ensuring staging target path is a valid mount point", "volumeID", volumeID, "stagingTargetPath", req.GetStagingTargetPath())
    notMnt, err := ns.ensureMountPoint(ctx, req.GetStagingTargetPath(), mountmanager.NewFileSystem())
    if err != nil {
        return nil, err
    }

    if !notMnt {
        // TODO(#95): Check who is mounted here. No error if its us
        /*
           1) Target Path MUST be the vol referenced by vol ID
           2) VolumeCapability MUST match
           3) Readonly MUST match

        */
        logger.V(4).Info("Staging target path is already a mount point", "volumeID", volumeID, "stagingTargetPath", req.GetStagingTargetPath())
        return &csi.NodeStageVolumeResponse{}, nil
    }

    // Check if the volume mode is set to 'Block'
    // Do nothing else with the mount point for stage
    if blk := req.VolumeCapability.GetBlock(); blk != nil {
        logger.V(4).Info("Volume is a block volume", "volumeID", volumeID)
        return &csi.NodeStageVolumeResponse{}, nil
    }

    // Mount device to stagingTargetPath
    // If LUKS is enabled, format the device accordingly
    logger.V(4).Info("Mounting device", "volumeID", volumeID, "devicePath", devicePath, "stagingTargetPath", req.GetStagingTargetPath())
    if err := ns.mountVolume(ctx, devicePath, req); err != nil {
        return nil, err
    }

    logger.V(2).Info("Successfully completed", "volumeID", volumeID)
    return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
    logger, ctx, done := GetLogger(ctx).WithMethod("NodeUnstageVolume")
    defer done()

    volumeID := req.GetVolumeId()
    logger.V(2).Info("Processing request", "volumeID", volumeID)

    ns.mux.Lock()
    defer ns.mux.Unlock()

    // Validate req (NodeUnstageVolumeRequest)
    logger.V(4).Info("Validating request", "volumeID", volumeID)
    err := validateNodeUnstageVolumeRequest(ctx, req)
    if err != nil {
        return nil, err
    }

    logger.V(4).Info("Unmounting staging target path", "volumeID", volumeID, "stagingTargetPath", req.GetStagingTargetPath())
    err = mount.CleanupMountPoint(req.GetStagingTargetPath(), ns.mounter.Interface, true /* bind mount */)
    if err != nil {
        return nil, errInternal("NodeUnstageVolume failed to unmount at path %s: %v", req.GetStagingTargetPath(), err)
    }

    // If LUKS volume is used, close the LUKS device
    logger.V(4).Info("Closing LUKS device", "volumeID", volumeID, "stagingTargetPath", req.GetStagingTargetPath())
    if err := ns.closeLuksMountSources(ctx, req.GetStagingTargetPath()); err != nil {
        return nil, err
    }

    logger.V(2).Info("Successfully completed", "volumeID", volumeID)
    return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
    logger, ctx, done := GetLogger(ctx).WithMethod("NodeExpandVolume")
    defer done()

    volumeID := req.GetVolumeId()
    logger.V(2).Info("Processing request", "volumeID", volumeID)

    // Validate req (NodeExpandVolumeRequest)
    logger.V(4).Info("Validating request", "volumeID", volumeID)
    if err := validateNodeExpandVolumeRequest(ctx, req); err != nil {
        return nil, err
    }

    // Check linode to see if a give volume exists by volume ID
    // Make call to linode api using the linode api client
    LinodeVolumeKey, err := linodevolumes.ParseLinodeVolumeKey(volumeID)
    if err != nil {
        return nil, errVolumeNotFound(LinodeVolumeKey.VolumeID)
    }
    jsonFilter, err := json.Marshal(map[string]string{"label": LinodeVolumeKey.Label})
    if err != nil {
        return nil, errInternal("marshal json filter: %v", err)
    }

    logger.V(4).Info("Listing volumes", "volumeID", volumeID)
    if _, err = ns.client.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter))); err != nil {
        return nil, errVolumeNotFound(LinodeVolumeKey.VolumeID)
    }

    logger.V(2).Info("Successfully completed", "volumeID", volumeID)
    return &csi.NodeExpandVolumeResponse{
        CapacityBytes: req.CapacityRange.RequiredBytes,
    }, nil
}

func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
    logger, _, done := GetLogger(ctx).WithMethod("NodeGetCapabilities")
    defer done()

    logger.V(2).Info("Processing request")

    return &csi.NodeGetCapabilitiesResponse{
        Capabilities: ns.driver.nscap,
    }, nil
}

func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
    logger, _, done := GetLogger(ctx).WithMethod("NodeGetInfo")
    defer done()

    logger.V(2).Info("Processing request", "req", req)

    // Get the number of currently attached instance disks, and subtract it
    // from the limit of block devices that can be attached to the instance,
    // which will effectively give us the number of block storage volumes
    // that can be attached to this node/instance.
    //
    // This is what the spec wants us to report: the actual number of volumes
    // that can be attached, and not the theoretical maximum number of
    // devices that can be attached.
    logger.V(4).Info("Listing instance disks", "nodeID", ns.metadata.ID)
    disks, err := ns.client.ListInstanceDisks(ctx, ns.metadata.ID, nil)
    if err != nil {
        return &csi.NodeGetInfoResponse{}, errInternal("list instance disks: %v", err)
    }
    maxVolumes := maxVolumeAttachments(ns.metadata.Memory) - len(disks)

    logger.V(2).Info("Successfully completed")
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
    return nodeGetVolumeStats(req)
}
