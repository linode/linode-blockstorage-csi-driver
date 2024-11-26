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

// unixStatfs is used to mock the unix.Statfs function.
var unixStatfs = unix.Statfs

func nodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	log := logger.GetLogger(ctx)

	if req.GetVolumeId() == "" || req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID or path empty")
	}

	var statfs unix.Statfs_t
	// See http://man7.org/linux/man-pages/man2/statfs.2.html for details.
	err := unixStatfs(req.GetVolumePath(), &statfs)
	switch {
	case errors.Is(err, unix.EIO):
		// EIO is returned when the filesystem is not mounted.
		return &csi.NodeGetVolumeStatsResponse{
			VolumeCondition: &csi.VolumeCondition{
				Abnormal: true,
				Message:  fmt.Sprintf("failed to get stats: %v", err.Error()),
			},
		}, nil
	case errors.Is(err, unix.ENOENT):
		// ENOENT is returned when the volume path does not exist.
		return nil, status.Errorf(codes.NotFound, "volume path not found: %v", err.Error())
	case err != nil:
		// Any other error is considered an internal error.
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err.Error())
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
			Abnormal: false,
			Message:  "healthy",
		},
	}

	log.V(2).Info("Successfully retrieved volume stats", "volumeID", req.GetVolumeId(), "volumePath", req.GetVolumePath(), "response", response)
	return response, nil
}
