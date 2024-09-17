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

func TestNodeServer_mountVolume_luks(t *testing.T) {
	var emptyStringArray []string
	tests := []struct {
		name                   string
		devicePath             string
		req                    *csi.NodeStageVolumeRequest
		expectExecCalls        func(m *mocks.MockExecutor, c *mocks.MockCommand)
		expectFsCalls          func(m *mocks.MockFileSystem)
		expectMounterCalls     func(m *mocks.MockMounter)
		expectCryptDeviceCalls func(m *mocks.MockDevice)
		expectCryptSetUpCalls  func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice)
		wantErr                bool
	}{
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Dump().Return(1).AnyTimes()
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().KeyslotAddByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Load(gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().ActivateByPassphrase("test", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			},
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true).AnyTimes()
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil).AnyTimes()
			},
			wantErr: false,
		},
		{
			name:       "Success - already formatted",
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Dump().Return(0).AnyTimes()
			},
			expectFsCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().IsNotExist(gomock.Any()).Return(true).AnyTimes()
				m.EXPECT().Stat(gomock.Any()).Return(nil, nil).AnyTimes()
			},
			wantErr: false,
		},
		{
			name:       "Error - unable to initialize LUKS volume by path",
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(nil, fmt.Errorf("luks initializing failed")).AnyTimes()
			},
			wantErr: true,
		},
		{
			name:       "Error - unable to format LUKS volume",
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Dump().Return(1).AnyTimes()
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(fmt.Errorf("luks formatting failed")).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
			},
			wantErr: true,
		},
		{
			name:       "Error - unable to add keyslot to LUKS volume",
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Dump().Return(1).AnyTimes()
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().KeyslotAddByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("luks adding keyslot failed")).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
			},
			wantErr: true,
		},
		{
			name:       "Error - unable to load LUKS volume",
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Dump().Return(1).AnyTimes()
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().KeyslotAddByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Load(gomock.Any()).Return(fmt.Errorf("luks loading failed")).AnyTimes()
			},
			wantErr: true,
		},
		{
			name:       "Error - unable to activate LUKS volume",
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
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().Init(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Dump().Return(1).AnyTimes()
				m.EXPECT().Format(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().KeyslotAddByVolumeKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Load(gomock.Any()).Return(nil).AnyTimes()
				m.EXPECT().ActivateByPassphrase("test", gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("luks activating failed")).AnyTimes()
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
			mockCryptSetupClient := mocks.NewMockCryptSetupClient(ctrl)

			if tt.expectExecCalls != nil {
				tt.expectExecCalls(mockExec, mockCommand)
			}
			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem)
			}
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			if tt.expectCryptSetUpCalls != nil {
				tt.expectCryptSetUpCalls(mockCryptSetupClient, mockDevice)
			}
			if tt.expectCryptDeviceCalls != nil {
				tt.expectCryptDeviceCalls(mockDevice)
			}

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				encrypt: NewLuksEncryption(mockExec, mockFileSystem, mockCryptSetupClient),
			}
			if err := ns.mountVolume(context.Background(), tt.devicePath, tt.req); (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.mountVolume() mountvolume error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeServer_closeLuksMountSource(t *testing.T) {
	tests := []struct {
		name                   string
		expectCryptDeviceCalls func(m *mocks.MockDevice)
		expectCryptSetUpCalls  func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice)
		volumeID               string
		wantErr                bool
	}{
		{
			name: "Success - Able to close LUKS volume",
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().InitByName(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Deactivate(gomock.Any()).Return(nil).AnyTimes()
			},
			volumeID: "3232-pvc1234",
			wantErr:  false,
		},
		{
			name: "Success - If volume is not a LUKS volume",
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().InitByName(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Deactivate(gomock.Any()).Return(nil).AnyTimes()
			},
			volumeID: "3232-pvc1234",
			wantErr:  false,
		},
		{
			name: "Error - unable to initialize LUKS volume by name",
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().InitByName(gomock.Any()).Return(nil, fmt.Errorf("luks initializing failed")).AnyTimes()
			},
			wantErr: true,
		},
		{
			name: "Error - Unable to close LUKS volume",
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().InitByName(gomock.Any()).Return(md, nil).AnyTimes()
			},
			expectCryptDeviceCalls: func(m *mocks.MockDevice) {
				m.EXPECT().Free().Return(true).AnyTimes()
				m.EXPECT().Deactivate(gomock.Any()).Return(fmt.Errorf("failed to deactivate")).AnyTimes()
			},
			volumeID: "3232-pvc1234",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockMounter := mocks.NewMockMounter(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockDevice := mocks.NewMockDevice(ctrl)
			mockCryptSetupClient := mocks.NewMockCryptSetupClient(ctrl)

			if tt.expectCryptDeviceCalls != nil {
				tt.expectCryptDeviceCalls(mockDevice)
			}
			if tt.expectCryptSetUpCalls != nil {
				tt.expectCryptSetUpCalls(mockCryptSetupClient, mockDevice)
			}

			ns := &NodeServer{
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				encrypt: NewLuksEncryption(mockExec, mockFileSystem, mockCryptSetupClient),
			}
			if err := ns.closeLuksMountSource(context.Background(), tt.volumeID); (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.closeLuksMountSources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeServer_formatLUKSVolume(t *testing.T) {
	tests := []struct {
		name                   string
		expectFsCalls          func(m *mocks.MockFileSystem)
		expectExecCalls        func(m *mocks.MockExecutor, c *mocks.MockCommand)
		expectCryptDeviceCalls func(m *mocks.MockDevice)
		expectCryptSetUpCalls  func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice)
		devicePath             string
		luksContext            LuksContext
		want                   string
		wantErr                bool
	}{
		{
			name:          "Error - Encryption enabled. Volume not formatted. We will proceed with luks formatting and fail to validate.",
			expectFsCalls: func(m *mocks.MockFileSystem) {},
			expectExecCalls: func(m *mocks.MockExecutor, c *mocks.MockCommand) {
				m.EXPECT().LookPath(gomock.Any()).Return("/bin/test", nil).AnyTimes()
				m.EXPECT().Command(gomock.Any(), gomock.Any()).Return(c).AnyTimes()
				c.EXPECT().Run().Return(exec.CodeExitError{Code: 2, Err: fmt.Errorf("test")}).AnyTimes()
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

			// SetUp()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockCommand := mocks.NewMockCommand(ctrl)
			mockDevice := mocks.NewMockDevice(ctrl)
			mockCryptSetupClient := mocks.NewMockCryptSetupClient(ctrl)

			if tt.expectCryptDeviceCalls != nil {
				tt.expectCryptDeviceCalls(mockDevice)
			}
			if tt.expectCryptSetUpCalls != nil {
				tt.expectCryptSetUpCalls(mockCryptSetupClient, mockDevice)
			}
			if tt.expectFsCalls != nil {
				tt.expectFsCalls(mockFileSystem)
			}
			if tt.expectExecCalls != nil {
				tt.expectExecCalls(mockExec, mockCommand)
			}

			ns := &NodeServer{
				encrypt: NewLuksEncryption(mockExec, mockFileSystem, mockCryptSetupClient),
			}

			got, err := ns.formatLUKSVolume(context.Background(), tt.devicePath, tt.luksContext)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeServer.formatLUKSVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NodeServer.formatLUKSVolume() = %v, want %v", got, tt.want)
			}

			// TearDown()
		})
	}
}
