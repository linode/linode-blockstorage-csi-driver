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
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	}
	ts := httptest.NewServer(fake)
	defer ts.Close()

	linodeClient := linodego.NewClient(nil)
	url, _ := url.Parse(ts.URL)
	linodeClient.BaseURL = url

	driver := &Driver{
		endpoint:     endpoint,
		nodeId:       "987654",
		region:       "us-east",
		linodeClient: linodeClient,
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

// fakeAPI implements a fake, cached DO API
type fakeAPI struct {
	t       *testing.T
	volumes map[string]*linodego.Volume
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		name := r.URL.Query().Get("name")
		if name != "" {
			// a list of volumes for the given name
			for _, vol := range f.volumes {
				if vol.Name == name {
					var resp = struct {
						Volumes []*linodego.Volume
						Links   *linodego.Links
					}{
						Volumes: []*linodego.Volume{vol},
					}
					err := json.NewEncoder(w).Encode(&resp)
					if err != nil {
						f.t.Fatal(err)
					}
					return
				}
			}
		} else {
			// single volume get
			id := filepath.Base(r.URL.Path)
			vol := f.volumes[id]
			var resp = struct {
				Volume *linodego.Volume
				Links  *linodego.Links
			}{
				Volume: vol,
			}
			_ = json.NewEncoder(w).Encode(&resp)
			return
		}

		// response with zero items
		var resp = struct {
			Volume []*linodego.Volume
			Links  *linodego.Links
		}{}
		err := json.NewEncoder(w).Encode(&resp)
		if err != nil {
			f.t.Fatal(err)
		}
	case "POST":
		v := new(linodego.VolumeCreateRequest)
		err := json.NewDecoder(r.Body).Decode(v)
		if err != nil {
			f.t.Fatal(err)
		}

		id := randString(10)
		vol := &linodego.Volume{
			ID: id,
			Region: &linodego.Region{
				Slug: v.Region,
			},
			Description:   v.Description,
			Name:          v.Name,
			SizeGigaBytes: v.SizeGigaBytes,
			CreatedAt:     time.Now().UTC(),
		}

		f.volumes[id] = vol

		var resp = struct {
			Volume *linodego.Volume
			Links  *linodego.Links
		}{
			Volume: vol,
		}
		err = json.NewEncoder(w).Encode(&resp)
		if err != nil {
			f.t.Fatal(err)
		}
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
