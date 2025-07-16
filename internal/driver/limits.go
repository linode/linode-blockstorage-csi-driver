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

// isKubePVCMountPoint checks if a given mount point string matches the pattern
// for a Kubernetes CSI-based PersistentVolumeClaim. This is the most reliable way
// to identify PVCs in a Kubernetes environment.
func isKubePVCMountPoint(ctx context.Context, mountPoint string) bool {
	log, _ := logger.GetLogger(ctx)
	log, done := logger.WithMethod(log, "isKubePVCMountPoint")
	defer done()

	if mountPoint == "" {
		return false
	}

	log.V(2).Info("Checking if mount point is a PVC", "mountPoint", mountPoint)
	// A PVC mount point managed by a CSI driver will contain both of these substrings.
	hasCsiPath := strings.Contains(mountPoint, "/volumes/kubernetes.io~csi/")
	hasPvcPrefix := strings.Contains(mountPoint, "pvc-")
	log.V(2).Info("Checking if mount point is a PVC", "hasCsiPath", hasCsiPath, "hasPvcPrefix", hasPvcPrefix)
	return hasCsiPath && hasPvcPrefix
}

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

		log.V(2).Info("Checking if disk is a PVC", "disk", disk)
		isKubernetesPVC := false
		for _, partition := range disk.Partitions {
			log.V(2).Info("Checking if partition is a PVC", "partition", partition)
			if isKubePVCMountPoint(ctx, partition.MountPoint) {
				isKubernetesPVC = true
				// If we find one partition is a PVC, the whole disk is considered a PVC volume.
				// No need to check other partitions on this disk.
				break
			}
		}

		// If the disk is identified as a PVC, skip it and move to the next one.
		if isKubernetesPVC {
			log.V(2).Info("Skipping disk because it is a PVC", "disk", disk)
			continue
		}

		// If the disk passed both the controller and PVC checks, increment the count.
		log.V(2).Info("Incrementing disk count", "disk", disk)
		count++
	}
	return count, nil
}
