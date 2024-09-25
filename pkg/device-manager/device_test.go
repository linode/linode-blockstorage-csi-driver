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
	"os"
	"reflect"
	"testing"

	mocks "github.com/linode/linode-blockstorage-csi-driver/mocks"
	"go.uber.org/mock/gomock"
)

func Test_deviceUtils_GetDiskByIdPaths(t *testing.T) {
	tests := []struct {
		name          string
		deviceName    string
		partition     string
		expectedPaths []string
	}{
		{
			name:       "No partition",
			deviceName: "vol-123",
			partition:  "",
			expectedPaths: []string{
				"/dev/disk/by-id/linode-vol-123",
				"/dev/disk/by-id/scsi-0Linode_Volume_vol-123",
			},
		},
		{
			name:       "With partition",
			deviceName: "vol-456",
			partition:  "1",
			expectedPaths: []string{
				"/dev/disk/by-id/linode-vol-456-part1",
				"/dev/disk/by-id/scsi-0Linode_Volume_vol-456-part1",
			},
		},
		{
			name:       "Empty device name",
			deviceName: "",
			partition:  "",
			expectedPaths: []string{
				"/dev/disk/by-id/linode-",
				"/dev/disk/by-id/scsi-0Linode_Volume_",
			},
		},
		{
			name:       "Empty device name with partition",
			deviceName: "",
			partition:  "2",
			expectedPaths: []string{
				"/dev/disk/by-id/linode--part2",
				"/dev/disk/by-id/scsi-0Linode_Volume_-part2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFs := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)

			deviceUtils := NewDeviceUtils(mockFs, mockExec)
			gotPaths := deviceUtils.GetDiskByIdPaths(tt.deviceName, tt.partition)

			if !reflect.DeepEqual(gotPaths, tt.expectedPaths) {
				t.Errorf("GetDiskByIdPaths() = %v, want %v", gotPaths, tt.expectedPaths)
			}
		})
	}
}

func Test_deviceUtils_VerifyDevicePath(t *testing.T) {
	tests := []struct {
		name        string
		devicePaths []string
		want        string
		wantErr     bool
		setup       func(*mocks.MockFileSystem, *mocks.MockExecutor, *mocks.MockCommand)
	}{
		{
			name:        "First path exists",
			devicePaths: []string{"/dev/disk/by-id/linode-vol-123", "/dev/disk/by-id/scsi-0Linode_Volume_vol-123"},
			want:        "/dev/disk/by-id/linode-vol-123",
			wantErr:     false,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().Glob(diskSDPattern).Return([]string{"/dev/sda", "/dev/sdb"}, nil).AnyTimes()
				mockFs.EXPECT().Stat("/dev/disk/by-id/linode-vol-123").Return(nil, nil).Times(1)
			},
		},
		{
			name:        "Second path exists",
			devicePaths: []string{"/dev/disk/by-id/linode-vol-123", "/dev/disk/by-id/scsi-0Linode_Volume_vol-123"},
			want:        "/dev/disk/by-id/scsi-0Linode_Volume_vol-123",
			wantErr:     false,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().Glob(diskSDPattern).Return([]string{"/dev/sda", "/dev/sdb"}, nil).AnyTimes()
				mockFs.EXPECT().Stat("/dev/disk/by-id/linode-vol-123").Return(nil, os.ErrNotExist).Times(1)
				mockFs.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(1)
				mockFs.EXPECT().Stat("/dev/disk/by-id/scsi-0Linode_Volume_vol-123").Return(nil, nil).Times(1)
			},
		},
		{
			name:        "No paths exist",
			devicePaths: []string{"/dev/disk/by-id/linode-vol-123", "/dev/disk/by-id/scsi-0Linode_Volume_vol-123"},
			want:        "",
			wantErr:     false,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().Glob(diskSDPattern).Return([]string{"/dev/sda", "/dev/sdb"}, nil).AnyTimes()
				mockFs.EXPECT().Stat("/dev/disk/by-id/linode-vol-123").Return(nil, os.ErrNotExist).Times(1)
				mockFs.EXPECT().Stat("/dev/disk/by-id/scsi-0Linode_Volume_vol-123").Return(nil, os.ErrNotExist).Times(1)
				mockFs.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(2)
			},
		},
		{
			name:        "Invoke udevadmChangeToDrive path",
			devicePaths: []string{"/dev/disk/by-id/linode-vol-123", "/dev/disk/by-id/scsi-0Linode_Volume_vol-123"},
			want:        "",
			wantErr:     false,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().Glob(diskSDPattern).Return([]string{"/dev/sda", "/dev/sdb"}, nil).Times(1)
				mockFs.EXPECT().Glob(diskSDPattern).Return([]string{"/dev/sda", "/dev/sdb", "/dev/sdw"}, nil).Times(1)

				mockFs.EXPECT().EvalSymlinks("/dev/sdw").Return("/dev/sdw", nil)
				mockExec.EXPECT().Command("udevadm", "trigger", "--action=change", "--property-match=DEVNAME=/dev/sdw").Return(mockCmd)
				mockCmd.EXPECT().CombinedOutput().Return([]byte(""), nil)

				mockFs.EXPECT().Stat("/dev/disk/by-id/linode-vol-123").Return(nil, os.ErrNotExist).Times(1)
				mockFs.EXPECT().Stat("/dev/disk/by-id/scsi-0Linode_Volume_vol-123").Return(nil, os.ErrNotExist).Times(1)
				mockFs.EXPECT().IsNotExist(os.ErrNotExist).Return(true).Times(2)
			},
		},
		{
			name:        "Error in Glob",
			devicePaths: []string{"/dev/disk/by-id/linode-vol-123"},
			want:        "/dev/disk/by-id/linode-vol-123",
			wantErr:     false,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().Glob(diskSDPattern).Return(nil, fmt.Errorf("glob error")).Times(2)
				mockFs.EXPECT().Stat("/dev/disk/by-id/linode-vol-123").Return(nil, nil).Times(1)
			},
		},
		{
			name:        "Error in Stat",
			devicePaths: []string{"/dev/disk/by-id/linode-vol-123"},
			want:        "",
			wantErr:     true,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().Glob(diskSDPattern).Return([]string{"/dev/sda"}, nil).Times(2)
				mockFs.EXPECT().Stat("/dev/disk/by-id/linode-vol-123").Return(nil, fmt.Errorf("stat error")).Times(1)
				mockFs.EXPECT().IsNotExist(fmt.Errorf("stat error")).Return(false).Times(1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFs := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCmd := mocks.NewMockCommand(ctrl)

			if tt.setup != nil {
				tt.setup(mockFs, mockExec, mockCmd)
			}

			m := NewDeviceUtils(mockFs, mockExec)
			got, err := m.VerifyDevicePath(tt.devicePaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("deviceUtils.VerifyDevicePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("deviceUtils.VerifyDevicePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_udevadmChangeToDrive(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		name      string
		drivePath string
		wantErr   bool
		setup     func(*mocks.MockFileSystem, *mocks.MockExecutor, *mocks.MockCommand)
	}{
		{
			name:      "Success case",
			drivePath: "/dev/sda",
			wantErr:   false,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().EvalSymlinks("/dev/sda").Return("/dev/sda", nil)
				mockExec.EXPECT().Command("udevadm", "trigger", "--action=change", "--property-match=DEVNAME=/dev/sda").Return(mockCmd)
				mockCmd.EXPECT().CombinedOutput().Return([]byte(""), nil)
			},
		},
		{
			name:      "EvalSymlinks error",
			drivePath: "/dev/sda",
			wantErr:   true,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().EvalSymlinks("/dev/sda").Return("", fmt.Errorf("symlink error"))
			},
		},
		{
			name:      "Invalid disk path",
			drivePath: "/dev/invalid",
			wantErr:   true,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().EvalSymlinks("/dev/invalid").Return("/dev/invalid", nil)
			},
		},
		{
			name:      "udevadm command error",
			drivePath: "/dev/sda",
			wantErr:   true,
			setup: func(mockFs *mocks.MockFileSystem, mockExec *mocks.MockExecutor, mockCmd *mocks.MockCommand) {
				mockFs.EXPECT().EvalSymlinks("/dev/sda").Return("/dev/sda", nil)
				mockExec.EXPECT().Command("udevadm", "trigger", "--action=change", "--property-match=DEVNAME=/dev/sda").Return(mockCmd)
				mockCmd.EXPECT().CombinedOutput().Return([]byte(""), fmt.Errorf("command error"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFs := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCmd := mocks.NewMockCommand(ctrl)

			if tt.setup != nil {
				tt.setup(mockFs, mockExec, mockCmd)
			}

			if err := udevadmChangeToDrive(tt.drivePath, mockFs, mockExec); (err != nil) != tt.wantErr {
				t.Errorf("udevadmChangeToDrive() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
