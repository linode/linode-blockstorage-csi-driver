package linodebs

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/metadata"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"

	"strconv"

	"fmt"
	"strings"

	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	"github.com/linode/linodego"
)

var (
	driver        = "linodebs.csi.linode.com"
	vendorVersion = "test-vendor"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestDriverSuite(t *testing.T) {
	socket := "/tmp/csi.sock"
	endpoint := "unix://" + socket
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove unix domain socket file %s, error: %s", socket, err)
	}

	bsPrefix := "test-"

	// mock Linode Server, not working yet ...
	fake := &fakeAPI{
		t:       t,
		volumes: map[string]linodego.Volume{},
		instance: &linodego.Instance{
			Label:      "linode123",
			Region:     "us-east",
			Image:      "linode/debian9",
			Type:       "g6-standard-2",
			Group:      "Linode-Group",
			ID:         123,
			Status:     "running",
			Hypervisor: "kvm",
			CreatedStr: "2018-01-01T00:01:01",
			UpdatedStr: "2018-01-01T00:01:01",
		},
	}

	ts := httptest.NewServer(fake)
	defer ts.Close()

	mounter := mountmanager.NewFakeSafeMounter()
	deviceUtils := mountmanager.NewFakeDeviceUtils()
	fakeCloudProvider := linodeclient.NewLinodeClient("dummy", fmt.Sprintf("LinodeCSI/%s", vendorVersion), ts.URL)
	// TODO fake metadata
	ms, err := metadata.NewMetadataService(fakeCloudProvider, fake.instance.Label)
	if err != nil {
		t.Fatalf("Failed to setup Linode metadata: %v", err)
	}
	linodeDriver := GetLinodeDriver()
	err = linodeDriver.SetupLinodeDriver(fakeCloudProvider, mounter, deviceUtils, ms, driver, vendorVersion, bsPrefix)
	if err != nil {
		t.Fatalf("Failed to setup Linode Driver: %v", err)
	}

	go linodeDriver.Run(endpoint)

	mntDir, err := ioutil.TempDir("", "mnt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mntDir)

	mntStageDir, err := ioutil.TempDir("", "mnt-stage")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mntStageDir)

	cfg := &sanity.Config{
		StagingPath: mntStageDir,
		TargetPath:  mntDir,
		Address:     endpoint,
	}

	sanity.Test(t, cfg)
}

// fakeAPI implements a fake, cached Linode API
type fakeAPI struct {
	t        *testing.T
	volumes  map[string]linodego.Volume
	instance *linodego.Instance
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case "GET":
		switch {
		case strings.HasPrefix(r.URL.Path, "/volumes/"):
			// single volume get
			id := filepath.Base(r.URL.Path)
			vol, ok := f.volumes[id]
			if ok {
				rr, _ := json.Marshal(vol)
				_, _ = w.Write(rr)
			} else {
				w.WriteHeader(404)
				resp := linodego.APIError{
					Errors: []linodego.APIErrorReason{
						{Reason: "Not Found"},
					},
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
			}
			return
		case strings.HasPrefix(r.URL.Path, "/volumes"):
			res := 0
			data := []linodego.Volume{}

			for _, vol := range f.volumes {
				data = append(data, vol)
			}
			resp := linodego.VolumesPagedResponse{
				PageOptions: &linodego.PageOptions{
					Page:    1,
					Pages:   1,
					Results: res,
				},
				Data: data,
			}
			rr, _ := json.Marshal(resp)
			_, _ = w.Write(rr)
			return
		case strings.HasPrefix(r.URL.Path, "/linode/instances/"):
			id := filepath.Base(r.URL.Path)
			if id == strconv.Itoa(f.instance.ID) {
				rr, _ := json.Marshal(&f.instance)
				_, _ = w.Write(rr)
				return
			} else {
				w.WriteHeader(404)
				resp := linodego.APIError{
					Errors: []linodego.APIErrorReason{
						{Reason: "Not Found"},
					},
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
			}
		case strings.HasPrefix(r.URL.Path, "/linode/instances"):
			res := 1
			data := []linodego.Instance{}

			data = append(data, *f.instance)
			resp := linodego.InstancesPagedResponse{
				PageOptions: &linodego.PageOptions{
					Page:    1,
					Pages:   1,
					Results: res,
				},
				Data: data,
			}
			rr, _ := json.Marshal(resp)
			_, _ = w.Write(rr)
			return

		}

	case "POST":
		tp := filepath.Base(r.URL.Path)
		var vol linodego.Volume
		var found bool
		if tp == "attach" {
			v := new(linodego.VolumeAttachOptions)
			if err := json.NewDecoder(r.Body).Decode(v); err != nil {
				f.t.Fatal(err)
			}
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 {
				f.t.Fatal("url not good")
			}
			vol, found = f.volumes[parts[2]]
			if !found {
				w.WriteHeader(404)
				resp := linodego.APIError{
					Errors: []linodego.APIErrorReason{
						{Reason: "Not Found"},
					},
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
				return
			}
			if vol.LinodeID != nil {
				f.t.Fatal("volume already attached")
				return
			}
			vol.LinodeID = &v.LinodeID
			f.volumes[parts[2]] = vol

		} else if tp == "detach" {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 {
				f.t.Fatal("url not good")
			}
			vol, found = f.volumes[parts[2]]
			if !found {
				w.WriteHeader(404)
				resp := linodego.APIError{
					Errors: []linodego.APIErrorReason{
						{Reason: "Not Found"},
					},
				}
				rr, _ := json.Marshal(resp)
				_, _ = w.Write(rr)
				return
			}
			vol.LinodeID = nil
			f.volumes[parts[2]] = vol
			return
		} else {
			v := new(linodego.VolumeCreateOptions)
			err := json.NewDecoder(r.Body).Decode(v)
			if err != nil {
				f.t.Fatal(err)
			}

			id := rand.Intn(99999)
			name := v.Label
			path := fmt.Sprintf("/dev/disk/by-id/scsi-0Linode_Volume_%v", name)
			vol = linodego.Volume{
				ID:             id,
				Region:         v.Region,
				Label:          name,
				Size:           v.Size,
				FilesystemPath: path,
				Status:         linodego.VolumeActive,
				Created:        time.Now(),
				Updated:        time.Now(),

				CreatedStr: time.Now().Format("2006-01-02T15:04:05"),
				UpdatedStr: time.Now().Format("2006-01-02T15:04:05"),
			}

			f.volumes[strconv.Itoa(id)] = vol

		}

		resp, err := json.Marshal(vol)
		if err != nil {
			f.t.Fatal(err)
		}
		_, _ = w.Write(resp)
	case "DELETE":
		id := filepath.Base(r.URL.Path)
		delete(f.volumes, id)
	}
}
