package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
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

			srv := LinodeControllerServer{
				CloudProvider: &fakeLinodeClient{
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
			s := &LinodeControllerServer{
				CloudProvider: &fakeLinodeClient{
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
			got, err := s.maxVolumeAttachments(context.Background(), tt.instance)
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
