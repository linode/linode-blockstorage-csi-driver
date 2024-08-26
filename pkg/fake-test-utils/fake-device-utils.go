/*
Copyright 2018 The Kubernetes Authors.

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

package faketestutils

import (
	"github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

type fakeDeviceUtils struct {
}

var _ mountmanager.DeviceUtils = &fakeDeviceUtils{}

func NewFakeDeviceUtils() *fakeDeviceUtils {
	return &fakeDeviceUtils{}
}

// Returns list of all /dev/disk/by-id/* paths for given PD.
func (m *fakeDeviceUtils) GetDiskByIdPaths(pdName string, partition string) []string {
	// Don't need to implement this in the fake because we have no actual device paths
	return nil
}

// Returns the first path that exists, or empty string if none exist.
func (m *fakeDeviceUtils) VerifyDevicePath(devicePaths []string) (string, error) {
	// Return any random device path to use as mount source
	return "/dev/disk/fake-path", nil
}
