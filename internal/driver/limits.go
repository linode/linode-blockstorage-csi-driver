package driver

import (
	"strings"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/hwinfo"
)

// maxVolumeAttachments returns the maximum number of block storage volumes
// that can be attached to a Linode instance, given the amount of memory the
// instance has.
func maxVolumeAttachments(memoryBytes uint) int {
	attachments := int(memoryBytes >> 30)
	return min(max(attachments, maxPersistentAttachments), maxAttachments)
}

const (
	// maxPersistentAttachments is the default number of volume attachments
	// allowed when they are persisted to an instance/boot config. This is
	// also the maximum number of allowed volume attachments when the
	// instance type has < 16GiB of RAM.
	maxPersistentAttachments = 8

	// maxAttachments it the hard limit of volumes that can be attached to
	// a single Linode instance.
	maxAttachments = 64
)

// diskCount calculates the number of attached block devices that are not
// being used as Kubernetes PersistentVolumeClaims (PVCs).
func diskCount(hw hwinfo.HardwareInfo) (int, error) {
	bdev, err := hw.Block()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, disk := range bdev.Disks {
		controllerType := strings.ToLower(disk.StorageController.String())
		// Only count disks that are SCSI or virtio
		if controllerType != "scsi" && controllerType != "virtio" {
			continue
		}

		// The boot disk & swap disk are from vendor QEMU.
		// All other attached volumes are from vendor Linode.
		if !strings.EqualFold(disk.Vendor, "qemu") {
			continue
		}

		// If the disk passed both the controller and vendor checks, increment
		// the count.
		count++
	}
	return count, nil
}
