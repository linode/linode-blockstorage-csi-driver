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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

const (
	defaultFSType = "ext4"
	rwPermission  = os.FileMode(0755)
	ownerGroupReadWritePermissions = os.FileMode(0660)
)

// TODO: Figure out a better home for these interfaces
type Mounter interface {
	mount.Interface
}

type Executor interface {
	utilexec.Interface
}

type Command interface {
	utilexec.Cmd
}

// ValidateNodeStageVolumeRequest validates the node stage volume request.
// It validates the volume ID, staging target path, and volume capability.
func validateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errNoVolumeID
	}
	if req.GetStagingTargetPath() == "" {
		return errNoStagingTargetPath
	}
	if req.GetVolumeCapability() == nil {
		return errNoVolumeCapability
	}
	return nil

}

// validateNodeUnstageVolumeRequest validates the node unstage volume request.
// It validates the volume ID and staging target path.
func validateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errNoVolumeID
	}
	if req.GetStagingTargetPath() == "" {
		return errNoStagingTargetPath
	}
	return nil

}

// validateNodeExpandVolumeRequest validates the node expand volume request.
// It checks the volume ID and volume path in the provided request.
func validateNodeExpandVolumeRequest(req *csi.NodeExpandVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errNoVolumeID
	}
	if req.GetVolumePath() == "" {
		return errNoVolumePath
	}
	return nil
}

// validateNodePublishVolumeRequest validates the node publish volume request.
// It checks the volume ID, staging target path, target path, and volume capability in the provided request.
func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errNoVolumeID
	}
	if req.GetStagingTargetPath() == "" {
		return errNoStagingTargetPath
	} 
	if req.GetTargetPath() == "" {
		return errNoTargetPath
	}
	if req.GetVolumeCapability() == nil {
		return errNoVolumeCapability
	}
	return nil
}

// getFSTypeAndMountOptions retrieves the file system type and mount options from the given volume capability.
// If the capability is not set, the default file system type and empty mount options will be returned.
func getFSTypeAndMountOptions(volumeCapability *csi.VolumeCapability) (string, []string) {
	// Use default file system type if not specified in the volume capability
	fsType := defaultFSType
	// Use mount options from the volume capability if specified
	var mountOptions []string

	if mnt := volumeCapability.GetMount(); mnt != nil {
		// Use file system type from volume capability if specified
		if mnt.FsType != "" {
			fsType = mnt.FsType
		}
		// Use mount options from volume capability if specified
		if mnt.MountFlags != nil {
			mountOptions = mnt.MountFlags
		}
	}

	return fsType, mountOptions
}

// findDevicePath locates the device path for a Linode Volume.
//
// It uses the provided LinodeVolumeKey and partition information to generate
// possible device paths, then verifies which path actually exists on the system.
func (ns *LinodeNodeServer) findDevicePath(key common.LinodeVolumeKey, partition string) (string, error) {
	// Get the device name and paths from the LinodeVolumeKey and partition.
	deviceName := key.GetNormalizedLabel()
	devicePaths := ns.DeviceUtils.GetDiskByIdPaths(deviceName, partition)

	// Verify the device path by checking if any of the paths exist.
	devicePath, err := ns.DeviceUtils.VerifyDevicePath(devicePaths)
	if err != nil {
		return "", status.Error(codes.Internal, fmt.Sprintf("Error verifying Linode Volume (%q) is attached: %v",
			key.GetVolumeLabel(), err))
	}

	// If no device path is found, return an error.
	if devicePath == "" {
		return "", status.Error(codes.Internal, fmt.Sprintf("Unable to find device path out of attempted paths: %v",
			devicePaths))
	}

	// If a device path is found, return it.
	klog.V(4).Infof("Successfully found attached Linode Volume %q at device path %s.", deviceName, devicePath)
	return devicePath, nil
}

// ensureMountPoint checks if the staging target path is a mount point or not.
// If not, it creates a directory at the target path.
func (ns *LinodeNodeServer) ensureMountPoint(path string, fs mountmanager.FileSystem) (bool, error) {
	// Check if the staging target path is a mount point.
	notMnt, err := ns.Mounter.IsLikelyNotMountPoint(path)
	if err != nil {
		// Checking IsNotExist returns true. If true, it mean we need to create directory at the target path.
		if fs.IsNotExist(err) {
			if err := fs.MkdirAll(path, rwPermission); err != nil {
				return true, status.Error(codes.Internal, fmt.Sprintf("Failed to create directory (%q): %v", path, err))
			}
		} else {
			// If the error is unknown, return an error.
			return true, status.Error(codes.Internal, fmt.Sprintf("Unknown error when checking mount point (%q): %v", path, err))
		}
	}
	return notMnt, nil
}

// nodePublishVolumeBlock handles the NodePublishVolume call for block volumes.
//
// It takes a CSI NodePublishVolumeRequest, a list of mount options, and a file system interface.
// The CSI NodePublishVolumeRequest contains the volume ID, target path, and publish context.
// The publish context is expected to contain the device path of the volume to be published.
// The function creates the target directory, creates a file to bind mount the block device to,
// and mounts the volume using the provided mount options.
// It returns a CSI NodePublishVolumeResponse and an error if the operation fails.
func (ns *LinodeNodeServer) nodePublishVolumeBlock(req *csi.NodePublishVolumeRequest, mountOptions []string, fs mountmanager.FileSystem) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	targetPathDir := filepath.Dir(targetPath)

	// Get the device path from the request
	devicePath := req.PublishContext["devicePath"]
	if devicePath == "" {
		return nil, status.Error(codes.Internal, "devicePath cannot be found")
	}

	// Create directory at the directory level of given path
	klog.V(5).Infof("NodePublishVolume[block]: making targetPathDir %s", targetPathDir)
	if err := fs.MkdirAll(targetPathDir, rwPermission); err != nil {
		klog.Errorf("mkdir failed on disk %s (%v)", targetPathDir, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to create directory (%q): %v", targetPathDir, err))
	}

	// Make file to bind mount block device to file
	klog.V(5).Infof("NodePublishVolume[block]: making target block bind mount device file %s", targetPath)
	file, err := fs.OpenFile(targetPath, os.O_CREATE, ownerGroupReadWritePermissions)
	if err != nil {
		if removeErr := fs.Remove(targetPath); removeErr != nil {
			return nil, status.Errorf(codes.Internal, "Failed remove mount target %s: %v", targetPath, err)
		}
		return nil, status.Errorf(codes.Internal, "Failed to create file %s: %v", targetPath, err)
	}
	file.Close()

	// Mount the volume
	if err := ns.Mounter.Mount(devicePath, targetPath, "", mountOptions); err != nil {
		if removeErr := fs.Remove(targetPath); removeErr != nil {
			return nil, status.Errorf(codes.Internal, "Failed remove mount target %q: %v", targetPath, err)
		}
		return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", devicePath, targetPath, err)
	}	

	return &csi.NodePublishVolumeResponse{}, nil
}

// mountVolume formats and mounts a volume to the staging target path.
//
// It handles both encrypted (LUKS) and non-encrypted volumes. For LUKS volumes,
// it prepares the encrypted volume before mounting. The function determines
// the filesystem type and mount options from the volume capability.
func (ns *LinodeNodeServer) mountVolume(devicePath string, req *csi.NodeStageVolumeRequest) error {
	stagingTargetPath := req.GetStagingTargetPath()
	volumeCapability := req.GetVolumeCapability()

	// Retrieve the file system type and mount options from the volume capability
	fsType, mountOptions := getFSTypeAndMountOptions(volumeCapability)

	fmtAndMountSource := devicePath

	// Check if LUKS encryption is enabled and prepare the LUKS volume if needed
	luksContext := getLuksContext(req.Secrets, req.VolumeContext, VolumeLifecycleNodeStageVolume)
	if luksContext.EncryptionEnabled {
		var err error
		fmtAndMountSource, err = ns.prepareLUKSVolume(devicePath, luksContext)
		if err != nil {
			return err
		}
	}

	// Format and mount the drive
	klog.V(4).Info("formatting and mounting the drive")
	if err := ns.Mounter.FormatAndMount(fmtAndMountSource, stagingTargetPath, fsType, mountOptions); err != nil {
		return status.Error(codes.Internal,
			fmt.Sprintf("Failed to format and mount device from (%q) to (%q) with fstype (%q) and options (%q): %v",
				devicePath, stagingTargetPath, fsType, mountOptions, err))
	}

	return nil
}

// prepareLUKSVolume prepares a LUKS-encrypted volume for mounting.
//
// It checks if the device at devicePath is already formatted with LUKS encryption.
// If not, it formats the device using the provided LuksContext.
// Finally, it prepares the LUKS volume for mounting.
func (ns *LinodeNodeServer) prepareLUKSVolume(devicePath string, luksContext LuksContext) (string, error) {
	// LUKS encryption enabled, check if the volume needs to be formatted.
	klog.V(4).Info("LUKS encryption enabled")

	// Validate if the device is formatted with LUKS encryption or if it needs formatting.
	formatted, err := ns.Encrypt.blkidValid(devicePath)
	if err != nil {
		return "", status.Error(codes.Internal, fmt.Sprintf("Failed to validate blkid (%q): %v", devicePath, err))
	}

	// If the device is not, format it.
	if !formatted {
		klog.V(4).Info("luks volume now formatting: ", devicePath)

		// Validate the LUKS context.
		if err := luksContext.validate(); err != nil {
			return "", status.Error(codes.Internal, fmt.Sprintf("Failed to luks format validation (%q): %v", devicePath, err))
		}

		// Format the volume with LUKS encryption.
		if err := ns.Encrypt.luksFormat(luksContext, devicePath); err != nil {
			return "", status.Error(codes.Internal, fmt.Sprintf("Failed to luks format (%q): %v", devicePath, err))
		}
	}

	// Prepare the LUKS volume for mounting.
	luksSource, err := ns.Encrypt.luksPrepareMount(luksContext, devicePath)
	if err != nil {
		return "", status.Error(codes.Internal, fmt.Sprintf("Failed to prepare luks mount (%q): %v", devicePath, err))
	}

	return luksSource, nil
}

// closeMountSources closes any LUKS-encrypted mount sources associated with the given path.
// It retrieves mount sources, checks if each source is a LUKS mapping, and closes it if so.
// Returns an error if any operation fails during the process.
func (ns *LinodeNodeServer) closeLuksMountSources(path string) error {
	mountSources, err := ns.getMountSources(path)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("closeMountSources failed to to get mount sources %s: %v", path, err))
	}
	klog.V(4).Info("closing mount sources: ", mountSources)
	for _, source := range mountSources {
		isLuksMapping, mappingName, err := ns.Encrypt.isLuksMapping(source)
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("closeMountSources failed determine if mount is a luks mapping %s: %v", path, err))
		}
		if isLuksMapping {
			klog.V(4).Infof("luksClose %s", mappingName)
			if err := ns.Encrypt.luksClose(mappingName); err != nil {
				return status.Error(codes.Internal, fmt.Sprintf("closeMountSources failed to close luks mount %s: %v", path, err))
			}
		}
	}

	return nil
}

// getMountSources retrieves the mount sources for a given target path using the 'findmnt' command.
// It returns a slice of strings containing the mount sources, or an error if the operation fails.
// If 'findmnt' is not found or returns no results, appropriate errors or an empty slice are returned.
func (ns *LinodeNodeServer) getMountSources(target string) ([]string, error) {
	_, err := ns.Mounter.Exec.LookPath("findmnt")
	if err != nil {
		if err == exec.ErrNotFound {
			return nil, fmt.Errorf("%q executable not found in $PATH", "findmnt")
		}
		return nil, err
	}
	out, err := ns.Mounter.Exec.Command("sh", "-c", fmt.Sprintf("findmnt -o SOURCE -n -M %s", target)).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("checking mounted failed: %v cmd: %q output: %q",
			err, "findmnt", string(out))
	}
	return strings.Split(string(out), "\n"), nil
}
