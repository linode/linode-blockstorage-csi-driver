package driver

import (
	"fmt"
	osexec "os/exec"
	"reflect"
	"runtime"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

// compareGRPCErrors compares two gRPC errors for equality.
//
// Parameters:
//   - t: The testing.T instance to log errors.
//   - got: The error received during the test.
//   - want: The expected error for comparison.
//
// Returns: void
func compareGRPCErrors(t *testing.T, got, want error) {
	t.Helper()

	if (got == nil) != (want == nil) {
		t.Errorf("Error presence mismatch: got %v, want %v", got, want)
		return
	}

	if got == nil && want == nil {
		return // Both are nil, so they're equal
	}

	gotStatus, ok := status.FromError(got)
	if !ok {
		t.Errorf("Got error is not a gRPC status error: %v", got)
		return
	}

	wantStatus, ok := status.FromError(want)
	if !ok {
		t.Errorf("Want error is not a gRPC status error: %v", want)
		return
	}

	if gotStatus.Code() != wantStatus.Code() {
		t.Errorf("Status code mismatch: got %v, want %v", gotStatus.Code(), wantStatus.Code())
		return
	}

	if gotStatus.Message() != wantStatus.Message() {
		t.Errorf("Error message mismatch: got %q, want %q", gotStatus.Message(), wantStatus.Message())
	}
}

func TestValidateNodeStageVolumeRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *csi.NodeStageVolumeRequest
		err  error
	}{
		{
			name: "Valid request",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				VolumeCapability:  &csi.VolumeCapability{},
			},
			err: nil,
		},
		{
			name: "Missing volume ID",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "",
				StagingTargetPath: "/mnt/staging",
				VolumeCapability:  &csi.VolumeCapability{},
			},
			err: errNoVolumeID,
		},
		{
			name: "Missing staging target path",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "",
				VolumeCapability:  &csi.VolumeCapability{},
			},
			err: errNoStagingTargetPath,
		},
		{
			name: "Missing volume capability",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				VolumeCapability:  nil,
			},
			err: errNoVolumeCapability,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateNodeStageVolumeRequest(tt.req)
			compareGRPCErrors(t, got, tt.err)
		})
	}
}

func Test_validateNodeUnstageVolumeRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *csi.NodeUnstageVolumeRequest
		err  error
	}{
		{
			name: "Valid request",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
			},
			err: nil,
		},
		{
			name: "Missing volume ID",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "",
				StagingTargetPath: "/mnt/staging",
			},
			err: errNoVolumeID,
		},
		{
			name: "Missing staging target path",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "",
			},
			err: errNoStagingTargetPath,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateNodeUnstageVolumeRequest(tt.req)
			compareGRPCErrors(t, got, tt.err)
		})
	}
}
func TestLinodeNodeServer_findDevicePath(t *testing.T) {
	tests := []struct {
		name           string
		key            common.LinodeVolumeKey
		expects        func(dUtils *mocks.MockDeviceUtils)
		wantDevicePath string
		wantErr        error
	}{
		{
			name: "Error - Couldn't verify Linode Volume is attached",
			key: common.LinodeVolumeKey{
				VolumeID: 123,
				Label:    "test",
			},
			expects: func(dUtils *mocks.MockDeviceUtils) {
				dUtils.EXPECT().GetDiskByIdPaths(gomock.Any(), gomock.Any()).Return([]string{"some/path"})
				dUtils.EXPECT().VerifyDevicePath(gomock.Any()).Return("", fmt.Errorf("no volume attached"))
			},
			wantDevicePath: "",
			wantErr:        status.Error(codes.Internal, "Error verifying Linode Volume (\"test\") is attached: no volume attached"),
		},
		{
			name: "Error - Couldn't get the devicepath to linode volume",
			key: common.LinodeVolumeKey{
				VolumeID: 123,
				Label:    "test",
			},
			expects: func(dUtils *mocks.MockDeviceUtils) {
				dUtils.EXPECT().GetDiskByIdPaths(gomock.Any(), gomock.Any()).Return([]string{"some/path"})
				dUtils.EXPECT().VerifyDevicePath(gomock.Any()).Return("", nil)
			},
			wantDevicePath: "",
			wantErr:        status.Error(codes.Internal, "Unable to find device path out of attempted paths: [some/path]"),
		},
		{
			name: "Success",
			key: common.LinodeVolumeKey{
				VolumeID: 123,
				Label:    "test",
			},
			expects: func(dUtils *mocks.MockDeviceUtils) {
				dUtils.EXPECT().GetDiskByIdPaths(gomock.Any(), gomock.Any()).Return([]string{"some/path", "/dev/test"})
				dUtils.EXPECT().VerifyDevicePath(gomock.Any()).Return("/dev/test", nil)
			},
			wantDevicePath: "/dev/test",
			wantErr:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Create gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock DeviceUtils
			mockDeviceUtils := mocks.NewMockDeviceUtils(ctrl)

			// Set expectations mock function call to deviceUtils if provided
			if tt.expects != nil {
				tt.expects(mockDeviceUtils)
			}

			// Create a new LinodeNodeServer with the mocked DeviceUtils
			// No need to set other fields as the function we are testing doesn't use them
			ns := &LinodeNodeServer{
				Driver:        nil,
				Mounter:       nil,
				DeviceUtils:   mockDeviceUtils,
				CloudProvider: nil,
				Metadata:      Metadata{},
			}

			// Call the function we are testing
			got, err := ns.findDevicePath(tt.key, "test")
			if err != nil {
				compareGRPCErrors(t, err, tt.wantErr)
			}
			if got != tt.wantDevicePath {
				t.Errorf("LinodeNodeServer.findDevicePath() = %v, want %v", got, tt.wantDevicePath)
			}
		})
	}
}

func TestLinodeNodeServer_ensureMountPoint(t *testing.T) {
	tests := []struct {
		name              string
		stagingTargetPath string
		mntExpects        func(m *mocks.MockMounter)
		fsExpects         func(m *mocks.MockFileSystem)
		want              bool
		wantErr           error
	}{
		{
			name:              "Success - Staging target path is a mount point (expect false)",
			stagingTargetPath: "/mnt/staging",
			mntExpects: func(m *mocks.MockMounter) {
				// Returning false because that means the target path is already a mount point
				m.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(false, nil)
			},
			fsExpects: func(m *mocks.MockFileSystem) {},
			want:      false,
			wantErr:   nil,
		},
		{
			name:              "Success -  Mount point didn't exist so we created a new mount point",
			stagingTargetPath: "/mnt/staging",
			mntExpects: func(m *mocks.MockMounter) {
				// Returning false because that means the target path is already a mount point
				m.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(false, fmt.Errorf("mount point doesn't exist"))
			},
			fsExpects: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
				m.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil)
			},
			want:    false,
			wantErr: status.Error(codes.Internal, "Failed to create directory (\"/mnt/staging\"): couldn't create directory"),
		},
		{
			name:              "Error -  mount point check fails and error is not IsNotExist",
			stagingTargetPath: "/mnt/staging",
			mntExpects: func(m *mocks.MockMounter) {
				// Returning false because that means the target path is already a mount point
				m.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(false, fmt.Errorf("some error"))
			},
			fsExpects: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(false)
			},
			want:    true,
			wantErr: status.Error(codes.Internal, "Unknown error when checking mount point (\"/mnt/staging\"): some error"),
		},
		{
			name:              "Error -  Mount point didn't exist and ran into error when create a new mount point or directory",
			stagingTargetPath: "/mnt/staging",
			mntExpects: func(m *mocks.MockMounter) {
				// Returning false because that means the target path is already a mount point
				m.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(false, fmt.Errorf("some error"))
			},
			fsExpects: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
				m.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(fmt.Errorf("couldn't create directory"))
			},
			want:    true,
			wantErr: status.Error(codes.Internal, "Failed to create directory (\"/mnt/staging\"): couldn't create directory"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMounter := mocks.NewMockMounter(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)

			if tt.mntExpects != nil {
				tt.mntExpects(mockMounter)
			}
			if tt.fsExpects != nil {
				tt.fsExpects(mockFileSystem)
			}

			ns := &LinodeNodeServer{
				Mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      nil,
				},
			}
			got, err := ns.ensureMountPoint(tt.stagingTargetPath, mockFileSystem)
			if err != nil {
				compareGRPCErrors(t, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("LinodeNodeServer.ensureMountPoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
