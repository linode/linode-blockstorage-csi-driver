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

package devicemanager

import (
	"fmt"
	"path"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	klog "k8s.io/klog/v2"

	filesystem "github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

const (
	diskByIdPath         = "/dev/disk/by-id/"
	diskLinodePrefix     = "linode-"
	diskScsiLinodePrefix = "scsi-0Linode_Volume_"
	diskPartitionSuffix  = "-part"
	diskSDPath           = "/dev/sd"
	diskSDPattern        = "/dev/sd*"
)

// DeviceUtils are a collection of methods that act on the devices attached
// to a Linode Instance
type DeviceUtils interface {
	// GetDiskByIdPaths returns a list of all possible paths for a
	// given Persistent Disk
	GetDiskByIdPaths(deviceName string, partition string) []string

	// VerifyDevicePath returns the first of the list of device paths that
	// exists on the machine, or an empty string if none exists
	VerifyDevicePath(devicePaths []string) (string, error)
}

type deviceUtils struct {
	exec mountmanager.Executor
	fs   filesystem.FileSystem
}

var _ DeviceUtils = &deviceUtils{}

func NewDeviceUtils(fs filesystem.FileSystem, exec mountmanager.Executor) *deviceUtils {
	return &deviceUtils{fs: fs, exec: exec}
}

// Returns list of all /dev/disk/by-id/* paths for given PD.
func (m *deviceUtils) GetDiskByIdPaths(deviceName, partition string) []string {
	devicePaths := []string{
		path.Join(diskByIdPath, diskLinodePrefix+deviceName),
		path.Join(diskByIdPath, diskScsiLinodePrefix+deviceName),
	}

	if partition != "" {
		for i, devicePath := range devicePaths {
			devicePaths[i] = devicePath + diskPartitionSuffix + partition
		}
	}

	return devicePaths
}

// Returns the first path that exists, or empty string if none exist.
func (m *deviceUtils) VerifyDevicePath(devicePaths []string) (string, error) {
	sdBefore, err := m.fs.Glob(diskSDPattern)
	if err != nil {
		// Seeing this error means that the diskSDPattern is malformed.
		klog.Errorf("Error filepath.Glob(\"%s\"): %v\r\n", diskSDPattern, err)
	}
	sdBeforeSet := sets.New[string](sdBefore...)
	// TODO(#69): Verify udevadm works as intended in driver
	if err := udevadmChangeToNewDrives(sdBeforeSet, m.fs, m.exec); err != nil {
		// udevadm errors should not block disk detachment, log and continue
		klog.Errorf("udevadmChangeToNewDrives failed with: %v", err)
	}

	for _, devicePath := range devicePaths {
		if pathExists, err := pathExists(devicePath, m.fs); err != nil {
			return "", fmt.Errorf("error checking if path exists: %w", err)
		} else if pathExists {
			return devicePath, nil
		}
	}

	return "", nil
}

// Triggers the application of udev rules by calling "udevadm trigger
// --action=change" for newly created "/dev/sd*" drives (exist only in
// after set). This is workaround for Issue #7972. Once the underlying
// issue has been resolved, this may be removed.

// s1 := Set[string]{} s2 := New[string]()
func udevadmChangeToNewDrives(sdBeforeSet sets.Set[string], fs filesystem.FileSystem, exec mountmanager.Executor) error {
	sdAfter, err := fs.Glob(diskSDPattern)
	if err != nil {
		return fmt.Errorf("error filepath.Glob(\"%s\"): %w", diskSDPattern, err)
	}

	for _, sd := range sdAfter {
		if !sdBeforeSet.Has(sd) {
			return udevadmChangeToDrive(sd, fs, exec)
		}
	}

	return nil
}

// Calls "udevadm trigger --action=change" on the specified drive.
// drivePath must be the block device path to trigger on, in the format "/dev/sd*", or a symlink to it.
// This is workaround for Issue #7972. Once the underlying issue has been resolved, this may be removed.
func udevadmChangeToDrive(drivePath string, fs filesystem.FileSystem, exec mountmanager.Executor) error {
	klog.V(5).Infof("udevadmChangeToDrive: drive=%q", drivePath)

	// Evaluate symlink, if any
	drive, err := fs.EvalSymlinks(drivePath)
	if err != nil {
		return fmt.Errorf("eval symlinks %q: %w", drivePath, err)
	}
	klog.V(5).Infof("udevadmChangeToDrive: symlink path is %q", drive)

	// Check to make sure input is "/dev/sd*"
	if !strings.Contains(drive, diskSDPath) {
		return fmt.Errorf("invalid disk %q: path must start with %q", drive, diskSDPattern)
	}

	// Call "udevadm trigger --action=change --property-match=DEVNAME=/dev/sd..."
	_, err = exec.Command(
		"udevadm",
		"trigger",
		"--action=change",
		fmt.Sprintf("--property-match=DEVNAME=%s", drive)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("udevadm trigger failed for %q: %w", drive, err)
	}
	return nil
}

// PathExists returns true if the specified path exists.
func pathExists(devicePath string, fs filesystem.FileSystem) (bool, error) {
	_, err := fs.Stat(devicePath)
	if err == nil {
		return true, nil
	}
	if fs.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
