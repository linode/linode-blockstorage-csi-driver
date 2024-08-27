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
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"github.com/linode/linodego"
	"golang.org/x/net/context"
	"k8s.io/klog/v2"
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
	ns.mux.Lock()
	defer ns.mux.Unlock()
	klog.V(4).Infof("NodePublishVolume called with req: %#v", req)

	// Validate the request object
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	// Set mount options:
	//  - bind mount to the full path to allow duplicate mounts of the same PD.
	//  - read-only if specified
	options := []string{"bind"}
	if req.GetReadonly() {
		options = append(options, "ro")
	}
	
	fs := mountmanager.NewFileSystem()
	// publish block volume
	if req.GetVolumeCapability().GetBlock() != nil {
		return ns.nodePublishVolumeBlock(req, options, fs)
	}
	
	// Path to where we want to mount the volume inside the pod
	targetPath := req.GetTargetPath()
	// Check if target path is a valid mount point. 
	// If not, create it.
	notMnt, err := ns.ensureMountPoint(targetPath, fs)
	if err != nil {
		return nil, err
	}
	// No errors but target path is not a valid mount point
	if !notMnt {
		// TODO(#95): check if mount is compatible. Return OK if it is, or appropriate error.
		/*
		1) Target Path MUST be the vol referenced by vol ID
		2) VolumeCapability MUST match
		3) Readonly MUST match
		
		*/
		return &csi.NodePublishVolumeResponse{}, nil
	}
	
	// Path to the volume on the host where the volume is currently staged (mounted)
	stagingTargetPath := req.GetStagingTargetPath()
	// Mount stagingTargetPath to targetPath
	err = ns.mounter.Mount(stagingTargetPath, targetPath, "ext4", options)
	if err != nil {
		klog.Errorf("Mount of disk %s failed: %v", targetPath, err)
		return nil, errInternal("NodePublishVolume could not mount %s at %s: %v", stagingTargetPath, targetPath, err)
	}

	klog.V(4).Infof("NodePublishVolume successfully mounted %s", targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()

	// Validate request object
	err := validateNodeUnpublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	// Unmount the target path and delete the remaining directory
	err = mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter.Interface, true /* bind mount */)
	if err != nil {
		return nil, errInternal("NodeUnpublishVolume could not unmount %s: %v", req.GetTargetPath(), err)
	}

	klog.V(4).Infof("NodeUnpublishVolume called with args: %v, targetPath %s", req, req.GetTargetPath())

	// If LUKS volume is used, close the LUKS device
	if err := ns.closeLuksMountSources(req.GetTargetPath()); err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	klog.V(4).Infof("NodeStageVolume called with req: %#v", req)

	// Before to start, validate the request object (NodeStageVolumeRequest)
	if err := validateNodeStageVolumeRequest(req); err != nil {
		return nil, err
	}

	// Get the LinodeVolumeKey which we need to find the device path
	LinodeVolumeKey, err := common.ParseLinodeVolumeKey(req.GetVolumeId())
	if err != nil {
		return nil, err
	}

	// Get device path of attached device
	partition := ""

	if part, ok := req.GetVolumeContext()["partition"]; ok {
		partition = part
	}

	devicePath, err := ns.findDevicePath(*LinodeVolumeKey, partition)
	if err != nil {
		return nil, err
	}

	// Check if staging target path is a valid mount point.
	notMnt, err := ns.ensureMountPoint(req.GetStagingTargetPath(), mountmanager.NewFileSystem())
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
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Check if the volume mode is set to 'Block'
	// Do nothing else with the mount point for stage
	if blk := req.VolumeCapability.GetBlock(); blk != nil {
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Mount device to stagingTargetPath
	// If LUKS is enabled, format the device accordingly
	if err := ns.mountVolume(devicePath, req); err != nil {
		return nil, err
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	klog.V(4).Infof("NodeUnstageVolume called with req: %#v", req)

	// Validate req (NodeUnstageVolumeRequest)
	err := validateNodeUnstageVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	err = mount.CleanupMountPoint(req.GetStagingTargetPath(), ns.mounter.Interface, true /* bind mount */)
	if err != nil {
		return nil, errInternal("NodeUnstageVolume failed to unmount at path %s: %v", req.GetStagingTargetPath(), err)
	}

	// If LUKS volume is used, close the LUKS device
	if err := ns.closeLuksMountSources(req.GetStagingTargetPath()); err != nil {
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	klog.V(4).Infof("NodeExpandVolume called with req: %#v", req)

	// Validate req (NodeExpandVolumeRequest)
	if err := validateNodeExpandVolumeRequest(req); err != nil {
		return nil, err
	}

	// Check linode to see if a give volume exists by volume ID
	// Make call to linode api using the linode api client
	LinodeVolumeKey, err := common.ParseLinodeVolumeKey(req.GetVolumeId())
	if err != nil {
		return nil, errVolumeNotFound(LinodeVolumeKey.VolumeID)
	}
	jsonFilter, err := json.Marshal(map[string]string{"label": LinodeVolumeKey.Label})
	if err != nil {
		return nil, errInternal("marshal json filter: %v", err)
	}
	if _, err = ns.client.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter))); err != nil {
		return nil, errVolumeNotFound(LinodeVolumeKey.VolumeID)
	}

	return &csi.NodeExpandVolumeResponse{
		CapacityBytes: req.CapacityRange.RequiredBytes,
	}, nil
}

func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(4).Infof("NodeGetCapabilities called with req: %#v", req)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.driver.nscap,
	}, nil
}

func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	// Get the number of currently attached instance disks, and subtract it
	// from the limit of block devices that can be attached to the instance,
	// which will effectively give us the number of block storage volumes
	// that can be attached to this node/instance.
	//
	// This is what the spec wants us to report: the actual number of volumes
	// that can be attached, and not the theoretical maximum number of
	// devices that can be attached.
	disks, err := ns.client.ListInstanceDisks(ctx, ns.metadata.ID, nil)
	if err != nil {
		return &csi.NodeGetInfoResponse{}, errInternal("list instance disks: %v", err)
	}
	maxVolumes := maxVolumeAttachments(ns.metadata.Memory) - len(disks)

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
