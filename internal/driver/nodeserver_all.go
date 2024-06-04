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
)

func nodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.VolumeId == "" || req.VolumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID or path empty")
	}

	var statfs unix.Statfs_t
	// See http://man7.org/linux/man-pages/man2/statfs.2.html for details.
	err := unix.Statfs(req.VolumePath, &statfs)
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

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: int64(statfs.Bavail) * int64(statfs.Bsize),
				Total:     int64(statfs.Blocks) * int64(statfs.Bsize),
				Used:      (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize),
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
	}, nil
}
