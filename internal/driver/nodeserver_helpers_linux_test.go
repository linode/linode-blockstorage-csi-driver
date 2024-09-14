//go:build linux
// +build linux

package driver

import (
	"context"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/mocks"

	osexec "os/exec"

	"go.uber.org/mock/gomock"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

func setup() {
	osexec.Command("/bin/dd", "if=/dev/urandom", "of=/tmp/test", "bs=64M", "count=1").Run()
}

func teardown() {
	osexec.Command("rm -rf", "/tmp/test").Run()
}

func TestNodeServer_mountVolume_linux(t *testing.T) {
	var emptyStringArray []string
	tests := []struct {
		name                  string
		devicePath            string
		req                   *csi.NodeStageVolumeRequest
		expectExecCalls       func(m *mocks.MockExecutor, c *mocks.MockCommand)
		expectFsCalls         func(m *mocks.MockFileSystem)
		expectMounterCalls    func(m *mocks.MockMounter)
		expectCryptSetupCalls func(m *mocks.MockDevice)
		wantErr               bool
	}{
		{
			name:       "Success - Mount the volume",
			devicePath: "/dev/test",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "test",
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().MountSensitive("/dev/test", "", "ext4", []string{"defaults"}, emptyStringArray).Return(nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				// Mount_linux: Check disk format. Disk is not formatted.
				m.EXPECT().Command("blkid", "-p", "-s", "TYPE", "-s", "PTTYPE", "-o", "export", "/dev/test").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte(""), exec.CodeExitError{Code: 2, Err: fmt.Errorf("not formatted")})

				// Mount_linux: Format disk
				m.EXPECT().Command("mkfs.ext4", "-F", "-m0", "/dev/test").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("Formatted successfully"), nil)
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
				m.EXPECT().MountSensitive("/dev/test", "", "ext4", []string{"defaults"}, emptyStringArray).Return(fmt.Errorf("Couldn't mount."))
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				// Mount_linux: Check disk format. Disk is not formatted.
				m.EXPECT().Command("blkid", "-p", "-s", "TYPE", "-s", "PTTYPE", "-o", "export", "/dev/test").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte(""), exec.CodeExitError{Code: 2, Err: fmt.Errorf("not formatted")})

				// Mount_linux: Format disk
				m.EXPECT().Command("mkfs.ext4", "-F", "-m0", "/dev/test").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("Formatted successfully"), nil)
			},
			wantErr: true,
		},
		{
			name:       "Success - mount LUKS volume",
			devicePath: "/tmp/test",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "test",
				VolumeContext: map[string]string{
					LuksEncryptedAttribute: "true",
					LuksCipherAttribute:    "aes-xts-plain64",
					LuksKeySizeAttribute:   "512",
					PublishInfoVolumeName:  "test",
				},
				Secrets: map[string]string{LuksKeyAttribute: "test"},
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().MountSensitive("/dev/mapper/test", "", "ext4", []string{"defaults"}, emptyStringArray).Return(nil).AnyTimes()
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				// Mount_linux: Format disk
				m.EXPECT().Command(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("TYPE=ext4"), nil).AnyTimes()
				m.EXPECT().Command(gomock.Any(), gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("TYPE=ext4"), nil).AnyTimes()
			},
			expectCryptSetupCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().KeyslotAddByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Load(gomock.Any()).Return(nil).AnyTimes()
			},
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true).AnyTimes()
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil).AnyTimes()
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
					LuksCipherAttribute:    "aes-xts-plain64",
					LuksKeySizeAttribute:   "512",
					PublishInfoVolumeName:  "test",
				},
				Secrets: map[string]string{LuksKeyAttribute: "test"},
			},
			// expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
			// 	// Mount_linux: Format disk
			// 	m.EXPECT().Command(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(c)
			// 	c.EXPECT().CombinedOutput().Return([]byte("TYPE=ext4"), nil).AnyTimes()
			// 	m.EXPECT().Command(gomock.Any(), gomock.Any(), gomock.Any()).Return(c)
			// 	c.EXPECT().CombinedOutput().Return([]byte("TYPE=ext4"), nil).AnyTimes()
			// },
			expectCryptSetupCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(fmt.Errorf("luks formatting failed")).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
			},
			// expectFsCalls: func(m *mocks.MockFileSystem) {
			// 	m.EXPECT().IsNotExist(gomock.Any()).Return(true)
			// 	m.EXPECT().Stat(gomock.Any()).Return(nil, nil)
			// },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMounter := mocks.NewMockMounter(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCommand := mocks.NewMockCommand(ctrl)
			mockDevice := mocks.NewMockDevice(ctrl)

			if tt.expectExecCalls != nil {
				tt.expectExecCalls(mockExec, mockCommand)
			}
			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem)
			}
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			if tt.expectCryptSetupCalls != nil {
				tt.expectCryptSetupCalls(mockDevice)
			}

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				encrypt: NewLuksEncryption(mockExec, mockFileSystem),
			}
			if err := ns.mountVolume(context.Background(), tt.devicePath, tt.req); (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.mountVolume() error = %v, wantErr %v", err, tt.wantErr)
			}

			teardown()
		})
	}
}
