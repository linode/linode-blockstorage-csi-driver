package driver

import (
	"fmt"
	"os"
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
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
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
			fsType, mountOptions := getFSTypeAndMountOptions(tt.volumeCapability)
			if fsType != tt.wantFsType {
				t.Errorf("getFSTypeAndMountOptions() fsType = %v, want %v", fsType, tt.wantFsType)
			}
			if !reflect.DeepEqual(mountOptions, tt.wantMountOptions) {
				t.Errorf("getFSTypeAndMountOptions() mountOptions = %v, want %v", mountOptions, tt.wantMountOptions)
			}
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
			ns := &NodeServer{
				driver:        nil,
				mounter:       nil,
				deviceutils:   mockDeviceUtils,
				client: nil,
				metadata:      Metadata{},
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

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
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

func TestLinodeNodeServer_prepareLUKSVolume(t *testing.T) {
	tests := []struct {
		name            string
		expectFsCalls   func(m *mocks.MockFileSystem)
		expectExecCalls func(m *mocks.MockExecutor, c *mocks.MockCommand)
		devicePath      string
		luksContext     LuksContext
		want            string
		wantErr         bool
	}{
		{
			name: "Success - Encryption enabled. Luks volume is not open",
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)
			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
			},
			want:    "/dev/mapper/test",
			wantErr: false,
		},
		{
			name:          "Error - Encryption enabled. Volume not formatted. We will proceed with luks formatting and fail to validate.",
			expectFsCalls: func(m *mocks.MockFileSystem) {},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(exec.CodeExitError{Code: 2, Err: fmt.Errorf("test")})
			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Error - Encryption enabled. Volume not formatted. We will proceed with luks formatting and fail to format with LUKS.",
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(exec.CodeExitError{Code: 2, Err: fmt.Errorf("test")})

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("failed test"))

			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
				VolumeLifecycle:   VolumeLifecycleNodeStageVolume,
				EncryptionCipher:  "aes-xts-plain64",
				EncryptionKeySize: "256",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Error - Encryption enabled. Volume not formatted. We will proceed with luks formatting and fail to mount.",
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)

				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(exec.CodeExitError{Code: 2, Err: fmt.Errorf("test")})

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("failed test"))

			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
				VolumeLifecycle:   VolumeLifecycleNodeStageVolume,
				EncryptionCipher:  "aes-xts-plain64",
				EncryptionKeySize: "256",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Success - Encryption enabled. Volume not formatted. We will proceed with luks formatting and mount.",
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)

				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(exec.CodeExitError{Code: 2, Err: fmt.Errorf("test")})

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)

			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
				VolumeLifecycle:   VolumeLifecycleNodeStageVolume,
				EncryptionCipher:  "aes-xts-plain64",
				EncryptionKeySize: "256",
			},
			want:    "/dev/mapper/test",
			wantErr: false,
		},
		{
			name: "Error - Encryption enabled. Cryptsetup is not installed",
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(nil)
				m.EXPECT().LookPath(gomock.Any()).Return("", osexec.ErrNotFound)
			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Success - Encryption enabled. Luks volume is open",
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(false)
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(nil)
			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
			},
			want:    "/dev/mapper/test",
			wantErr: false,
		},
		{
			name:          "Error - Failed to validate blkid (executable invalid)",
			expectFsCalls: func(m *mocks.MockFileSystem) {},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("", osexec.ErrNotFound)
			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
			},
			want:    "",
			wantErr: true,
		},
		{
			name:          "Error - Failed to validate blkid (checking blkdid failed)",
			expectFsCalls: func(m *mocks.MockFileSystem) {},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(exec.CodeExitError{Err: fmt.Errorf("Couldn't run command")})
			},
			devicePath: "/dev/test",
			luksContext: LuksContext{
				EncryptionEnabled: true,
				EncryptionKey:     "test",
				VolumeName:        "test",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCommand := mocks.NewMockCommand(ctrl)

			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem)
			}
			if tt.expectExecCalls != nil {
				tt.expectExecCalls(mockExec, mockCommand)
			}

			ns := &NodeServer{
				encrypt: NewLuksEncryption(mockExec, mockFileSystem),
			}

			got, err := ns.prepareLUKSVolume(tt.devicePath, tt.luksContext)
			if (err != nil) != tt.wantErr {
				t.Errorf("LinodeNodeServer.prepareLUKSVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("LinodeNodeServer.prepareLUKSVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinodeNodeServer_mountVolume(t *testing.T) {
	tests := []struct {
		name               string
		devicePath         string
		req                *csi.NodeStageVolumeRequest
		expectExecCalls    func(m *mocks.MockExecutor, c *mocks.MockCommand)
		expectFsCalls      func(m *mocks.MockFileSystem)
		expectMounterCalls func(m *mocks.MockMounter)
		wantErr            bool
	}{
		{
			name:       "Success - Mount the volume",
			devicePath: "/dev/test",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "test",
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr: false,
		},
		{
			name:       "Error - Unable to mount the volume",
			devicePath: "/dev/test",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "test",
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("Unable to mount the volume"))
			},
			wantErr: true,
		},
		{
			name:       "Success - mount LUKS volume",
			devicePath: "/dev/test",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "test",
				VolumeContext: map[string]string{
					LuksEncryptedAttribute: "true",
					LuksCipherAttribute:    "test",
					LuksKeySizeAttribute:   "test",
					PublishInfoVolumeName:  "test",
				},
				Secrets: map[string]string{LuksKeyAttribute: "test"},
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)
			},
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
			},
			wantErr: false,
		},
		{
			name:       "Error - unable to prepare LUKS volume",
			devicePath: "/dev/test",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "test",
				VolumeContext: map[string]string{
					LuksEncryptedAttribute: "true",
					LuksCipherAttribute:    "test",
					LuksKeySizeAttribute:   "test",
					PublishInfoVolumeName:  "test",
				},
				Secrets: map[string]string{LuksKeyAttribute: "test"},
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().Run().Return(nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("Unable to mount LUKS volume"))
			},
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true)
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Skip this test on Linux. Linux supported alternative test case can be found in nodeserver_helpers_linux_test.go
			if runtime.GOOS == "linux" {
				t.Skipf("Skipping test on Linux")
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMounter := mocks.NewMockMounter(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCommand := mocks.NewMockCommand(ctrl)

			if tt.expectExecCalls != nil {
				tt.expectExecCalls(mockExec, mockCommand)
			}
			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem)
			}
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				encrypt: NewLuksEncryption(mockExec, mockFileSystem),
			}
			if err := ns.mountVolume(tt.devicePath, tt.req); (err != nil) != tt.wantErr {
				t.Errorf("LinodeNodeServer.mountVolume() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLinodeNodeServer_closeLuksMountSources(t *testing.T) {
	tests := []struct {
		name               string
		expectMounterCalls func(m *mocks.MockMounter)
		expectExecCalls    func(m *mocks.MockExecutor, c *mocks.MockCommand)
		expectFsCalls      func(m *mocks.MockFileSystem)
		path               string
		wantErr            bool
	}{
		{
			name: "Success - close mount source",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)
			},
			path:    "/test",
			wantErr: false,
		},
		{
			name: "Error - unable to close mount source. Executable not found",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("", osexec.ErrNotFound)
			},
			path:    "/test",
			wantErr: true,
		},
		{
			name: "Error - unable to close mount source. unexpected error",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("", fmt.Errorf("Unexpected error"))
			},
			path:    "/test",
			wantErr: true,
		},
		{
			name: "Error - unable to close mount source. Command to check mounts failed",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("test"), fmt.Errorf("Unexpected error"))
			},
			path:    "/test",
			wantErr: true,
		},
		{
			name: "Success - close LUKS mount source",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("/dev/mapper/test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("type: luks"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("test"), nil)
			},
			path:    "/dev/mapper/test",
			wantErr: false,
		},
		{
			name: "Error - failed to close LUKS mount source",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("/dev/mapper/test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("type: luks"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("Unexpected error"))
			},
			path:    "/dev/mapper/test",
			wantErr: true,
		},
		{
			name: "Error - failed to determine if mount is a luks mapping",
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil)
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("/dev/mapper/test"), nil)

				m.EXPECT().LookPath(gomock.Any()).Return("", osexec.ErrNotFound)
			},
			path:    "/dev/mapper/test",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMounter := mocks.NewMockMounter(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCommand := mocks.NewMockCommand(ctrl)

			if tt.expectExecCalls != nil {
				tt.expectExecCalls(mockExec, mockCommand)
			}
			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem)
			}
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				encrypt: NewLuksEncryption(mockExec, mockFileSystem),
			}
			if err := ns.closeLuksMountSources(tt.path); (err != nil) != tt.wantErr {
				t.Errorf("LinodeNodeServer.closeLuksMountSources() error = %v, wantErr %v", err, tt.wantErr)
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
			if err := validateNodeExpandVolumeRequest(tt.req); (err != nil) != tt.wantErr {
				t.Errorf("validateNodeExpandVolumeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateNodeUnpublishVolumeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req *csi.NodeUnpublishVolumeRequest
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "Valid request",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:          "vol-123",
				TargetPath:        "/mnt/staging",
			},
			wantErr: false,
		},
		{
			name: "Missing volume ID",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:          "",
				TargetPath:        "/mnt/staging",
			},
			wantErr: true,
		},
		{
			name: "Missing staging target path",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:          "vol-123",
				TargetPath:        "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodeUnpublishVolumeRequest(tt.req); (err != nil) != tt.wantErr {
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
			if err := validateNodePublishVolumeRequest(tt.req); (err != nil) != tt.wantErr {
				t.Errorf("validateNodePublishVolumeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLinodeNodeServer_nodePublishVolumeBlock(t *testing.T) {
	tests := []struct {
		name    string
		req          *csi.NodePublishVolumeRequest
		mountOptions []string
		expectFsCalls     func(m *mocks.MockFileSystem, f *mocks.MockFileInterface)
		expectMounterCalls func(m *mocks.MockMounter)
		expectFileCalls    func(m *mocks.MockFileInterface)
		want    *csi.NodePublishVolumeResponse
		wantErr bool
	}{
		{
			name: "Valid request",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     func(m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(f, nil)
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().Mount("/dev/sda", "/mnt/target", "", []string{"bind"}).Return(nil)
			},
			expectFileCalls:    func(m *mocks.MockFileInterface) {
				m.EXPECT().Close().Return(nil)
			},
			want:              &csi.NodePublishVolumeResponse{},
			wantErr:           false,
		},
		{
			name: "Error - devicePath missing",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     nil,
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:              nil,
			wantErr:           true,
		},
		{
			name: "Error - unable to create targetPathDir",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     func (m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(fmt.Errorf("unable to create targetPathDir..."))
			},
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:              nil,
			wantErr:           true,
		},
		{
			name: "Error - unable to create file at targetPath",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     func (m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(nil, fmt.Errorf("unable to create file..."))
				m.EXPECT().Remove("/mnt/target").Return(nil)
			},
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:              nil,
			wantErr:           true,
		},
		{
			name: "Error - unable to create file at targetPath and remove targetPath fails",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     func (m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(nil, fmt.Errorf("unable to create file..."))
				m.EXPECT().Remove("/mnt/target").Return(fmt.Errorf("unable to remove %s...", "/mnt/target"))
			},
			expectMounterCalls: nil,
			expectFileCalls:    nil,
			want:              nil,
			wantErr:           true,
		},
		{
			name: "Error - unable to mount the block device to targetPath",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     func (m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(f, nil)
				m.EXPECT().Remove("/mnt/target").Return(nil)
			},
			expectMounterCalls: func (m *mocks.MockMounter) {
				m.EXPECT().Mount("/dev/sda", "/mnt/target", "", []string{"bind"}).Return(fmt.Errorf("unable to mount..."))
			},
			expectFileCalls:    func (f *mocks.MockFileInterface) {
				f.EXPECT().Close().Return(nil)
			},
			want:              nil,
			wantErr:           true,
		},
		{
			name: "Error - unable to mount the block device to targetPath and remove targetPath fails",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				StagingTargetPath: "/mnt/staging",
				TargetPath:        "/mnt/target",
				PublishContext:    map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			mountOptions:      []string{"bind"},
			expectFsCalls:     func (m *mocks.MockFileSystem, f *mocks.MockFileInterface) {
				m.EXPECT().MkdirAll("/mnt", rwPermission).Return(nil)
				m.EXPECT().OpenFile("/mnt/target", os.O_CREATE, ownerGroupReadWritePermissions).Return(f, nil)
				m.EXPECT().Remove("/mnt/target").Return(fmt.Errorf("unable to remove %s...", "/mnt/target"))
			},
			expectMounterCalls: func (m *mocks.MockMounter) {
				m.EXPECT().Mount("/dev/sda", "/mnt/target", "", []string{"bind"}).Return(fmt.Errorf("unable to mount the block device at %s...", "/mnt/target"))
			},
			expectFileCalls:    func (f *mocks.MockFileInterface) {
				f.EXPECT().Close().Return(nil)
			},
			want:              nil,
			wantErr:           true,
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

			got, err := ns.nodePublishVolumeBlock(tt.req, tt.mountOptions, mockFileSystem)
			if (err != nil) != tt.wantErr {
				t.Errorf("LinodeNodeServer.nodePublishVolumeBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LinodeNodeServer.nodePublishVolumeBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}
