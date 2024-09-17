package driver

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
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
			got := validateNodeStageVolumeRequest(context.Background(), tt.req)
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
			got := validateNodeUnstageVolumeRequest(context.Background(), tt.req)
			compareGRPCErrors(t, got, tt.err)
		})
	}
}

func Test_getFSTypeAndMountOptions(t *testing.T) {
	tests := []struct {
		name             string
		volumeCapability *csi.VolumeCapability
		wantFsType       string
		wantMountOptions []string
	}{
		{
			name:             "Valid request - no volume capability set",
			volumeCapability: nil,
			wantFsType:       "ext4",
			wantMountOptions: *new([]string),
		},
		{
			name: "Valid request - volume capability set",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "ext4",
						MountFlags: []string{
							"noatime",
						},
					},
				},
			},
			wantFsType: "ext4",
			wantMountOptions: []string{
				"noatime",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsType, mountOptions := getFSTypeAndMountOptions(context.Background(), tt.volumeCapability)
			if fsType != tt.wantFsType {
				t.Errorf("getFSTypeAndMountOptions() fsType = %v, want %v", fsType, tt.wantFsType)
			}
			if !reflect.DeepEqual(mountOptions, tt.wantMountOptions) {
				t.Errorf("getFSTypeAndMountOptions() mountOptions = %v, want %v", mountOptions, tt.wantMountOptions)
			}
		})
	}
}

func TestNodeServer_findDevicePath(t *testing.T) {
	tests := []struct {
		name           string
		key            linodevolumes.LinodeVolumeKey
		expects        func(dUtils *mocks.MockDeviceUtils)
		wantDevicePath string
		wantErr        error
	}{
		{
			name: "Error - Couldn't verify Linode Volume is attached",
			key: linodevolumes.LinodeVolumeKey{
				VolumeID: 123,
				Label:    "test",
			},
			expects: func(dUtils *mocks.MockDeviceUtils) {
				dUtils.EXPECT().GetDiskByIdPaths(gomock.Any(), gomock.Any()).Return([]string{"some/path"})
				dUtils.EXPECT().VerifyDevicePath(gomock.Any()).Return("", fmt.Errorf("no volume attached"))
			},
			wantDevicePath: "",
			wantErr:        errInternal("Error verifying Linode Volume (\"test\") is attached: no volume attached"),
		},
		{
			name: "Error - Couldn't get the devicepath to linode volume",
			key: linodevolumes.LinodeVolumeKey{
				VolumeID: 123,
				Label:    "test",
			},
			expects: func(dUtils *mocks.MockDeviceUtils) {
				dUtils.EXPECT().GetDiskByIdPaths(gomock.Any(), gomock.Any()).Return([]string{"some/path"})
				dUtils.EXPECT().VerifyDevicePath(gomock.Any()).Return("", nil)
			},
			wantDevicePath: "",
			wantErr:        errInternal("Unable to find device path out of attempted paths: [some/path]"),
		},
		{
			name: "Success",
			key: linodevolumes.LinodeVolumeKey{
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

			// Create a new NodeServer with the mocked DeviceUtils
			// No need to set other fields as the function we are testing doesn't use them
			ns := &NodeServer{
				driver:      nil,
				mounter:     nil,
				deviceutils: mockDeviceUtils,
				client:      nil,
				metadata:    Metadata{},
			}

			// Call the function we are testing
			got, err := ns.findDevicePath(context.Background(), tt.key, "test")
			if err != nil {
				compareGRPCErrors(t, err, tt.wantErr)
			}
			if got != tt.wantDevicePath {
				t.Errorf("NodeServer.findDevicePath() = %v, want %v", got, tt.wantDevicePath)
			}
		})
	}
}

func TestNodeServer_ensureMountPoint(t *testing.T) {
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
			wantErr: errInternal("Failed to create directory (/mnt/staging): couldn't create directory"),
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
			wantErr: errInternal("Unknown error when checking mount point (\"/mnt/staging\"): some error"),
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
			wantErr: errInternal("Failed to create directory (\"/mnt/staging\"): couldn't create directory"),
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

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      nil,
				},
			}
			got, err := ns.ensureMountPoint(context.Background(), tt.stagingTargetPath, mockFileSystem)
			if err != nil {
				compareGRPCErrors(t, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("NodeServer.ensureMountPoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateNodeExpandVolumeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *csi.NodeExpandVolumeRequest
		wantErr bool
	}{
		{
			name: "Valid request",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "vol-123",
				VolumePath: "/mnt/staging",
			},
			wantErr: false,
		},
		{
			name: "Missing volume ID",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "",
				VolumePath: "/mnt/staging",
			},
			wantErr: true,
		},
		{
			name: "Missing staging target path",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "vol-123",
				VolumePath: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodeExpandVolumeRequest(context.Background(), tt.req); (err != nil) != tt.wantErr {
				t.Errorf("validateNodeExpandVolumeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateNodeUnpublishVolumeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *csi.NodeUnpublishVolumeRequest
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "Valid request",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "vol-123",
				TargetPath: "/mnt/staging",
			},
			wantErr: false,
		},
		{
			name: "Missing volume ID",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "",
				TargetPath: "/mnt/staging",
			},
			wantErr: true,
		},
		{
			name: "Missing staging target path",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "vol-123",
				TargetPath: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodeUnpublishVolumeRequest(context.Background(), tt.req); (err != nil) != tt.wantErr {
				t.Errorf("validateNodeUnpublishVolumeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateNodePublishVolumeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *csi.NodePublishVolumeRequest
		wantErr bool
	}{
		{
			name: "Valid request",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{},
				},
			},
			wantErr: false,
		},
		{
			name: "Missing volume ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{},
				},
			},
			wantErr: true,
		},
		{
			name: "Missing staging target path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "",
				TargetPath:        "/mnt/target",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{},
				},
			},
			wantErr: true,
		},
		{
			name: "Missing target path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{},
				},
			},
			wantErr: true,
		},
		{
			name: "Missing volume capability",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				VolumeCapability:  nil,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodePublishVolumeRequest(context.Background(), tt.req); (err != nil) != tt.wantErr {
				t.Errorf("validateNodePublishVolumeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeServer_nodePublishVolumeBlock(t *testing.T) {
	tests := []struct {
		name               string
		req                *csi.NodePublishVolumeRequest
		mountOptions       []string
		expectFsCalls      func(m *mocks.MockFileSystem, f *mocks.MockFileInterface)
		expectMounterCalls func(m *mocks.MockMounter)
		expectFileCalls    func(m *mocks.MockFileInterface)
		want               *csi.NodePublishVolumeResponse
		wantErr            bool
	}{
		{
			name: "Valid request",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions: []string{"bind"},
			expectFsCalls: func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(f, nil)
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount("/dev/sda", "/mnt/target", "", []string{"bind"}).Return(nil)
			},
			expectFileCalls: func(m *mocks.MockFileInterface) {
				m.EXPECT().Close().Return(nil)
			},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name: "Error - devicePath missing",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:       []string{"bind"},
			expectFsCalls:      nil,
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:               nil,
			wantErr:            true,
		},
		{
			name: "Error - unable to create targetPathDir",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions: []string{"bind"},
			expectFsCalls: func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(fmt.Errorf("unable to create targetPathDir..."))
			},
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:               nil,
			wantErr:            true,
		},
		{
			name: "Error - unable to create file at targetPath",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions: []string{"bind"},
			expectFsCalls: func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(nil, fmt.Errorf("unable to create file..."))
				m.EXPECT().Remove("/mnt/target").Return(nil)
			},
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:               nil,
			wantErr:            true,
		},
		{
			name: "Error - unable to create file at targetPath and remove targetPath fails",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions: []string{"bind"},
			expectFsCalls: func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(nil, fmt.Errorf("unable to create file..."))
				m.EXPECT().Remove("/mnt/target").Return(fmt.Errorf("unable to remove %s...", "/mnt/target"))
			},
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:               nil,
			wantErr:            true,
		},
		{
			name: "Error - unable to mount the block device to targetPath",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions: []string{"bind"},
			expectFsCalls: func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(f, nil)
				m.EXPECT().Remove("/mnt/target").Return(nil)
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount("/dev/sda", "/mnt/target", "", []string{"bind"}).Return(fmt.Errorf("unable to mount..."))
			},
			expectFileCalls: func(f *mocks.MockFileInterface) {
				f.EXPECT().Close().Return(nil)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Error - unable to mount the block device to targetPath and remove targetPath fails",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions: []string{"bind"},
			expectFsCalls: func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(f, nil)
				m.EXPECT().Remove("/mnt/target").Return(fmt.Errorf("unable to remove %s...", "/mnt/target"))
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount("/dev/sda", "/mnt/target", "", []string{"bind"}).Return(fmt.Errorf("unable to mount the block device at %s...", "/mnt/target"))
			},
			expectFileCalls: func(f *mocks.MockFileInterface) {
				f.EXPECT().Close().Return(nil)
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMounter := mocks.NewMockMounter(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockFileInterface := mocks.NewMockFileInterface(ctrl)

			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem, mockFileInterface)
			}
			if tt.expectFileCalls != nil {
				tt.expectFileCalls(mockFileInterface)
			}
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      nil,
				},
			}

			got, err := ns.nodePublishVolumeBlock(context.Background(), tt.req, tt.mountOptions, mockFileSystem)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.nodePublishVolumeBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeServer.nodePublishVolumeBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}
