// Copyright 2024 Linode LLC
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package sanity_test

import (
	"os"
	"sync"

	"github.com/jaypipes/ghw"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
)

// setupMockExpectations configures all other mocks to accept any calls
func setupMockExpectations(
	mockCtrl *gomock.Controller,
	mockMounter *mocks.MockMounter,
	mockExecutor *mocks.MockExecutor,
	mockFormater *mocks.MockFormater,
	mockDeviceUtils *mocks.MockDeviceUtils,
	mockResizeFS *mocks.MockResizeFSer,
	mockFileSystem *mocks.MockFileSystem,
	mockCryptSetup *mocks.MockCryptSetupClient,
	mockFsStatter *mocks.MockFilesystemStatter,
) {
	// Track mounted paths for IsLikelyNotMountPoint and published paths for Stats
	mountedPaths := make(map[string]bool)
	publishedPaths := make(map[string]bool)
	var mountMu sync.Mutex

	// Mounter expectations
	mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(source, target, fstype string, options []string) error {
			mountMu.Lock()
			defer mountMu.Unlock()
			mountedPaths[target] = true
			publishedPaths[target] = true
			return nil
		}).AnyTimes()
	mockMounter.EXPECT().MountSensitive(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(source, target, fstype string, options, sensitiveOptions []string) error {
			mountMu.Lock()
			defer mountMu.Unlock()
			mountedPaths[target] = true
			return nil
		}).AnyTimes()
	mockMounter.EXPECT().MountSensitiveWithoutSystemd(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(source, target, fstype string, options, sensitiveOptions []string) error {
			mountMu.Lock()
			defer mountMu.Unlock()
			mountedPaths[target] = true
			return nil
		}).AnyTimes()
	mockMounter.EXPECT().MountSensitiveWithoutSystemdWithMountFlags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(source, target, fstype string, options, sensitiveOptions, mountFlags []string) error {
			mountMu.Lock()
			defer mountMu.Unlock()
			mountedPaths[target] = true
			return nil
		}).AnyTimes()
	mockMounter.EXPECT().Unmount(gomock.Any()).DoAndReturn(
		func(target string) error {
			mountMu.Lock()
			defer mountMu.Unlock()
			delete(mountedPaths, target)
			return nil
		}).AnyTimes()
	mockMounter.EXPECT().List().Return(nil, nil).AnyTimes()
	mockMounter.EXPECT().IsMountPoint(gomock.Any()).DoAndReturn(
		func(file string) (bool, error) {
			mountMu.Lock()
			defer mountMu.Unlock()
			return mountedPaths[file], nil
		}).AnyTimes()
	mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).DoAndReturn(
		func(file string) (bool, error) {
			mountMu.Lock()
			defer mountMu.Unlock()
			return !mountedPaths[file], nil
		}).AnyTimes()
	mockMounter.EXPECT().GetMountRefs(gomock.Any()).Return(nil, nil).AnyTimes()
	mockMounter.EXPECT().CanSafelySkipMountPointCheck().Return(false).AnyTimes()

	// Executor expectations
	mockExecutor.EXPECT().Command(gomock.Any(), gomock.Any()).Return(&fakeCmd{}).AnyTimes()
	mockExecutor.EXPECT().CommandContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(&fakeCmd{}).AnyTimes()
	mockExecutor.EXPECT().LookPath(gomock.Any()).Return("", nil).AnyTimes()

	// Formater expectations
	mockFormater.EXPECT().FormatAndMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// DeviceUtils expectations
	mockDeviceUtils.EXPECT().GetDiskByIdPaths(gomock.Any(), gomock.Any()).Return([]string{"/dev/disk/by-id/test"}).AnyTimes()
	mockDeviceUtils.EXPECT().VerifyDevicePath(gomock.Any()).Return("/dev/sda", nil).AnyTimes()

	// ResizeFS expectations
	mockResizeFS.EXPECT().Resize(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	mockResizeFS.EXPECT().NeedResize(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

	// FileSystem expectations
	mockFileSystem.EXPECT().IsNotExist(gomock.Any()).DoAndReturn(os.IsNotExist).AnyTimes()
	mockFileSystem.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).DoAndReturn(os.MkdirAll).AnyTimes()
	mockFileSystem.EXPECT().Stat(gomock.Any()).DoAndReturn(os.Stat).AnyTimes()
	mockFileSystem.EXPECT().Remove(gomock.Any()).Return(nil).AnyTimes()
	mockFileSystem.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockFileSystem.EXPECT().Open(gomock.Any()).Return(nil, nil).AnyTimes()
	mockFileSystem.EXPECT().Glob(gomock.Any()).Return(nil, nil).AnyTimes()
	mockFileSystem.EXPECT().EvalSymlinks(gomock.Any()).Return("", nil).AnyTimes()

	// CryptSetup expectations - create a mock device to return
	mockDevice := mocks.NewMockDevice(mockCtrl)
	mockDevice.EXPECT().Format(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockDevice.EXPECT().KeyslotAddByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockDevice.EXPECT().ActivateByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockDevice.EXPECT().ActivateByPassphrase(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockDevice.EXPECT().VolumeKeyGet(gomock.Any(), gomock.Any()).Return(nil, 0, nil).AnyTimes()
	mockDevice.EXPECT().Load(gomock.Any()).Return(nil).AnyTimes()
	mockDevice.EXPECT().Free().Return(true).AnyTimes()
	mockDevice.EXPECT().Dump().Return(0).AnyTimes()
	mockDevice.EXPECT().Type().Return("LUKS2").AnyTimes()
	mockDevice.EXPECT().Deactivate(gomock.Any()).Return(nil).AnyTimes()

	mockCryptSetup.EXPECT().Init(gomock.Any()).Return(mockDevice, nil).AnyTimes()
	mockCryptSetup.EXPECT().InitByName(gomock.Any()).Return(mockDevice, nil).AnyTimes()

	// FilesystemStatter expectations
	mockFsStatter.EXPECT().Statfs(gomock.Any(), gomock.Any()).DoAndReturn(
		func(path string, stat *unix.Statfs_t) error {
			// Only return stats for paths that were actually published/mounted
			// This ensures NodeGetVolumeStats fails for non-existent or unpublished volumes
			mountMu.Lock()
			isPublished := publishedPaths[path]
			mountMu.Unlock()

			if !isPublished {
				// Path was never published - return ENOENT
				return unix.ENOENT
			}

			// Return valid filesystem stats for published paths
			stat.Blocks = 1000000
			stat.Bfree = 500000
			stat.Bavail = 450000
			stat.Files = 10000
			stat.Ffree = 5000
			stat.Bsize = 4096
			return nil
		}).AnyTimes()
}

// setupHardwareInfoExpectations configures MockHardwareInfo with stateful behavior
func setupHardwareInfoExpectations(mock *mocks.MockHardwareInfo) {
	// Mock Block() to return 1 boot disk (QEMU vendor, SCSI controller)
	// This simulates a real Linode instance with 1 boot disk
	// With 4GB RAM, maxVolumeAttachments = 8, so 8 - 1 = 7 volumes can be attached
	mock.EXPECT().Block().DoAndReturn(
		func() (*ghw.BlockInfo, error) {
			return &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{
						Name:              "sda",
						Vendor:            "QEMU",
						StorageController: ghw.STORAGE_CONTROLLER_SCSI,
					},
				},
			}, nil
		}).AnyTimes()
}
