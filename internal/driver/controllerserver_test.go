package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
)

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

			resp, err := cs.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
			if err != nil && !tt.throwErr {
				t.Fatal("failed to list volumes:", err)
			} else if err != nil && tt.throwErr {
				// expected failure
			} else if err == nil && tt.throwErr {
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
