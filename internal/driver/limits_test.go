package driver

import (
	"fmt"
	"testing"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/block"
	"go.uber.org/mock/gomock"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
)

const (
	DRIVE_TYPE_UNKNOWN = iota
	DRIVE_TYPE_HDD
	DRIVE_TYPE_FDD
	DRIVE_TYPE_ODD
	DRIVE_TYPE_SSD
	DRIVE_TYPE_VIRTUAL
)

func TestMaxVolumeAttachments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		memory uint
		want   int
	}{
		{memory: 1 << 30, want: maxPersistentAttachments},
		{memory: 2 << 30, want: maxPersistentAttachments},
		{memory: 4 << 30, want: maxPersistentAttachments},
		{memory: 8 << 30, want: maxPersistentAttachments},
		{memory: 16 << 30, want: 16},
		{memory: 32 << 30, want: 32},
		{memory: 64 << 30, want: maxAttachments},
		{memory: 96 << 30, want: maxAttachments},
		{memory: 128 << 30, want: maxAttachments},
		{memory: 150 << 30, want: maxAttachments},
		{memory: 256 << 30, want: maxAttachments},
		{memory: 300 << 30, want: maxAttachments},
		{memory: 512 << 30, want: maxAttachments},
	}

	for _, tt := range tests {
		tname := fmt.Sprintf("%dGB", tt.memory>>30)
		t.Run(tname, func(t *testing.T) {
			got := maxVolumeAttachments(tt.memory)
			if got != tt.want {
				t.Errorf("want=%d got=%d", tt.want, got)
			}
		})
	}
}

func TestAttachedVolumeCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		expectedVolumeCount int
		expectedError       error
		blkInfo             *ghw.BlockInfo
	}{
		{
			name:                "AllLoopDevices",
			expectedVolumeCount: 0,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{
						DriveType:         DRIVE_TYPE_VIRTUAL,
						StorageController: block.StorageControllerUnknown,
						IsRemovable:       false,
						Partitions: []*ghw.Partition{
							{
								Name:       "vd1p1",
								MountPoint: "/mnt/virtual1",
								SizeBytes:  1024 * 1024 * 1024,
							},
						},
					},
					{
						DriveType:         DRIVE_TYPE_VIRTUAL,
						StorageController: block.StorageControllerLoop,
						IsRemovable:       false,
						Partitions: []*ghw.Partition{
							{
								Name:       "loop1",
								MountPoint: "/mnt/virtual1",
								SizeBytes:  1024 * 1024 * 1024,
							},
						},
					},
				},
			},
		},
		{
			name:                "OneDisk",
			expectedVolumeCount: 1,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{
						DriveType:         DRIVE_TYPE_VIRTUAL,
						IsRemovable:       false,
						StorageController: block.StorageControllerUnknown,
						Partitions: []*ghw.Partition{
							{
								Name:       "vd1p1",
								MountPoint: "/mnt/virtual1",
								SizeBytes:  1024 * 1024 * 1024,
							},
						},
					},
					{
						DriveType:         DRIVE_TYPE_VIRTUAL,
						IsRemovable:       false,
						StorageController: block.StorageControllerLoop,
						Partitions: []*ghw.Partition{
							{
								Name:       "loop1",
								MountPoint: "/foo",
								SizeBytes:  1024 * 1024 * 1024,
							},
						},
					},
					{
						DriveType:         DRIVE_TYPE_SSD,
						IsRemovable:       false,
						StorageController: block.StorageControllerSCSI,
						Partitions: []*ghw.Partition{
							{
								Name:       "nvme0n1",
								MountPoint: "/foo",
								SizeBytes:  1024 * 1024 * 1024,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHW := mocks.NewMockHardwareInfo(ctrl)
			mockHW.EXPECT().Block().Return(tt.blkInfo, nil)

			count, err := attachedVolumeCount(mockHW)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != tt.expectedVolumeCount {
				t.Errorf("expected %d, got %d", tt.expectedVolumeCount, count)
			}
		})
	}
}
