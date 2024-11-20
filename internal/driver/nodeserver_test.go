package driver

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"go.uber.org/mock/gomock"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	devicemanager "github.com/linode/linode-blockstorage-csi-driver/pkg/device-manager"
	filesystem "github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
)

func TestNodePublishVolume(t *testing.T) {
	tests := []struct {
		name               string
		req                *csi.NodePublishVolumeRequest
		resp               *csi.NodePublishVolumeResponse
		expectMounterCalls func(m *mocks.MockMounter)
		expectedError      error
	}{
		{
			name: "publishhappypath",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "vol-123",
				TargetPath:        "/mnt/target",
				StagingTargetPath: "/mnt/staging",
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			resp: &csi.NodePublishVolumeResponse{},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(false, nil)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockMounter := mocks.NewMockMounter(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			ns := &NodeServer{
				driver: &LinodeDriver{},
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
			}
			returnedResp, err := ns.NodePublishVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodePublishVolume error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodePublishVolume() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	tests := []struct {
		name                  string
		req                   *csi.NodeUnpublishVolumeRequest
		resp                  *csi.NodeUnpublishVolumeResponse
		expectMounterCalls    func(m *mocks.MockMounter)
		expectCryptSetUpCalls func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice)
		expectedError         error
	}{
		{
			name: "unpublishhappypath",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "vol-123",
				TargetPath: "/mnt/target",
			},
			resp:               &csi.NodeUnpublishVolumeResponse{},
			expectMounterCalls: func(m *mocks.MockMounter) {},
			expectedError:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockMounter := mocks.NewMockMounter(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			ns := &NodeServer{
				driver: &LinodeDriver{},
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
			}
			returnedResp, err := ns.NodeUnpublishVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodeUnpublishVolume error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodeUnpublishVolume() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNodeStageVolume(t *testing.T) {
	tests := []struct {
		name               string
		req                *csi.NodeStageVolumeRequest
		resp               *csi.NodeStageVolumeResponse
		expectMounterCalls func(m *mocks.MockMounter)
		expectFSCalls      func(m *mocks.MockFileSystem)
		expectedError      error
	}{
		{
			name: "stagehappypath",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "1000-stagehappypath",
				StagingTargetPath: "/mnt/staging",
				PublishContext: map[string]string{
					"devicePath": "/dev/stagehappypath",
				},
				VolumeCapability: &csi.VolumeCapability{},
			},
			resp: &csi.NodeStageVolumeResponse{},
			expectMounterCalls: func(m *mocks.MockMounter) {
				m.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(false, nil).Times(2)
			},
			expectedError: nil,
			expectFSCalls: func(m *mocks.MockFileSystem) {
				m.EXPECT().Glob("/dev/sd*").Return([]string{"/dev/sda", "/dev/sdb"}, nil).AnyTimes()
				m.EXPECT().Stat("/dev/disk/by-id/linode-stagehappypath").Return(nil, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockMounter := mocks.NewMockMounter(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockFS := mocks.NewMockFileSystem(ctrl)
			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			if tt.expectFSCalls != nil {
				tt.expectFSCalls(mockFS)
			}
			ns := &NodeServer{
				driver: &LinodeDriver{},
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				deviceutils: devicemanager.NewDeviceUtils(mockFS, mockExec),
			}
			ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
				VolumeId:          "1000-stagehappypath",
				TargetPath:        "/dev/stagehappypath",
				StagingTargetPath: "/mnt/staging",
				PublishContext: map[string]string{
					"devicePath": "/dev/stagehappypath",
				},
				VolumeCapability: &csi.VolumeCapability{},
			})
			returnedResp, err := ns.NodeStageVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodeStageVolume error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodeStageVolume() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	tests := []struct {
		name                   string
		req                    *csi.NodeUnstageVolumeRequest
		resp                   *csi.NodeUnstageVolumeResponse
		expectMounterCalls     func(m *mocks.MockMounter)
		expectFSCalls          func(m *mocks.MockFileSystem)
		expectCryptDeviceCalls func(m *mocks.MockDevice)
		expectCryptSetUpCalls  func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice)
		expectedError          error
	}{
		{
			name: "unstagehappypath",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "1001-volkey",
				StagingTargetPath: "/mnt/staging",
			},
			resp:               &csi.NodeUnstageVolumeResponse{},
			expectMounterCalls: func(m *mocks.MockMounter) {},
			expectFSCalls:      func(m *mocks.MockFileSystem) {},
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().InitByName(gomock.Any()).Return(nil, fmt.Errorf("some error")).AnyTimes()
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockMounter := mocks.NewMockMounter(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockDevice := mocks.NewMockDevice(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockCryptSetupClient := mocks.NewMockCryptSetupClient(ctrl)

			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			if tt.expectFSCalls != nil {
				tt.expectFSCalls(mockFileSystem)
			}
			if tt.expectCryptSetUpCalls != nil {
				tt.expectCryptSetUpCalls(mockCryptSetupClient, mockDevice)
			}
			ns := &NodeServer{
				driver: &LinodeDriver{},
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				deviceutils: devicemanager.NewDeviceUtils(mockFileSystem, mockExec),
				encrypt:     NewLuksEncryption(mockExec, mockFileSystem, mockCryptSetupClient),
			}
			returnedResp, err := ns.NodeUnstageVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodeUnstageVolume error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodeUnstageVolume() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNodeExpandVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.NodeExpandVolumeRequest
		resp                    *csi.NodeExpandVolumeResponse
		expectMounterCalls      func(m *mocks.MockMounter)
		expectFSCalls           func(m *mocks.MockFileSystem)
		expectCryptDeviceCalls  func(m *mocks.MockDevice)
		expectCryptSetUpCalls   func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice)
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "expandhappypath",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "1001-volkey",
				VolumePath: "/mnt/staging",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10,
				},
			},
			resp: &csi.NodeExpandVolumeResponse{
				CapacityBytes: 10,
			},
			expectCryptSetUpCalls: func(mc *mocks.MockCryptSetupClient, md *mocks.MockDevice) {
				mc.EXPECT().InitByName(gomock.Any()).Return(nil, fmt.Errorf("some error")).AnyTimes()
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockMounter := mocks.NewMockMounter(ctrl)
			mockExec := mocks.NewMockExecutor(ctrl)
			mockDevice := mocks.NewMockDevice(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			mockCryptSetupClient := mocks.NewMockCryptSetupClient(ctrl)
			mockClient := mocks.NewMockLinodeClient(ctrl)
			if tt.expectLinodeClientCalls != nil {
				tt.expectLinodeClientCalls(mockClient)
			}

			if tt.expectMounterCalls != nil {
				tt.expectMounterCalls(mockMounter)
			}
			if tt.expectFSCalls != nil {
				tt.expectFSCalls(mockFileSystem)
			}
			if tt.expectCryptSetUpCalls != nil {
				tt.expectCryptSetUpCalls(mockCryptSetupClient, mockDevice)
			}
			ns := &NodeServer{
				driver: &LinodeDriver{},
				mounter: &mount.SafeFormatAndMount{
					Interface: mockMounter,
					Exec:      mockExec,
				},
				deviceutils: devicemanager.NewDeviceUtils(mockFileSystem, mockExec),
				encrypt:     NewLuksEncryption(mockExec, mockFileSystem, mockCryptSetupClient),
				client:      mockClient,
			}
			returnedResp, err := ns.NodeExpandVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodeExpandVolume error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodeExpandVolume() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNodeGetCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		req           *csi.NodeGetCapabilitiesRequest
		resp          *csi.NodeGetCapabilitiesResponse
		expectedError error
	}{
		{
			name: "expandhappypath",
			req:  &csi.NodeGetCapabilitiesRequest{},
			resp: &csi.NodeGetCapabilitiesResponse{
				Capabilities: []*csi.NodeServiceCapability{
					{
						Type: &csi.NodeServiceCapability_Rpc{
							Rpc: &csi.NodeServiceCapability_RPC{
								Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
							},
						},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ns := &NodeServer{
				driver: &LinodeDriver{
					nscap: []*csi.NodeServiceCapability{
						{
							Type: &csi.NodeServiceCapability_Rpc{
								Rpc: &csi.NodeServiceCapability_RPC{
									Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
								},
							},
						},
					},
				},
			}
			returnedResp, err := ns.NodeGetCapabilities(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodeGetCapabilities error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodeGetCapabilities() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNodeGetInfo(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.NodeGetInfoRequest
		resp                    *csi.NodeGetInfoResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "getinfohappypath",
			req:  &csi.NodeGetInfoRequest{},
			resp: &csi.NodeGetInfoResponse{
				NodeId:            "10",
				MaxVolumesPerNode: 7,
				AccessibleTopology: &csi.Topology{
					Segments: map[string]string{
						"topology.linode.com/region": "testregion",
					},
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListInstanceDisks(gomock.Any(), gomock.Any(), gomock.Any()).Return([]linodego.InstanceDisk{
					{
						ID:    1,
						Label: "main",
					},
				}, nil)
			},
			expectedError: nil,
		},
		{
			name: "with swap disk",
			req:  &csi.NodeGetInfoRequest{},
			resp: &csi.NodeGetInfoResponse{
				NodeId:            "10",
				MaxVolumesPerNode: 6,
				AccessibleTopology: &csi.Topology{
					Segments: map[string]string{
						"topology.linode.com/region": "testregion",
					},
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListInstanceDisks(gomock.Any(), gomock.Any(), gomock.Any()).Return([]linodego.InstanceDisk{
					{
						ID:    1,
						Label: "main",
					},
					{
						ID:    2,
						Label: "swap",
					},
				}, nil)
			},
			expectedError: nil,
		},
		{
			name: "with a pv already attached",
			req:  &csi.NodeGetInfoRequest{},
			resp: &csi.NodeGetInfoResponse{
				NodeId:            "10",
				MaxVolumesPerNode: 7,
				AccessibleTopology: &csi.Topology{
					Segments: map[string]string{
						"topology.linode.com/region": "testregion",
					},
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListInstanceDisks(gomock.Any(), gomock.Any(), gomock.Any()).Return([]linodego.InstanceDisk{
					{
						ID:    1,
						Label: "main",
					},
					{
						ID:    2,
						Label: "pvc123",
					},
				}, nil)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := mocks.NewMockLinodeClient(ctrl)
			if tt.expectLinodeClientCalls != nil {
				tt.expectLinodeClientCalls(mockClient)
			}
			ns := &NodeServer{
				driver: &LinodeDriver{
					volumeLabelPrefix: "pvc",
				},
				client: mockClient,
				metadata: Metadata{
					ID:     10,
					Region: "testregion",
					Memory: 10,
				},
			}
			returnedResp, err := ns.NodeGetInfo(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("NodeGetCapabilities error = %v, wantErr %v", err, tt.expectedError)
			}
			if !reflect.DeepEqual(returnedResp, tt.resp) {
				t.Errorf("NodeServer.NodeGetCapabilities() = %v, want %v", returnedResp, tt.resp)
			}
		})
	}
}

func TestNewNodeServer(t *testing.T) {
	type args struct {
		linodeDriver *LinodeDriver
		mounter      *mount.SafeFormatAndMount
		deviceUtils  devicemanager.DeviceUtils
		client       linodeclient.LinodeClient
		metadata     Metadata
		encrypt      Encryption
	}
	tests := []struct {
		name    string
		args    args
		want    *NodeServer
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  devicemanager.NewDeviceUtils(filesystem.NewFileSystem(), exec.New()),
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want: &NodeServer{
				driver:      &LinodeDriver{},
				mounter:     &mount.SafeFormatAndMount{},
				deviceutils: devicemanager.NewDeviceUtils(filesystem.NewFileSystem(), exec.New()),
				client:      &linodego.Client{},
				metadata:    Metadata{},
				encrypt:     Encryption{},
			},
			wantErr: false,
		},
		{
			name: "nil linodeDriver",
			args: args{
				linodeDriver: nil,
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  devicemanager.NewDeviceUtils(filesystem.NewFileSystem(), exec.New()),
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil mounter",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      nil,
				deviceUtils:  devicemanager.NewDeviceUtils(filesystem.NewFileSystem(), exec.New()),
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil deviceUtils",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  nil,
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil linode client",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  devicemanager.NewDeviceUtils(filesystem.NewFileSystem(), exec.New()),
				client:       nil,
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNodeServer(context.Background(), tt.args.linodeDriver, tt.args.mounter, tt.args.deviceUtils, tt.args.client, tt.args.metadata, tt.args.encrypt)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNodeServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewNodeServer() = %v, want %v", got, tt.want)
			}
		})
	}
}
