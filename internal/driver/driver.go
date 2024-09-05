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
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

// Name is the name of the driver provided by this package.
// It is also used as the name of the socket file used for container
// orchestrator and driver communications.
const Name = "linodebs.csi.linode.com"

type LinodeDriver struct {
	name              string
	vendorVersion     string
	volumeLabelPrefix string

	ns  *NodeServer
	ids *IdentityServer
	cs  *ControllerServer

	vcap  []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
	nscap []*csi.NodeServiceCapability

	readyMu sync.Mutex // protects ready
	ready   bool
}

// MaxVolumeLabelPrefixLength is the maximum allowed length of a volume label
// prefix.
const MaxVolumeLabelPrefixLength = 12

func GetLinodeDriver() *LinodeDriver {
	return &LinodeDriver{
		vcap:  VolumeCapabilityAccessModes(),
		cscap: ControllerServiceCapabilities(),
		nscap: NodeServiceCapabilities(),
	}
}

func (linodeDriver *LinodeDriver) SetupLinodeDriver(
	linodeClient linodeclient.LinodeClient,
	mounter *mount.SafeFormatAndMount,
	deviceUtils mountmanager.DeviceUtils,
	metadata Metadata,
	name,
	vendorVersion,
	volumeLabelPrefix string,
	encrypt Encryption,
) error {
	if name == "" {
		return fmt.Errorf("driver name missing")
	}

	linodeDriver.name = name
	linodeDriver.vendorVersion = vendorVersion

	// Validate the volume label prefix, if it is set.
	//
	// First, we want to make sure it is the right length, then we will make
	// sure it only contains the acceptable characters.
	//
	// When checking the length, we will convert it to a []rune first, to count
	// the number of unicode characters.
	if r := []rune(volumeLabelPrefix); len(r) > MaxVolumeLabelPrefixLength {
		return fmt.Errorf("volume label prefix is too long: length=%d max=%d", len(r), MaxVolumeLabelPrefixLength)
	}
	matched, err := regexp.MatchString(`^[0-9A-Za-z_-]{0,`+strconv.Itoa(MaxVolumeLabelPrefixLength)+`}$`, volumeLabelPrefix)
	if err != nil {
		return fmt.Errorf("invalid regexp pattern: %w", err)
	}
	if !matched {
		return errors.New("volume label prefix may only contain: [A-Za-z0-9_-]")
	}
	linodeDriver.volumeLabelPrefix = volumeLabelPrefix

	// Set up RPC Servers
	linodeDriver.ns, err = NewNodeServer(linodeDriver, mounter, deviceUtils, linodeClient, metadata, encrypt)
	if err != nil {
		return fmt.Errorf("new node server: %w", err)
	}

	linodeDriver.ids, err = NewIdentityServer(linodeDriver)
	if err != nil {
		return fmt.Errorf("new identity server: %w", err)
	}

	cs, err := NewControllerServer(linodeDriver, linodeClient, metadata)
	if err != nil {
		return fmt.Errorf("new controller server: %w", err)
	}
	linodeDriver.cs = cs

	return nil
}

func (linodeDriver *LinodeDriver) ValidateControllerServiceRequest(c csi.ControllerServiceCapability_RPC_Type) error {
	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		return nil
	}

	for _, cap := range linodeDriver.cscap {
		if c == cap.GetRpc().Type {
			return nil
		}
	}

	return status.Error(codes.InvalidArgument, "Invalid controller service request")
}

func (linodeDriver *LinodeDriver) Run(ctx context.Context, endpoint string) {
	log := logger.GetLogger(ctx)
	log.V(4).Info("Driver", "name", linodeDriver.name)
	if len(linodeDriver.volumeLabelPrefix) > 0 {
		log.V(4).Info("BS Volume Prefix", "prefix", linodeDriver.volumeLabelPrefix)
	}

	linodeDriver.readyMu.Lock()
	linodeDriver.ready = true
	linodeDriver.readyMu.Unlock()

	// Start the nonblocking GRPC
	s := NewNonBlockingGRPCServer()
	// TODO(#34): Only start specific servers based on a flag.
	// In the future have this only run specific combinations of servers depending on which version this is.
	// The schema for that was in util. basically it was just s.start but with some nil servers.

	s.Start(endpoint, linodeDriver.ids, linodeDriver.cs, linodeDriver.ns)
	s.Wait()
}
