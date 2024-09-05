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

func GetLinodeDriver(ctx context.Context) *LinodeDriver {
	log, _, done := logger.GetLogger(ctx).WithMethod("GetLinodeDriver")
	defer done()

	log.V(4).Info("Creating LinodeDriver")
	driver := &LinodeDriver{
		vcap:  VolumeCapabilityAccessModes(),
		cscap: ControllerServiceCapabilities(),
		nscap: NodeServiceCapabilities(),
	}
	log.V(4).Info("LinodeDriver created successfully")
	return driver
}

func (linodeDriver *LinodeDriver) SetupLinodeDriver(
	ctx context.Context,
	linodeClient linodeclient.LinodeClient,
	mounter *mount.SafeFormatAndMount,
	deviceUtils mountmanager.DeviceUtils,
	metadata Metadata,
	name,
	vendorVersion,
	volumeLabelPrefix string,
	encrypt Encryption,
) error {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("SetupLinodeDriver")
	defer done()

	log.V(4).Info("Setting up LinodeDriver")

	if name == "" {
		return fmt.Errorf("driver name missing")
	}

	linodeDriver.name = name
	linodeDriver.vendorVersion = vendorVersion

	log.V(3).Info("Validating volume label prefix", "prefix", volumeLabelPrefix)
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

	log.V(3).Info("Setting up RPC Servers")
	linodeDriver.ns, err = NewNodeServer(ctx, linodeDriver, mounter, deviceUtils, linodeClient, metadata, encrypt)
	if err != nil {
		return fmt.Errorf("new node server: %w", err)
	}

	linodeDriver.ids, err = NewIdentityServer(ctx, linodeDriver)
	if err != nil {
		return fmt.Errorf("new identity server: %w", err)
	}

	cs, err := NewControllerServer(ctx, linodeDriver, linodeClient, metadata)
	if err != nil {
		return fmt.Errorf("new controller server: %w", err)
	}
	linodeDriver.cs = cs

	log.V(4).Info("LinodeDriver setup completed successfully")
	return nil
}

func (linodeDriver *LinodeDriver) ValidateControllerServiceRequest(ctx context.Context, c csi.ControllerServiceCapability_RPC_Type) error {
	log, _, done := logger.GetLogger(ctx).WithMethod("ValidateControllerServiceRequest")
	defer done()

	log.V(4).Info("Validating controller service request", "type", c)

	if c == csi.ControllerServiceCapability_RPC_UNKNOWN {
		log.V(4).Info("Unknown controller service capability, skipping validation")
		return nil
	}

	for _, cap := range linodeDriver.cscap {
		if c == cap.GetRpc().Type {
			log.V(4).Info("Controller service request validated successfully")
			return nil
		}
	}

	return status.Error(codes.InvalidArgument, "Invalid controller service request")
}

func (linodeDriver *LinodeDriver) Run(ctx context.Context, endpoint string) {
	log, _, done := logger.GetLogger(ctx).WithMethod("Run")
	defer done()

	log.V(4).Info("Starting LinodeDriver", "name", linodeDriver.name)
	if len(linodeDriver.volumeLabelPrefix) > 0 {
		log.V(4).Info("BS Volume Prefix", "prefix", linodeDriver.volumeLabelPrefix)
	}

	linodeDriver.readyMu.Lock()
	linodeDriver.ready = true
	linodeDriver.readyMu.Unlock()

	log.V(3).Info("Starting non-blocking GRPC server")
	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, linodeDriver.ids, linodeDriver.cs, linodeDriver.ns)
	log.V(3).Info("GRPC server started successfully")
	s.Wait()
	log.V(4).Info("LinodeDriver run completed")
}
