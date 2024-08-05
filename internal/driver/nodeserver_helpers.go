package driver

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

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)
// ValidateNodeStageVolumeRequest validates the node stage volume request.
// It validates the volume ID, staging target path, and volume capability.
func validateNodeStageVolumeRequest(req *csi.NodeStageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errNoVolumeID
	}
	if req.GetStagingTargetPath() == "" {
		return errNoStagingTargetPath
	}
	if req.GetVolumeCapability() == nil {
		return errNoVolumeCapability
	}
	return nil

}

// validateNodeUnstageVolumeRequest validates the node unstage volume request.
// It validates the volume ID and staging target path.
func validateNodeUnstageVolumeRequest(req *csi.NodeUnstageVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errNoVolumeID
	}
	if req.GetStagingTargetPath() == "" {
		return errNoStagingTargetPath
	}
	return nil

}
// closeMountSources closes any LUKS-encrypted mount sources associated with the given path.
// It retrieves mount sources, checks if each source is a LUKS mapping, and closes it if so.
// Returns an error if any operation fails during the process.
func (ns *LinodeNodeServer) closeLuksMountSources(path string) error {
	mountSources, err := ns.getMountSources(path)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("closeMountSources failed to to get mount sources %s: %v", path, err))
	}
	klog.V(4).Info("closing mount sources: ", mountSources)
	for _, source := range mountSources {
		isLuksMapping, mappingName, err := ns.Encrypt.isLuksMapping(source)
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("closeMountSources failed determine if mount is a luks mapping %s: %v", path, err))
		}
		if isLuksMapping {
			klog.V(4).Infof("luksClose %s", mappingName)
			if err := ns.Encrypt.luksClose(mappingName); err != nil {
				return status.Error(codes.Internal, fmt.Sprintf("closeMountSources failed to close luks mount %s: %v", path, err))
			}
		}
	}

	return nil
}

// getMountSources retrieves the mount sources for a given target path using the 'findmnt' command.
// It returns a slice of strings containing the mount sources, or an error if the operation fails.
// If 'findmnt' is not found or returns no results, appropriate errors or an empty slice are returned.
func (ns *LinodeNodeServer) getMountSources(target string) ([]string, error) {
	_, err := ns.Mounter.Exec.LookPath("findmnt")
	if err != nil {
		if err == exec.ErrNotFound {
			return nil, fmt.Errorf("%q executable not found in $PATH", "findmnt")
		}
		return nil, err
	}
	out, err := ns.Mounter.Exec.Command("sh", "-c", fmt.Sprintf("findmnt -o SOURCE -n -M %s", target)).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("checking mounted failed: %v cmd: %q output: %q",
			err, "findmnt", string(out))
	}
	return strings.Split(string(out), "\n"), nil
}
