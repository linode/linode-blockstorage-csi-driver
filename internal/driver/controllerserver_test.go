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
			if err != nil && !reflect.DeepEqual(tt.expectedError, err) {
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
				m.EXPECT().GetVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
				m.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 1001, LinodeID: createLinodeID(1003), Size: 10, Status: linodego.VolumeActive}, nil)
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
				},
			},
		},
		"volume not attached": {
			volumes: []linodego.Volume{
				{
					ID:    1,
					Label: "bar",
					Size:  30,
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
				},
				{
					ID:       2,
					Label:    "foo",
					Size:     60,
					LinodeID: createLinodeID(10),
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
				},
				{
					ID:    2,
					Label: "foo",
					Size:  30,
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
