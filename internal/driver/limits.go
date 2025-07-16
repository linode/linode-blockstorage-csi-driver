package driver

import (
	"context"
	"strings"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/hwinfo"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
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
func diskCount(ctx context.Context, hw hwinfo.HardwareInfo) (int, error) {
	log, _ := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "diskCount")
	defer done()

	bdev, err := hw.Block()
	if err != nil {
		return 0, err
	}

	log.V(2).Info("Listing disks", "disks", bdev.Disks)

	count := 0
	for _, disk := range bdev.Disks {
		controllerType := strings.ToLower(disk.StorageController.String())
		// Only count disks that are SCSI or virtio
		if controllerType != "scsi" && controllerType != "virtio" {
			continue
		}

		// The boot disk seems to be from vendor QEMU.
		// All other attached disks are from vendor Linode.
		if strings.ToLower(disk.Vendor) != "qemu" {
			continue
		}

		// If the disk passed both the controller and PVC checks, increment the count.
		log.V(2).Info("Incrementing disk count", "disk", disk)
		count++
	}
	return count, nil
}
