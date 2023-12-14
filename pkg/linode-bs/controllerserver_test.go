package linodebs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	"github.com/linode/linodego"
)

func TestListVolumes(t *testing.T) {
	cases := map[string]struct {
		volumes  []linodego.Volume
		throwErr bool
	}{
		"volume attached to node": {
			volumes: []linodego.Volume{
				{
					ID:             1,
					Label:          "foo",
					Status:         "",
					Region:         "danmaaag",
					Size:           30,
					LinodeID:       createLinodeID(10),
					FilesystemPath: "",
					Tags:           []string{},
				},
			},
			throwErr: false,
		},
		"volume not attached": {
			volumes: []linodego.Volume{
				{
					ID:             1,
					Label:          "bar",
					Status:         "",
					Region:         "",
					Size:           30,
					FilesystemPath: "",
				},
			},
			throwErr: false,
		},
		"multiple volumes - with attachments": {
			volumes: []linodego.Volume{
				{
					ID:             1,
					Label:          "foo",
					Status:         "",
					Region:         "",
					Size:           30,
					LinodeID:       createLinodeID(5),
					FilesystemPath: "",
					Tags:           []string{},
				},
				{
					ID:             2,
					Label:          "foo",
					Status:         "",
					Region:         "",
					Size:           60,
					FilesystemPath: "",
					Tags:           []string{},
					LinodeID:       createLinodeID(10),
				},
			},
			throwErr: false,
		},
		"multiple volumes - mixed attachments": {
			volumes: []linodego.Volume{
				{
					ID:             1,
					Label:          "foo",
					Status:         "",
					Region:         "",
					Size:           30,
					LinodeID:       createLinodeID(5),
					FilesystemPath: "",
					Tags:           []string{},
				},
				{
					ID:             2,
					Label:          "foo",
					Status:         "",
					Region:         "",
					Size:           30,
					FilesystemPath: "",
					Tags:           []string{},
					LinodeID:       nil,
				},
			},
			throwErr: false,
		},
		"Linode API error": {
			volumes:  nil,
			throwErr: true,
		},
	}

	for c, tt := range cases {
		t.Run(c, func(t *testing.T) {
			cs := &LinodeControllerServer{
				CloudProvider: &fakeLinodeClient{
					volumes:  tt.volumes,
					throwErr: tt.throwErr,
				},
			}

			listVolsResp, err := cs.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
			if err != nil {
				if !tt.throwErr {
					t.Fatalf("test case got unexpected err: %s", err)
				}
				return
			}

			for _, entry := range listVolsResp.Entries {
				gotVol := entry.GetVolume()
				if gotVol == nil {
					t.Fatal("vol was nil")
				}

				var wantVol *linodego.Volume
				for _, v := range tt.volumes {
					v := v
					// The issue is that the ID returned is
					// not the same as what is passed in
					key := common.CreateLinodeVolumeKey(v.ID, v.Label)
					if gotVol.VolumeId == key.GetVolumeKey() {
						wantVol = &v
						break
					}
				}

				if wantVol == nil {
					t.Fatalf("failed to find input volume equivalent to: %#v", gotVol)
				}

				if gotVol.CapacityBytes != int64(wantVol.Size)*gigabyte {
					t.Errorf("volume size not equal, got: %d, want: %d", gotVol.CapacityBytes, wantVol.Size*gigabyte)
				}

				for _, i := range gotVol.GetAccessibleTopology() {
					region, ok := i.Segments[VolumeTopologyRegion]
					if !ok {
						t.Errorf("got empty region")
					}

					if region != wantVol.Region {
						t.Errorf("regions do not match, got: %s, want: %s", region, wantVol.Region)
					}
				}

				status := entry.GetStatus()
				if status == nil {
					t.Fatal("status was nil")
				}

				if status.VolumeCondition.Abnormal {
					t.Errorf("got abnormal volume condition")
				}

				if len(status.GetPublishedNodeIds()) > 1 {
					t.Errorf("volume was published on more than 1 node, got: %s", status.GetPublishedNodeIds())
				}

				switch publishedNodes := status.GetPublishedNodeIds(); {
				case len(publishedNodes) == 0 && wantVol.LinodeID == nil:
				// This case is fine - having it here prevents a segfault if we try to index into publishedNodes in the last case
				case len(publishedNodes) == 0 && wantVol.LinodeID != nil:
					t.Errorf("expected volume to be attached, got: %s, want: %d", status.GetPublishedNodeIds(), *wantVol.LinodeID)
				case len(publishedNodes) != 0 && wantVol.LinodeID == nil:
					t.Errorf("expected volume to be unattached, got: %s", publishedNodes)
				case publishedNodes[0] != fmt.Sprintf("%d", *wantVol.LinodeID):
					t.Fatalf("got: %s, want: %d published node id", status.GetPublishedNodeIds()[0], *wantVol.LinodeID)
				}
			}
		})

	}

}

type fakeLinodeClient struct {
	volumes  []linodego.Volume
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
func (flc *fakeLinodeClient) GetInstance(context.Context, int) (*linodego.Instance, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) GetVolume(context.Context, int) (*linodego.Volume, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) CreateVolume(context.Context, linodego.VolumeCreateOptions) (*linodego.Volume, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) CloneVolume(context.Context, int, string) (*linodego.Volume, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) AttachVolume(context.Context, int, *linodego.VolumeAttachOptions) (*linodego.Volume, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) DetachVolume(context.Context, int) error { return nil }
func (flc *fakeLinodeClient) WaitForVolumeLinodeID(context.Context, int, *int, int) (*linodego.Volume, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) WaitForVolumeStatus(context.Context, int, linodego.VolumeStatus, int) (*linodego.Volume, error) {
	return nil, nil
}
func (flc *fakeLinodeClient) DeleteVolume(context.Context, int) error      { return nil }
func (flc *fakeLinodeClient) ResizeVolume(context.Context, int, int) error { return nil }
func (flc *fakeLinodeClient) NewEventPoller(context.Context, any, linodego.EntityType, linodego.EventAction) (*linodego.EventPoller, error) {
	return nil, nil
}

func createLinodeID(i int) *int {
	return &i
}
