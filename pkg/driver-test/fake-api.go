package drivertest

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linode/linodego"
)

// fakeAPI implements a fake, cached Linode API
type FakeAPI struct {
	T        *testing.T
	Volumes  map[string]linodego.Volume
	Instance *linodego.Instance
}

func (f *FakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case "GET":
		switch {
		case strings.HasPrefix(r.URL.Path, "/v4/volumes/"):
			// single volume get
			id := filepath.Base(r.URL.Path)
			vol, ok := f.Volumes[id]
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
		case strings.HasPrefix(r.URL.Path, "/v4/volumes"):
			res := 0
			data := []linodego.Volume{}

			var filters map[string]string
			hf := r.Header.Get("X-Filter")
			if hf != "" {
				_ = json.Unmarshal([]byte(hf), &filters)
			}

			for _, vol := range f.Volumes {
				if filters["label"] != "" && filters["label"] != vol.Label {
					continue
				}
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
		case strings.HasPrefix(r.URL.Path, "/v4/linode/instances/"):
			id := filepath.Base(r.URL.Path)
			if id == strconv.Itoa(f.Instance.ID) {
				rr, _ := json.Marshal(&f.Instance)
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
		case strings.HasPrefix(r.URL.Path, "/v4/linode/instances"):
			res := 1
			data := []linodego.Instance{}

			data = append(data, *f.Instance)
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
				f.T.Fatal(err)
			}
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 {
				f.T.Fatal("url not good")
			}
			vol, found = f.Volumes[parts[2]]
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
				f.T.Fatal("volume already attached")
				return
			}
			vol.LinodeID = &v.LinodeID
			f.Volumes[parts[2]] = vol
		} else if tp == "detach" {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 4 {
				f.T.Fatal("url not good")
			}
			vol, found = f.Volumes[parts[2]]
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
			f.Volumes[parts[2]] = vol
			return
		} else {
			v := new(linodego.VolumeCreateOptions)
			err := json.NewDecoder(r.Body).Decode(v)
			if err != nil {
				f.T.Fatal(err)
			}

			id := rand.Intn(99999)
			name := v.Label
			path := fmt.Sprintf("/dev/disk/by-id/scsi-0Linode_Volume_%v", name)
			now := time.Now()
			vol = linodego.Volume{
				ID:             id,
				Region:         v.Region,
				Label:          name,
				Size:           v.Size,
				FilesystemPath: path,
				Status:         linodego.VolumeActive,
				Tags:           v.Tags,
				Created:        &now,
				Updated:        &now,
			}

			f.Volumes[strconv.Itoa(id)] = vol
		}

		resp, err := json.Marshal(vol)
		if err != nil {
			f.T.Fatal(err)
		}
		_, _ = w.Write(resp)
	case "DELETE":
		id := filepath.Base(r.URL.Path)
		delete(f.Volumes, id)
	}
}
