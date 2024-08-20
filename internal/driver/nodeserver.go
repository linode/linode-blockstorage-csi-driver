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
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"github.com/linode/linodego"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

type LinodeNodeServer struct {
	Driver        *LinodeDriver
	Mounter       *mount.SafeFormatAndMount
	DeviceUtils   mountmanager.DeviceUtils
	CloudProvider linodeclient.LinodeClient
	Metadata      Metadata
	Encrypt       Encryption
	// TODO: Only lock mutually exclusive calls and make locking more fine grained
	mux sync.Mutex

	csi.UnimplementedNodeServer
}

var _ csi.NodeServer = &LinodeNodeServer{}

func (ns *LinodeNodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	klog.V(4).Infof("NodePublishVolume called with req: %#v", req)

	// Validate Arguments
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, err
	}

	targetPath := req.GetTargetPath()
	readOnly := req.GetReadonly()
	volumeCapability := req.GetVolumeCapability()

	// Setting staging target path
	stagingTargetPath := req.GetStagingTargetPath()
	// If block volume, set staging target path to device path
	if volumeCapability.GetBlock() != nil {
		stagingTargetPath = req.PublishContext["devicePath"]
	}
	
	// Set mount options:
	//  - bind mount to the full path to allow duplicate mounts of the same PD.
	//  - read-only if specified
	options := []string{"bind"}
	if readOnly {
		options = append(options, "ro")
	}

	notMnt, err := ns.Mounter.Interface.IsLikelyNotMountPoint(targetPath)
	if err != nil && !os.IsNotExist(err) {
		klog.Errorf("cannot validate mount point: %s %v", targetPath, err)
		return nil, err
	}
	if !notMnt {
		// TODO(#95): check if mount is compatible. Return OK if it is, or appropriate error.
		/*
			1) Target Path MUST be the vol referenced by vol ID
			2) VolumeCapability MUST match
			3) Readonly MUST match

		*/
		return &csi.NodePublishVolumeResponse{}, nil
	}

	if blk := volumeCapability.GetBlock(); blk != nil {
		// VolumeMode: Block
		klog.V(5).Infof("NodePublishVolume[block]: making targetPathDir %s", targetPathDir)
		if err := os.MkdirAll(targetPathDir, os.FileMode(0755)); err != nil {
			klog.Errorf("mkdir failed on disk %s (%v)", targetPathDir, err)
			return nil, err
		}

		// Update staging path to devicePath
		stagingTargetPath = req.PublishContext["devicePath"]
		klog.V(5).Infof("NodePublishVolume[block]: set stagingTargetPath to devicePath %s", stagingTargetPath)

		// Make file to bind mount device to file
		klog.V(5).Infof("NodePublishVolume[block]: making target block bind mount device file %s", targetPath)
		file, err := os.OpenFile(targetPath, os.O_CREATE, 0660)
		if err != nil {
			if removeErr := os.Remove(targetPath); removeErr != nil {
				return nil, status.Errorf(codes.Internal, "Failed remove mount target %s: %v", targetPath, err)
			}
			return nil, status.Errorf(codes.Internal, "Failed to create file %s: %v", targetPath, err)
		}
		file.Close()
	} else {
		// VolumeMode: Filesystem
		klog.V(5).Infof("NodePublishVolume[filesystem]: making targetPath %s", targetPath)
		if err := os.MkdirAll(targetPath, os.FileMode(0755)); err != nil {
			klog.Errorf("mkdir failed on disk %s (%v)", targetPath, err)
			return nil, err
		}
	}

	// Mount Source to Target
	err = ns.Mounter.Interface.Mount(stagingTargetPath, targetPath, "ext4", options)
	if err != nil {
		notMnt, mntErr := ns.Mounter.Interface.IsLikelyNotMountPoint(targetPath)
		if mntErr != nil {
			klog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
			return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to check whether target path is a mount point: %v", err))
		}
		if !notMnt {
			if mntErr = ns.Mounter.Interface.Unmount(targetPath); mntErr != nil {
				klog.Errorf("Failed to unmount: %v", mntErr)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to unmount target path: %v", err))
			}
			notMnt, mntErr := ns.Mounter.Interface.IsLikelyNotMountPoint(targetPath)
			if mntErr != nil {
				klog.Errorf("IsLikelyNotMountPoint check failed: %v", mntErr)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to check whether target path is a mount point: %v", err))
			}
			if !notMnt {
				// This is very odd, we don't expect it.  We'll try again next sync loop.
				klog.Errorf("%s is still mounted, despite call to unmount().  Will try again next sync loop.", targetPath)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume something is wrong with mounting: %v", err))
			}
		}
		os.Remove(targetPath)
		klog.Errorf("Mount of disk %s failed: %v", targetPath, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume mount of disk failed: %v", err))
	}

	klog.V(4).Infof("NodePublishVolume successfully mounted %s", targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *LinodeNodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	// Validate Arguments
	targetPath := req.GetTargetPath()
	volID := req.GetVolumeId()
	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	err := mount.CleanupMountPoint(targetPath, ns.Mounter.Interface, true /* bind mount */)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Unmount failed: %v\nUnmounting arguments: %s\n", err, targetPath))
	}

	klog.V(4).Infof("NodeUnpublishVolume called with args: %v, targetPath %s", req, targetPath)

	// If LUKS volume is used, close the LUKS device
	if err := ns.closeLuksMountSources(targetPath); err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *LinodeNodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
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
	notMnt, err := ns.ensureMountPoint(req.GetStagingTargetPath(), NewFileSystem())
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

func (ns *LinodeNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	ns.mux.Lock()
	defer ns.mux.Unlock()
	klog.V(4).Infof("NodeUnstageVolume called with req: %#v", req)

	// Validate req (NodeUnstageVolumeRequest)
	err := validateNodeUnstageVolumeRequest(req)
	if err != nil {
		return nil, err
	}

	err = mount.CleanupMountPoint(req.GetStagingTargetPath(), ns.Mounter.Interface, true /* bind mount */)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("NodeUnstageVolume failed to unmount at path %s: %v", req.GetStagingTargetPath(), err))
	}

	// If LUKS volume is used, close the LUKS device
	if err := ns.closeLuksMountSources(req.GetStagingTargetPath()); err != nil {
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *LinodeNodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	klog.V(4).Infof("NodeExpandVolume called with req: %#v", req)

	// Validate req (NodeExpandVolumeRequest)
	if err := validateNodeExpandVolumeRequest(req); err != nil {
		return nil, err
	}

	// Check linode to see if a give volume exists by volume ID
	// Make call to linode api using the linode api client
	LinodeVolumeKey, err := common.ParseLinodeVolumeKey(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Error parsing linode volume key: %v", err)
	}
	jsonFilter, err := json.Marshal(map[string]string{"label": LinodeVolumeKey.Label})
	if err != nil {
		return nil, errInternal("marshal json filter: %v", err)
	}
	if _, err = ns.CloudProvider.ListVolumes(ctx, linodego.NewListOptions(0, string(jsonFilter))); err != nil {
		return nil, status.Errorf(codes.NotFound, "list volumes: %v", err)
	}

	return &csi.NodeExpandVolumeResponse{
		CapacityBytes: req.CapacityRange.RequiredBytes,
	}, nil
}

func (ns *LinodeNodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(4).Infof("NodeGetCapabilities called with req: %#v", req)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.Driver.nscap,
	}, nil
}

func (ns *LinodeNodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	// Get the number of currently attached instance disks, and subtract it
	// from the limit of block devices that can be attached to the instance,
	// which will effectively give us the number of block storage volumes
	// that can be attached to this node/instance.
	//
	// This is what the spec wants us to report: the actual number of volumes
	// that can be attached, and not the theoretical maximum number of
	// devices that can be attached.
	disks, err := ns.CloudProvider.ListInstanceDisks(ctx, ns.Metadata.ID, nil)
	if err != nil {
		return &csi.NodeGetInfoResponse{}, status.Errorf(codes.Internal, "list instance disks: %v", err)
	}
	maxVolumes := maxVolumeAttachments(ns.Metadata.Memory) - len(disks)

	return &csi.NodeGetInfoResponse{
		NodeId:            strconv.Itoa(ns.Metadata.ID),
		MaxVolumesPerNode: int64(maxVolumes),
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				"topology.linode.com/region": ns.Metadata.Region,
			},
		},
	}, nil
}

func (ns *LinodeNodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nodeGetVolumeStats(req)
}
