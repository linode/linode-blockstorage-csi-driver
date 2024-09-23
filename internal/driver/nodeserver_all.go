//go:build !windows

package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
)

func nodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	log := logger.GetLogger(ctx)

	if req.GetVolumeId() == "" || req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID or path empty")
	}

	var statfs unix.Statfs_t
	// See http://man7.org/linux/man-pages/man2/statfs.2.html for details.
	err := unix.Statfs(req.GetVolumePath(), &statfs)
	if err != nil && !errors.Is(err, unix.EIO) {
		if errors.Is(err, unix.ENOENT) {
			return nil, status.Errorf(codes.NotFound, "volume path not found: %v", err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err.Error())
	}

	// If we got a filesystem error that suggests things are not well with this volume
	var abnormal bool
	if errors.Is(err, unix.EIO) {
		abnormal = true
	}

	response := &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: int64(statfs.Bavail) * int64(statfs.Bsize),              //nolint:unconvert // probably false positive because uint32 and int64 dont match
				Total:     int64(statfs.Blocks) * int64(statfs.Bsize),              //nolint:unconvert // probably false positive because uint32 and int64 dont match
				Used:      int64(statfs.Blocks-statfs.Bfree) * int64(statfs.Bsize), //nolint:unconvert // probably false positive because uint32 and int64 dont match
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: int64(statfs.Ffree),
				Total:     int64(statfs.Files),
				Used:      int64(statfs.Files) - int64(statfs.Ffree),
				Unit:      csi.VolumeUsage_INODES,
			},
		},
		VolumeCondition: &csi.VolumeCondition{
			Abnormal: abnormal,
			Message:  fmt.Sprintf("failed to call statfs on volume, got err: %s", err),
		},
	}

	log.V(2).Info("Successfully retrieved volume stats", "volumeID", req.GetVolumeId(), "volumePath", req.GetVolumePath(), "response", response)
	return response, nil
}
