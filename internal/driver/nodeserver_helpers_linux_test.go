//go:build linux
// +build linux

package driver

import (
	"context"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/mock/gomock"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

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
			devicePath: "/tmp/test_success_noluks",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "123-test_success_noluks",
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().MountSensitive("/tmp/test_success_noluks", "", "ext4", []string{"defaults"}, emptyStringArray).Return(nil)
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				// Mount_linux: Check disk format. Disk is not formatted.
				m.EXPECT().Command(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte(""), exec.CodeExitError{Code: 2, Err: fmt.Errorf("not formatted")})

				// Mount_linux: Format disk
				m.EXPECT().Command("mkfs.ext4", "-F", "-m0", "/tmp/test_success_noluks").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("Formatted successfully"), nil)
			},
			wantErr: false,
		},
		{
			name:       "Error - Unable to mount the volume",
			devicePath: "/tmp/test_error_noluks",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "123-test_error_noluks",
			},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().MountSensitive("/tmp/test_error_noluks", "", "ext4", []string{"defaults"}, emptyStringArray).Return(fmt.Errorf("Couldn't mount."))
			},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				// Mount_linux: Check disk format. Disk is not formatted.
				m.EXPECT().Command(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(c)
				c.EXPECT().CombinedOutput().Return([]byte(""), exec.CodeExitError{Code: 2, Err: fmt.Errorf("not formatted")})

				// Mount_linux: Format disk
				m.EXPECT().Command("mkfs.ext4", "-F", "-m0", "/tmp/test_error_noluks").Return(c)
				c.EXPECT().CombinedOutput().Return([]byte("Formatted successfully"), nil)
			},
			wantErr: true,
		},
		{
			name:       "Error - No volume ID",
			devicePath: "/tmp/test_error_noluks",
			req: &csi.NodeStageVolumeRequest{
				VolumeId: "",
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
			mockDevice := mocks.NewMockDevice(ctrl)
			mockCryptSetup := mocks.NewMockCryptSetupClient(ctrl)

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

			sfm := mount.SafeFormatAndMount{
				Interface: mockMounter,
				Exec:      mockExec,
			}
			ns := &NodeServer{
				mounter: &mountmanager.SafeFormatAndMount{
					SafeFormatAndMount: &sfm,
					Formater:           &sfm,
				},
				encrypt: NewLuksEncryption(mockExec, mockFileSystem, mockCryptSetup),
			}
			if err := ns.mountVolume(context.Background(), tt.devicePath, tt.req); (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.mountVolume() mountvolume error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
