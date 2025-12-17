package driver

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"go.uber.org/mock/gomock"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
)

func TestCreateVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.CreateVolumeRequest
		resp                    *csi.CreateVolumeResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "createhappypath",
			req: &csi.CreateVolumeRequest{
				Name: "createhappypath",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			resp: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      "1-",
					CapacityBytes: 10 << 30,
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								VolumeTopologyRegion: "",
							},
						},
					},
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, Size: 10}, nil)
				m.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1, Size: 10, Status: linodego.VolumeActive}, nil)
			},
			expectedError: nil,
		},
		{
			name: "createapierror",
			req: &csi.CreateVolumeRequest{
				Name: "createapierror",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			resp: &csi.CreateVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("volume creation failed"))
			},
			expectedError: errInternal("create volume: volume creation failed"),
		},
		{
			name: "incorrectsize",
			req: &csi.CreateVolumeRequest{
				Name: "incorrectsize",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			resp: &csi.CreateVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				m.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1002, Size: 20}, nil) // pass 20GB instead of 10
			},
			expectedError: errAlreadyExists("volume 1002 already exists with size 20"),
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
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client: mockClient,
				driver: ns.driver,
			}
			returnedResp, err := s.CreateVolume(context.Background(), tt.req)
			if err != nil && !errors.Is(err, tt.expectedError) {
				t.Errorf("CreateVolume error = %v, wantErr %v", err, tt.expectedError)
			} else if returnedResp.GetVolume() != nil && tt.resp.GetVolume() != nil {
				if returnedResp.GetVolume().GetCapacityBytes() != tt.resp.GetVolume().GetCapacityBytes() {
					t.Errorf("expected capacity: %+v, got: %+v", tt.resp.GetVolume().GetCapacityBytes(), returnedResp.GetVolume().GetCapacityBytes())
				}
			}
		})
	}
}

func TestDeleteVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.DeleteVolumeRequest
		resp                    *csi.DeleteVolumeResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "deletehappypath",
			req: &csi.DeleteVolumeRequest{
				VolumeId: "1001",
			},
			resp: &csi.DeleteVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedError: nil,
		},
		{
			name: "deleteapierror",
			req: &csi.DeleteVolumeRequest{
				VolumeId: "1001",
			},
			resp: &csi.DeleteVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).Return(fmt.Errorf("volume deletion failed"))
			},
			expectedError: errInternal("delete volume 597150807: volume deletion failed"), // 597150807 comes from converting 1001 string using hashStringToInt function
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
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client: mockClient,
				driver: ns.driver,
			}
			_, err := s.DeleteVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("DeleteVolume error = %+v, wantErr %+v", err, tt.expectedError)
			}
		})
	}
}

func TestControllerPublishVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.ControllerPublishVolumeRequest
		resp                    *csi.ControllerPublishVolumeResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		publishedCaps           map[string]*publishedVolumeInfo
		expectedError           error
	}{
		{
			name: "publishsuccess",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "1003",
				NodeId:   "1003",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeContext: map[string]string{
					VolumeTopologyRegion: "us-east",
				},
				Readonly: false,
			},
			resp: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"devicePath": "/dev/sda",
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetInstance(gomock.Any(), gomock.Any()).Return(&linodego.Instance{ID: 1003, Specs: &linodego.InstanceSpec{Memory: 16 << 10}}, nil)
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil).AnyTimes()
				m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), 630706045, gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().AttachVolume(gomock.Any(), 630706045, gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().ListInstanceVolumes(gomock.Any(), 1003, gomock.Any()).Return([]linodego.Volume{{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}}, nil)
				m.EXPECT().ListInstanceDisks(gomock.Any(), 1003, gomock.Any()).Return([]linodego.InstanceDisk{}, nil)
			},
			expectedError: nil,
		},
		{
			name: "idempotent publish with compatible capability",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "1004-testvol",
				NodeId:   "2004",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeContext: map[string]string{
					VolumeTopologyRegion: "us-east",
				},
				Readonly: false,
			},
			resp: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"devicePath": "/dev/disk/by-id/scsi-0Linode_Volume_test",
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetInstance(gomock.Any(), 2004).Return(&linodego.Instance{ID: 2004, Specs: &linodego.InstanceSpec{Memory: 16 << 10}}, nil)
				// Volume already attached to the same instance
				m.EXPECT().GetVolume(gomock.Any(), 1004).Return(&linodego.Volume{
					ID:             1004,
					LinodeID:       createLinodeID(2004),
					Size:           10,
					Status:         linodego.VolumeActive,
					FilesystemPath: "/dev/disk/by-id/scsi-0Linode_Volume_test",
				}, nil)
			},
			publishedCaps: map[string]*publishedVolumeInfo{
				"1004:2004": {
					capability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					readonly: false,
				},
			},
			expectedError: nil,
		},
		{
			name: "idempotent publish with incompatible readonly flag",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "1005-testvol",
				NodeId:   "2005",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeContext: map[string]string{
					VolumeTopologyRegion: "us-east",
				},
				Readonly: true, // Different from existing (false)
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetInstance(gomock.Any(), 2005).Return(&linodego.Instance{ID: 2005, Specs: &linodego.InstanceSpec{Memory: 16 << 10}}, nil)
				// Volume already attached to the same instance
				m.EXPECT().GetVolume(gomock.Any(), 1005).Return(&linodego.Volume{
					ID:             1005,
					LinodeID:       createLinodeID(2005),
					Size:           10,
					Status:         linodego.VolumeActive,
					FilesystemPath: "/dev/disk/by-id/scsi-0Linode_Volume_test",
				}, nil)
			},
			publishedCaps: map[string]*publishedVolumeInfo{
				"1005:2005": {
					capability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					readonly: false,
				},
			},
			expectedError: errAlreadyExists("volume 1005 already published to node 2005 with incompatible readonly flag"),
		},
		{
			name: "idempotent publish with incompatible capability type",
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "1006-testvol",
				NodeId:   "2006",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeContext: map[string]string{
					VolumeTopologyRegion: "us-east",
				},
				Readonly: false,
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetInstance(gomock.Any(), 2006).Return(&linodego.Instance{ID: 2006, Specs: &linodego.InstanceSpec{Memory: 16 << 10}}, nil)
				// Volume already attached to the same instance
				m.EXPECT().GetVolume(gomock.Any(), 1006).Return(&linodego.Volume{
					ID:             1006,
					LinodeID:       createLinodeID(2006),
					Size:           10,
					Status:         linodego.VolumeActive,
					FilesystemPath: "/dev/disk/by-id/scsi-0Linode_Volume_test",
				}, nil)
			},
			publishedCaps: map[string]*publishedVolumeInfo{
				"1006:2006": {
					capability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					readonly: false,
				},
			},
			expectedError: errAlreadyExists("volume 1006 already published to node 2006 with incompatible capability"),
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
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client:        mockClient,
				driver:        ns.driver,
				publishedCaps: tt.publishedCaps,
			}
			_, err := s.ControllerPublishVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("ControllerPublishVolume error: %+v, wantErr %+v", err, tt.expectedError)
			}
		})
	}
}

func TestControllerUnPublishVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.ControllerUnpublishVolumeRequest
		resp                    *csi.ControllerUnpublishVolumeResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "unpublishsuccess",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "1003",
				NodeId:   "1003",
			},
			resp: &csi.ControllerUnpublishVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), 630706045, gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().DetachVolume(gomock.Any(), 630706045).Return(nil)
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
			},
			expectedError: nil,
		},
		{
			name: "unplubishOptionnalNodeID",
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "1003",
			},
			resp: &csi.ControllerUnpublishVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), 630706045, gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().DetachVolume(gomock.Any(), 630706045).Return(nil)
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
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
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client: mockClient,
				driver: ns.driver,
			}
			_, err := s.ControllerUnpublishVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("ControllerUnpublishVolume error: %+v, wantErr %+v", err, tt.expectedError)
			}
		})
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.ValidateVolumeCapabilitiesRequest
		resp                    *csi.ValidateVolumeCapabilitiesResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "validatecapabilities",
			req: &csi.ValidateVolumeCapabilitiesRequest{
				VolumeId: "1003",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			resp: &csi.ValidateVolumeCapabilitiesResponse{
				Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
							},
						},
					},
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
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
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client: mockClient,
				driver: ns.driver,
			}
			_, err := s.ValidateVolumeCapabilities(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("ValidateVolumeCapabilities error %+v, wantErr %+v", err, tt.expectedError)
			}
		})
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.ControllerGetCapabilitiesRequest
		resp                    *csi.ControllerGetCapabilitiesResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "getcapabilities",
			req:  &csi.ControllerGetCapabilitiesRequest{},
			resp: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_PUBLISH_READONLY,
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
			mockClient := mocks.NewMockLinodeClient(ctrl)
			if tt.expectLinodeClientCalls != nil {
				tt.expectLinodeClientCalls(mockClient)
			}

			ns := &NodeServer{
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client: mockClient,
				driver: ns.driver,
			}
			_, err := s.ControllerGetCapabilities(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("ControllerGetCapabilities error: %+v, wantErr %+v", err, tt.expectedError)
			}
		})
	}
}

func TestControllerExpandVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.ControllerExpandVolumeRequest
		resp                    *csi.ControllerExpandVolumeResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "expandvolumeInUse",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "1003",
				CapacityRange: &csi.CapacityRange{
					LimitBytes: 20 << 30, // 20 GiB
				},
			},
			resp: &csi.ControllerExpandVolumeResponse{
				CapacityBytes:         10 << 30,
				NodeExpansionRequired: false,
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, Size: 10, Status: linodego.VolumeActive}, nil)
			},
			expectedError: nil,
		},
		{
			name: "expandvolume",
			req: &csi.ControllerExpandVolumeRequest{
				VolumeId: "1003",
				CapacityRange: &csi.CapacityRange{
					LimitBytes: 20 << 30, // 20 GiB
				},
			},
			resp: &csi.ControllerExpandVolumeResponse{
				CapacityBytes:         10 << 30,
				NodeExpansionRequired: false,
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, Size: 10, Status: linodego.VolumeActive}, nil)
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
				driver: &LinodeDriver{},
			}
			s := &ControllerServer{
				client: mockClient,
				driver: ns.driver,
			}
			_, err := s.ControllerExpandVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("ControllerExpandVolume error: %+v, wantErr %+v", err, tt.expectedError)
			}
		})
	}
}

//nolint:gocognit // As simple as possible.
func TestListVolumes(t *testing.T) {
	cases := map[string]struct {
		volumes  []linodego.Volume
		throwErr bool
	}{
		"volume attached to node": {
			volumes: []linodego.Volume{
				{
					ID:       1,
					Label:    "foo",
					Region:   "danmaaag",
					Size:     30,
					LinodeID: createLinodeID(10),
					Status:   linodego.VolumeActive,
				},
			},
		},
		"volume not attached": {
			volumes: []linodego.Volume{
				{
					ID:     1,
					Label:  "bar",
					Size:   30,
					Status: linodego.VolumeActive,
				},
			},
		},
		"multiple volumes - with attachments": {
			volumes: []linodego.Volume{
				{
					ID:       1,
					Label:    "foo",
					Size:     30,
					LinodeID: createLinodeID(5),
					Status:   linodego.VolumeActive,
				},
				{
					ID:       2,
					Label:    "foo",
					Size:     60,
					LinodeID: createLinodeID(10),
					Status:   linodego.VolumeActive,
				},
			},
		},
		"multiple volumes - mixed attachments": {
			volumes: []linodego.Volume{
				{
					ID:       1,
					Label:    "foo",
					Size:     30,
					LinodeID: createLinodeID(5),
					Status:   linodego.VolumeActive,
				},
				{
					ID:     2,
					Label:  "foo",
					Size:   30,
					Status: linodego.VolumeActive,
				},
			},
		},
		"Linode API error": {
			throwErr: true,
		},
	}

	for c, tt := range cases {
		t.Run(c, func(t *testing.T) {
			cs := &ControllerServer{
				client: &fakeLinodeClient{
					volumes:  tt.volumes,
					throwErr: tt.throwErr,
				},
			}

			resp, err := cs.ListVolumes(context.Background(), &csi.ListVolumesRequest{StartingToken: "10"})
			switch {
			case (err != nil && !tt.throwErr):
				t.Fatal("failed to list volumes:", err)
			case (err != nil && tt.throwErr):
				// expected failure
			case (err == nil && tt.throwErr):
				t.Fatal("should have failed to list volumes")
			}

			for _, entry := range resp.GetEntries() {
				volume := entry.GetVolume()
				if volume == nil {
					t.Error("nil volume")
					continue
				}

				var linodeVolume *linodego.Volume
				for _, v := range tt.volumes {
					key := linodevolumes.CreateLinodeVolumeKey(v.ID, v.Label)
					if volume.GetVolumeId() == key.GetVolumeKey() {
						v := v
						linodeVolume = &v
						break
					}
				}
				if linodeVolume == nil {
					t.Fatalf("no matching linode volume for %#v", volume)
					return
				}

				if want, got := int64(linodeVolume.Size<<30), volume.GetCapacityBytes(); want != got {
					t.Errorf("mismatched volume size: want=%d got=%d", want, got)
				}
				for _, topology := range volume.GetAccessibleTopology() {
					region, ok := topology.GetSegments()[VolumeTopologyRegion]
					if !ok {
						t.Error("region not set in volume topology")
					}
					if region != linodeVolume.Region {
						t.Errorf("mismatched regions: want=%q got=%q", linodeVolume.Region, region)
					}
				}

				status := entry.GetStatus()
				if status == nil {
					t.Error("nil status")
					continue
				}
				if status.GetVolumeCondition().GetAbnormal() {
					t.Error("abnormal volume condition")
				}

				if n := len(status.GetPublishedNodeIds()); n > 1 {
					t.Errorf("volume published to too many nodes (%d)", n)
				}

				switch publishedNodes := status.GetPublishedNodeIds(); {
				case len(publishedNodes) == 0 && linodeVolume.LinodeID == nil:
				// This case is fine - having it here prevents a segfault if we try to index into publishedNodes in the last case
				case len(publishedNodes) == 0 && linodeVolume.LinodeID != nil:
					t.Errorf("expected volume to be attached, got: %s, want: %d", status.GetPublishedNodeIds(), *linodeVolume.LinodeID)
				case len(publishedNodes) != 0 && linodeVolume.LinodeID == nil:
					t.Errorf("expected volume to be unattached, got: %s", publishedNodes)
				case publishedNodes[0] != fmt.Sprintf("%d", *linodeVolume.LinodeID):
					t.Fatalf("got: %s, want: %d published node id", status.GetPublishedNodeIds()[0], *linodeVolume.LinodeID)
				}
			}
		})
	}
}

var _ linodeclient.LinodeClient = &fakeLinodeClient{}

type fakeLinodeClient struct {
	volumes  []linodego.Volume
	disks    []linodego.InstanceDisk
	throwErr bool
}

func (flc *fakeLinodeClient) ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error) {
	return nil, nil
}

func (flc *fakeLinodeClient) ListVolumes(context.Context, *linodego.ListOptions) ([]linodego.Volume, error) {
	if flc.throwErr {
		return nil, errors.New("sad times mate")
	}
	return flc.volumes, nil
}

func (c *fakeLinodeClient) ListInstanceVolumes(_ context.Context, _ int, _ *linodego.ListOptions) ([]linodego.Volume, error) {
	return c.volumes, nil
}

func (c *fakeLinodeClient) ListInstanceDisks(_ context.Context, _ int, _ *linodego.ListOptions) ([]linodego.InstanceDisk, error) {
	return c.disks, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) GetRegion(context.Context, string) (*linodego.Region, error) {
	return nil, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) GetInstance(context.Context, int) (*linodego.Instance, error) {
	return nil, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) GetVolume(context.Context, int) (*linodego.Volume, error) {
	return nil, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) CreateVolume(context.Context, linodego.VolumeCreateOptions) (*linodego.Volume, error) {
	return nil, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) CloneVolume(context.Context, int, string) (*linodego.Volume, error) {
	return nil, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) AttachVolume(context.Context, int, *linodego.VolumeAttachOptions) (*linodego.Volume, error) {
	return nil, nil
}

func (flc *fakeLinodeClient) DetachVolume(context.Context, int) error { return nil }

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) WaitForVolumeLinodeID(context.Context, int, *int, int) (*linodego.Volume, error) {
	return nil, nil
}

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) WaitForVolumeStatus(context.Context, int, linodego.VolumeStatus, int) (*linodego.Volume, error) {
	return nil, nil
}

func (flc *fakeLinodeClient) DeleteVolume(context.Context, int) error { return nil }

func (flc *fakeLinodeClient) ResizeVolume(context.Context, int, int) error { return nil }

//nolint:nilnil // TODO: re-work tests
func (flc *fakeLinodeClient) NewEventPoller(context.Context, any, linodego.EntityType, linodego.EventAction) (*linodego.EventPoller, error) {
	return nil, nil
}

func createLinodeID(i int) *int {
	return &i
}

func TestControllerCanAttach(t *testing.T) {
	t.Parallel()

	tests := []struct {
		memory uint // memory in bytes
		nvols  int  // number of volumes already attached
		ndisks int  // number of attached disks
		want   bool // can we attach another?
		fail   bool // should we expect a non-nil error
	}{
		{
			memory: 1 << 30, // 1GiB
			nvols:  7,       // maxed out
			ndisks: 1,
		},
		{
			memory: 16 << 30, // 16GiB
			nvols:  14,       // should allow one more
			ndisks: 1,
			want:   true,
		},
		{
			memory: 16 << 30,
			nvols:  15,
			ndisks: 1,
		},
		{
			memory: 256 << 30, // 256GiB
			nvols:  64,        // maxed out
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, tt := range tests {
		tname := fmt.Sprintf("%dGB-%d", tt.memory>>30, tt.nvols)
		t.Run(tname, func(t *testing.T) {
			vols := make([]linodego.Volume, 0, tt.nvols)
			for i := 0; i < tt.nvols; i++ {
				vols = append(vols, linodego.Volume{ID: i})
			}

			disks := make([]linodego.InstanceDisk, 0, tt.ndisks)
			for i := 0; i < tt.ndisks; i++ {
				disks = append(disks, linodego.InstanceDisk{ID: i})
			}

			memMB := 8192
			if tt.memory != 0 {
				memMB = int(tt.memory >> 20) // convert bytes -> MB
			}
			instance := &linodego.Instance{
				Specs: &linodego.InstanceSpec{Memory: memMB},
			}

			srv := ControllerServer{
				client: &fakeLinodeClient{
					volumes: vols,
					disks:   disks,
				},
			}

			got, err := srv.canAttach(ctx, instance)
			if err != nil && !tt.fail {
				t.Fatal(err)
			} else if err == nil && tt.fail {
				t.Fatal("should have failed")
			}

			if got != tt.want {
				t.Errorf("got=%t want=%t", got, tt.want)
			}
		})
	}
}

func TestControllerMaxVolumeAttachments(t *testing.T) {
	tests := []struct {
		name     string
		instance *linodego.Instance
		want     int
		fail     bool
	}{
		{
			name: "NilInstance",
			fail: true,
		},
		{
			name:     "NilInstanceSpecs",
			instance: &linodego.Instance{},
			fail:     true,
		},

		// The test cases that follow should return the maximum number of
		// volumes (not block devices) that can be attached to the instance.
		// [maxPersistentAttachments] is the ideal maximum number of block
		// devices that can be attached to an instance.
		// Since this test uses a (fake) Linode client that reports instances
		// with a single instance disk, we need to subtract 1 (one) from
		// the expected result to count as "the number of volumes that can be
		// attached".
		{
			name: "1GB",
			instance: &linodego.Instance{
				Specs: &linodego.InstanceSpec{Memory: 1 << 10},
			},
			want: maxPersistentAttachments - 1,
		},
		{
			name: "16GB",
			instance: &linodego.Instance{
				Specs: &linodego.InstanceSpec{Memory: 16 << 10},
			},
			want: 15,
		},
		{
			name: "32GB",
			instance: &linodego.Instance{
				Specs: &linodego.InstanceSpec{Memory: 32 << 10},
			},
			want: 31,
		},
		{
			name: "64GB",
			instance: &linodego.Instance{
				Specs: &linodego.InstanceSpec{Memory: 64 << 10},
			},
			want: maxAttachments - 1,
		},
		{
			name: "96GB",
			instance: &linodego.Instance{
				Specs: &linodego.InstanceSpec{Memory: 96 << 10},
			},
			want: maxAttachments - 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ControllerServer{
				client: &fakeLinodeClient{
					disks: []linodego.InstanceDisk{
						{
							ID:         1,
							Label:      "boot",
							Status:     linodego.DiskReady,
							Size:       25 << 20, // 25GB in MB
							Filesystem: linodego.FilesystemExt4,
						},
					},
				},
			}
			got, err := s.maxAllowedVolumeAttachments(context.Background(), tt.instance)
			if err != nil && !tt.fail {
				t.Fatal(err)
			} else if err == nil && tt.fail {
				t.Fatal("should have failed")
			}
			if got != tt.want {
				t.Errorf("got=%d want=%d", got, tt.want)
			}
		})
	}
}

func Test_getVolumeResponse(t *testing.T) {
	type args struct {
		volume *linodego.Volume
	}
	tests := []struct {
		name                 string
		args                 args
		wantCsiVolume        *csi.Volume
		wantPublishedNodeIds []string
		wantVolumeCondition  *csi.VolumeCondition
	}{
		{
			name: "volume attached to node",
			args: args{
				volume: &linodego.Volume{
					ID:       1,
					Label:    "foo",
					Region:   "danmaaag",
					Size:     30,
					LinodeID: createLinodeID(10),
					Status:   linodego.VolumeActive,
				},
			},
			wantCsiVolume: &csi.Volume{
				VolumeId:      "1-foo",
				CapacityBytes: 30 << 30,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{
							VolumeTopologyRegion: "danmaaag",
						},
					},
				},
			},
			wantPublishedNodeIds: []string{"10"},
			wantVolumeCondition: &csi.VolumeCondition{
				Abnormal: false,
				Message:  "active",
			},
		},
		{
			name: "volume not attached",
			args: args{
				volume: &linodego.Volume{
					ID:     1,
					Label:  "bar",
					Size:   30,
					Status: linodego.VolumeActive,
				},
			},
			wantCsiVolume: &csi.Volume{
				VolumeId:      "1-bar",
				CapacityBytes: 30 << 30,
			},
			wantPublishedNodeIds: []string{},
			wantVolumeCondition: &csi.VolumeCondition{
				Abnormal: false,
				Message:  "active",
			},
		},
		{
			name: "volume attached with abnormal status",
			args: args{
				volume: &linodego.Volume{
					ID:       1,
					Label:    "foo",
					Region:   "danmaaag",
					Size:     30,
					LinodeID: createLinodeID(10),
					Status:   linodego.VolumeContactSupport,
				},
			},
			wantCsiVolume: &csi.Volume{
				VolumeId:      "1-foo",
				CapacityBytes: 30 << 30,
				AccessibleTopology: []*csi.Topology{
					{
						Segments: map[string]string{
							VolumeTopologyRegion: "danmaaag",
						},
					},
				},
			},
			wantPublishedNodeIds: []string{"10"},
			wantVolumeCondition: &csi.VolumeCondition{
				Abnormal: true,
				Message:  "contact_support",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCsiVolume, gotPublishedNodeIds, gotVolumeCondition := getVolumeResponse(tt.args.volume)
			if !reflect.DeepEqual(gotCsiVolume, tt.wantCsiVolume) {
				t.Errorf("getVolumeResponse() gotCsiVolume = %v, want %v", gotCsiVolume, tt.wantCsiVolume)
			}
			if !reflect.DeepEqual(gotPublishedNodeIds, tt.wantPublishedNodeIds) {
				t.Errorf("getVolumeResponse() gotPublishedNodeIds = %v, want %v", gotPublishedNodeIds, tt.wantPublishedNodeIds)
			}
			if !reflect.DeepEqual(gotVolumeCondition, tt.wantVolumeCondition) {
				t.Errorf("getVolumeResponse() gotVolumeCondition = %v, want %v", gotVolumeCondition, tt.wantVolumeCondition)
			}
		})
	}
}
func TestControllerGetVolume(t *testing.T) {
	tests := []struct {
		name                    string
		req                     *csi.ControllerGetVolumeRequest
		resp                    *csi.ControllerGetVolumeResponse
		expectLinodeClientCalls func(m *mocks.MockLinodeClient)
		expectedError           error
	}{
		{
			name: "volume exists",
			req: &csi.ControllerGetVolumeRequest{
				VolumeId: "1001",
			},
			resp: &csi.ControllerGetVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      "1001-foo",
					CapacityBytes: 30 << 30,
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								VolumeTopologyRegion: "us-east",
							},
						},
					},
				},
				Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
					PublishedNodeIds: []string{"10"},
					VolumeCondition: &csi.VolumeCondition{
						Abnormal: false,
						Message:  "active",
					},
				},
			},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), 597150807).Return(&linodego.Volume{
					ID:       1001,
					Label:    "foo",
					Region:   "us-east",
					Size:     30,
					LinodeID: createLinodeID(10),
					Status:   linodego.VolumeActive,
				}, nil)
			},
			expectedError: nil,
		},
		{
			name: "volume not found",
			req: &csi.ControllerGetVolumeRequest{
				VolumeId: "1002",
			},
			resp: &csi.ControllerGetVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), 613928426).Return(nil, &linodego.Error{Code: 404})
			},
			expectedError: errVolumeNotFound(613928426),
		},
		{
			name: "internal error",
			req: &csi.ControllerGetVolumeRequest{
				VolumeId: "1003",
			},
			resp: &csi.ControllerGetVolumeResponse{},
			expectLinodeClientCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), 630706045).Return(nil, fmt.Errorf("internal error"))
			},
			expectedError: errInternal("get volume: %v", fmt.Errorf("internal error")),
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

			cs := &ControllerServer{
				client: mockClient,
			}
			resp, err := cs.ControllerGetVolume(context.Background(), tt.req)
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
				t.Errorf("ControllerGetVolume error = %v, wantErr %v", err, tt.expectedError)
			} else if !reflect.DeepEqual(resp, tt.resp) {
				t.Errorf("ControllerGetVolume response = %v, want %v", resp, tt.resp)
			}
		})
	}
}

func TestControllerServer_checkPublishCompatibility(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		driver   *LinodeDriver
		client   linodeclient.LinodeClient
		metadata Metadata
		// Named input parameters for target function.
		volumeID int
		linodeID int
		req      *csi.ControllerPublishVolumeRequest
		wantErr  bool
	}{
		{
			name:     "no existing entry - should succeed",
			volumeID: 100,
			linodeID: 200,
			req: &csi.ControllerPublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				Readonly: false,
			},
			wantErr: false,
		},
		{
			name:     "existing entry with matching capability and readonly - should succeed",
			volumeID: 101,
			linodeID: 201,
			req: &csi.ControllerPublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				Readonly: false,
			},
			wantErr: false,
		},
		{
			name:     "existing entry with different readonly flag - should fail",
			volumeID: 102,
			linodeID: 202,
			req: &csi.ControllerPublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				Readonly: true, // existing is false
			},
			wantErr: true,
		},
		{
			name:     "existing entry with different access type (block vs mount) - should fail",
			volumeID: 103,
			linodeID: 203,
			req: &csi.ControllerPublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				Readonly: false,
			},
			wantErr: true,
		},
		{
			name:     "existing entry with different access mode - should fail",
			volumeID: 104,
			linodeID: 204,
			req: &csi.ControllerPublishVolumeRequest{
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY, // existing is SINGLE_NODE_WRITER
					},
				},
				Readonly: false,
			},
			wantErr: true,
		},
	}

	// Pre-populate existing entries for tests that need them
	existingCaps := map[string]*publishedVolumeInfo{
		"101:201": {
			capability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			readonly: false,
		},
		"102:202": {
			capability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			readonly: false, // request will have readonly=true
		},
		"103:203": {
			capability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			readonly: false, // request will have block instead of mount
		},
		"104:204": {
			capability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			readonly: false, // request will have different access mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &ControllerServer{
				driver:        &LinodeDriver{},
				publishedCaps: existingCaps,
			}
			gotErr := cs.checkPublishCompatibility(tt.volumeID, tt.linodeID, tt.req)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("checkPublishCompatibility() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("checkPublishCompatibility() succeeded unexpectedly")
			}
		})
	}
}
