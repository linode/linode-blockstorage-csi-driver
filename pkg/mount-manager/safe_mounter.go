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

package mountmanager

import (
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

type Mounter interface {
	mount.Interface
}

type Executor interface {
	exec.Interface
}

type Command interface {
	exec.Cmd
}

type Formater interface {
	FormatAndMount(source string, target string, fstype string, options []string) error
}

type ResizeFSer interface {
	Resize(devicePath string, deviceMountPath string) (bool, error)
	NeedResize(devicePath string, deviceMountPath string) (bool, error)
}

// alias mount.SafeFormatAndMount struct to add the Formater interface
type SafeFormatAndMount struct {
	mount.SafeFormatAndMount
	Formater
}

func NewSafeMounter() *SafeFormatAndMount {
	realMounter := mount.New("")
	realExec := exec.New()
	return &SafeFormatAndMount{
		SafeFormatAndMount: mount.SafeFormatAndMount{
			Interface: realMounter,
			Exec:      realExec,
		},
	}
}
