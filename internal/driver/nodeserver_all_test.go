package driver

import (
	"context"
	"errors"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testFilesystemStatter struct {
	statfsFunc func(string, *unix.Statfs_t) error
}

func (t *testFilesystemStatter) Statfs(path string, stat *unix.Statfs_t) error {
	return t.statfsFunc(path, stat)
}

func TestNodeGetVolumeStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFsStatter := func(path string, stat *unix.Statfs_t) error {
		switch path {
		case "/valid/path":
			stat.Blocks = 1000
			stat.Bfree = 200
			stat.Bavail = 150
			stat.Files = 500
			stat.Ffree = 100
			stat.Bsize = 4096
			return nil
		case "/not/mounted":
			return unix.EIO
		case "/not/exist":
			return unix.ENOENT
		default:
			return errors.New("internal error")
		}
	}

	// Create a simple implementation for testing
	testStatter := &testFilesystemStatter{statfsFunc: mockFsStatter}

	testCases := []struct {
		name        string
		volumeID    string
		volumePath  string
		expectedErr error
		expectedRes *csi.NodeGetVolumeStatsResponse
	}{
		{
			name:        "Valid request with healthy volume",
			volumeID:    "valid-volume",
			volumePath:  "/valid/path",
			expectedErr: nil,
			expectedRes: &csi.NodeGetVolumeStatsResponse{
				Usage: []*csi.VolumeUsage{
					{
						Available: 150 * 4096,
						Total:     1000 * 4096,
						Used:      (1000 - 200) * 4096,
						Unit:      csi.VolumeUsage_BYTES,
					},
					{
						Available: 100,
						Total:     500,
						Used:      500 - 100,
						Unit:      csi.VolumeUsage_INODES,
					},
				},
				VolumeCondition: &csi.VolumeCondition{
					Abnormal: false,
					Message:  "healthy",
				},
			},
		},
		{
			name:        "Request with empty volume ID or path",
			volumeID:    "",
			volumePath:  "",
			expectedErr: status.Error(codes.InvalidArgument, "volume ID or path empty"),
			expectedRes: nil,
		},
		{
			name:        "Filesystem not mounted",
			volumeID:    "not-mounted-volume",
			volumePath:  "/not/mounted",
			expectedErr: nil,
			expectedRes: &csi.NodeGetVolumeStatsResponse{
				VolumeCondition: &csi.VolumeCondition{
					Abnormal: true,
					Message:  "failed to get stats: input/output error",
				},
			},
		},
		{
			name:        "Volume path does not exist",
			volumeID:    "non-existent-volume",
			volumePath:  "/not/exist",
			expectedErr: status.Errorf(codes.NotFound, "volume path not found: no such file or directory"),
			expectedRes: nil,
		},
		{
			name:        "Internal error during Statfs call",
			volumeID:    "internal-error-volume",
			volumePath:  "/internal/error",
			expectedErr: status.Errorf(codes.Internal, "failed to get stats: internal error"),
			expectedRes: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			req := &csi.NodeGetVolumeStatsRequest{
				VolumeId:   tc.volumeID,
				VolumePath: tc.volumePath,
			}

			resp, err := nodeGetVolumeStats(ctx, req, testStatter)

			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expectedRes, resp)
		})
	}
}
