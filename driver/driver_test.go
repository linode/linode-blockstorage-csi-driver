/*
Copyright 2018 Digital Ocean

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

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

	"context"
	"strconv"

	"fmt"
	"strings"

	"github.com/chiefy/linodego"
	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	"github.com/sirupsen/logrus"
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

	// fake DO Server, not working yet ...
	fake := &fakeAPI{
		t:       t,
		volumes: map[string]*linodego.Volume{},
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

	linodeClient := linodego.NewClient(http.DefaultClient)
	linodeClient.SetBaseURL(ts.URL)

	driver := &Driver{
		endpoint:     endpoint,
		nodeId:       strconv.Itoa(fake.instance.ID),
		region:       "us-east",
		linodeClient: &linodeClient,
		mounter:      &fakeMounter{},
		log:          logrus.New().WithField("test_enabed", true),
	}
	defer driver.Stop()

	go driver.Run()

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

func createLinode(t *testing.T, client *linodego.Client) int {
	req := &linodego.InstanceCreateOptions{
		Type:   "g6-nanode-1",
		Region: "us-east",
	}
	in, err := client.CreateInstance(context.TODO(), req)
	if err != nil {
		t.Fatal(err)
	}
	return in.ID
}

func deleteLinode(t *testing.T, client *linodego.Client, id int) {
	err := client.DeleteInstance(context.TODO(), id)
	if err != nil {
		t.Fatal(err)
	}
}

// fakeAPI implements a fake, cached DO API
type fakeAPI struct {
	t        *testing.T
	volumes  map[string]*linodego.Volume
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
			vol := f.volumes[id]
			if vol == nil {
				resp := linodego.VolumesPagedResponse{
					PageOptions: &linodego.PageOptions{
						Page:    1,
						Pages:   1,
						Results: 0,
					},
					Data: []*linodego.Volume{},
				}
				rr, _ := json.Marshal(resp)
				w.Write(rr)
			}
			rr, _ := json.Marshal(&vol)
			w.Write(rr)

			return
		case strings.HasPrefix(r.URL.Path, "/volumes"):
			res := 0
			data := []*linodego.Volume{}

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
			w.Write(rr)
			return
		case strings.HasPrefix(r.URL.Path, "/linode/instances/"):
			id := filepath.Base(r.URL.Path)
			if id == strconv.Itoa(f.instance.ID) {
				rr, _ := json.Marshal(&f.instance)
				w.Write(rr)
				return
			}
		}

	case "POST":
		tp := filepath.Base(r.URL.Path)
		var vol linodego.Volume
		if tp == "attach" {
			v := new(linodego.VolumeAttachOptions)
			if err := json.NewDecoder(r.Body).Decode(v); err != nil {
				f.t.Fatal(err)
			}
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 {
				f.t.Fatal("url not good")
			}
			vol = *f.volumes[parts[2]]
			if vol.LinodeID != nil {
				f.t.Fatal("volume already attached")
				return
			}
			vol.LinodeID = &v.LinodeID
			f.volumes[parts[2]] = &vol

		} else if tp == "detach" {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 {
				f.t.Fatal("url not good")
			}
			vol = *f.volumes[parts[2]]
			vol.LinodeID = nil
			f.volumes[parts[2]] = &vol
			return
		} else {
			v := new(linodego.VolumeCreateOptions)
			err := json.NewDecoder(r.Body).Decode(v)
			if err != nil {
				f.t.Fatal(err)
			}

			id := rand.Intn(99999)
			name := randString(10)
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

			f.volumes[strconv.Itoa(id)] = &vol

		}

		resp, err := json.Marshal(vol)
		if err != nil {
			f.t.Fatal(err)
		}
		w.Write(resp)
	case "DELETE":
		id := filepath.Base(r.URL.Path)
		delete(f.volumes, id)
	}
}

func randString(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

type fakeMounter struct{}

func (f *fakeMounter) Format(source string, fsType string) error {
	return nil
}

func (f *fakeMounter) Mount(source string, target string, fsType string, options ...string) error {
	return nil
}

func (f *fakeMounter) Unmount(target string) error {
	return nil
}

func (f *fakeMounter) IsFormatted(source string) (bool, error) {
	return true, nil
}
func (f *fakeMounter) IsMounted(source, target string) (bool, error) {
	return true, nil
}
