package driver

import (
	"context"
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

func TestDiskCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		expectedVolumeCount int
		blkInfo             *ghw.BlockInfo
	}{
		{
			name:                "no disks",
			expectedVolumeCount: 0,
			blkInfo:             &ghw.BlockInfo{Disks: []*ghw.Disk{}},
		},
		{
			name:                "skips non-scsi/virtio controllers",
			expectedVolumeCount: 0,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerUnknown, Vendor: "QEMU"},
					{StorageController: block.StorageControllerLoop, Vendor: "QEMU"},
					{StorageController: block.StorageControllerIDE, Vendor: "QEMU"},
				},
			},
		},
		{
			name:                "counts scsi disk with qemu vendor (uppercase)",
			expectedVolumeCount: 1,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerSCSI, Vendor: "QEMU"},
				},
			},
		},
		{
			name:                "counts scsi disk with qemu vendor (lowercase)",
			expectedVolumeCount: 1,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerSCSI, Vendor: "qemu"},
				},
			},
		},
		{
			name:                "counts virtio disk with qemu vendor",
			expectedVolumeCount: 1,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerVirtIO, Vendor: "QEMU"},
				},
			},
		},
		{
			name:                "skips scsi disk with non-qemu vendor",
			expectedVolumeCount: 0,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerSCSI, Vendor: "Linode"},
				},
			},
		},
		{
			name:                "skips virtio disk with non-qemu vendor",
			expectedVolumeCount: 0,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerVirtIO, Vendor: "OtherVendor"},
				},
			},
		},
		{
			name:                "counts multiple qemu disks",
			expectedVolumeCount: 3,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerSCSI, Vendor: "QEMU"},
					{StorageController: block.StorageControllerVirtIO, Vendor: "qemu"},
					{StorageController: block.StorageControllerSCSI, Vendor: "QEMU"},
				},
			},
		},
		{
			name:                "mixed vendors and controllers",
			expectedVolumeCount: 2,
			blkInfo: &ghw.BlockInfo{
				Disks: []*ghw.Disk{
					{StorageController: block.StorageControllerSCSI, Vendor: "QEMU"},     // count
					{StorageController: block.StorageControllerVirtIO, Vendor: "Linode"}, // skip (vendor)
					{StorageController: block.StorageControllerLoop, Vendor: "QEMU"},     // skip (controller)
					{StorageController: block.StorageControllerSCSI, Vendor: "Other"},    // skip (vendor)
					{StorageController: block.StorageControllerVirtIO, Vendor: "qemu"},   // count
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

			count, err := diskCount(context.Background(), mockHW)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != tt.expectedVolumeCount {
				t.Errorf("expected %d, got %d", tt.expectedVolumeCount, count)
			}
		})
	}
}
