//go:build linux
// +build linux

package driver

import (
	"context"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/mocks"

	"go.uber.org/mock/gomock"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

func TestNodeServer_mountVolume_linux(t *testing.T) {
	var emptyStringArray []string
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
				m.EXPECT().MountSensitive("/dev/mapper/test", "", "ext4", []string{"defaults"}, emptyStringArray).Return(nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath("blkid").Return("/bin/blkid", nil)
				m.EXPECT().Command("blkid", "/dev/test").Return(c)
				c.EXPECT().Run().Return(nil)

				// LuksOpen
				m.EXPECT().LookPath("cryptsetup").Return("/bin/cryptsetup", nil)
				m.EXPECT().Command("cryptsetup", "--batch-mode", "luksOpen", "--key-file", "-", "/dev/test", "test").Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return([]byte("Command Successful"), nil)

				// Mount_linux: Check disk format. Disk is not formatted.
				m.EXPECT().Command("blkid", "-p", "-s", "TYPE", "-s", "PTTYPE", "-o", "export", "/dev/mapper/test").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte(""), exec.CodeExitError{Code: 2, Err: fmt.Errorf("not formatted")})

				// Mount_linux: Format disk
				m.EXPECT().Command("mkfs.ext4", "-F", "-m0", "/dev/mapper/test").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("Formatted successfully"), nil)
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
				m.EXPECT().LookPath("blkid").Return("/bin/blkid", nil)
				m.EXPECT().Command("blkid", "/dev/test").Return(c)
				c.EXPECT().Run().Return(nil)

				// LuksOpen
				m.EXPECT().LookPath("cryptsetup").Return("/bin/cryptsetup", nil)
				m.EXPECT().Command("cryptsetup", "--batch-mode", "luksOpen", "--key-file", "-", "/dev/test", "test").Return(c)
				c.EXPECT().SetStdin(gomock.Any())
				c.EXPECT().CombinedOutput().Return(nil, fmt.Errorf("Unable to LuksOpen"))
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
			if err := ns.mountVolume(context.Background(), tt.devicePath, tt.req); (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.mountVolume() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
