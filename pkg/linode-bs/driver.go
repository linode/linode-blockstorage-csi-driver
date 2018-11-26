package linodebs

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

	"github.com/container-storage-interface/spec/lib/go/csi"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/metadata"

	"github.com/golang/glog"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

type LinodeDriver struct {
	name          string
	vendorVersion string

	ids *LinodeIdentityServer
	ns  *LinodeNodeServer
	cs  *LinodeControllerServer

	vcap  []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
	nscap []*csi.NodeServiceCapability
}

func GetLinodeDriver() *LinodeDriver {
	return &LinodeDriver{}
}

func (linodeDriver *LinodeDriver) SetupLinodeDriver(linodeClient linodeclient.LinodeClient, mounter *mount.SafeFormatAndMount,
	deviceUtils mountmanager.DeviceUtils, metadata metadata.MetadataService, name, vendorVersion string) error {
	if name == "" {
		return fmt.Errorf("Driver name missing")
	}

	linodeDriver.name = name
	linodeDriver.vendorVersion = vendorVersion

	// Adding Capabilities
	vcam := []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		// csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
	}
	if err := linodeDriver.AddVolumeCapabilityAccessModes(vcam); err != nil {
		return err
	}
	csc := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		// csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		// csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_PUBLISH_READONLY,
	}
	if err := linodeDriver.AddControllerServiceCapabilities(csc); err != nil {
		return err
	}
	ns := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	}
	if err := linodeDriver.AddNodeServiceCapabilities(ns); err != nil {
		return err
	}

	// Set up RPC Servers
	linodeDriver.ids = NewIdentityServer(linodeDriver)
	linodeDriver.ns = NewNodeServer(linodeDriver, mounter, deviceUtils, linodeClient, metadata)
	linodeDriver.cs = NewControllerServer(linodeDriver, linodeClient, metadata)

	return nil
}

func (linodeDriver *LinodeDriver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) error {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		glog.V(4).Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, NewVolumeCapabilityAccessMode(c))
	}
	linodeDriver.vcap = vca
	return nil
}

func (linodeDriver *LinodeDriver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) error {
	var csc []*csi.ControllerServiceCapability
	for _, c := range cl {
		glog.V(4).Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}
	linodeDriver.cscap = csc
	return nil
}

func (linodeDriver *LinodeDriver) AddNodeServiceCapabilities(nl []csi.NodeServiceCapability_RPC_Type) error {
	var nsc []*csi.NodeServiceCapability
	for _, n := range nl {
		glog.V(4).Infof("Enabling node service capability: %v", n.String())
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	linodeDriver.nscap = nsc
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

func NewIdentityServer(linodeDriver *LinodeDriver) *LinodeIdentityServer {
	return &LinodeIdentityServer{
		Driver: linodeDriver,
	}
}

func NewNodeServer(linodeDriver *LinodeDriver, mounter *mount.SafeFormatAndMount, deviceUtils mountmanager.DeviceUtils, cloudProvider linodeclient.LinodeClient, meta metadata.MetadataService) *LinodeNodeServer {
	return &LinodeNodeServer{
		Driver:          linodeDriver,
		Mounter:         mounter,
		DeviceUtils:     deviceUtils,
		CloudProvider:   cloudProvider,
		MetadataService: meta,
	}
}

func NewControllerServer(linodeDriver *LinodeDriver, cloudProvider linodeclient.LinodeClient, meta metadata.MetadataService) *LinodeControllerServer {
	return &LinodeControllerServer{
		Driver:          linodeDriver,
		CloudProvider:   cloudProvider,
		MetadataService: meta,
	}
}

func (linodeDriver *LinodeDriver) Run(endpoint string) {
	glog.V(4).Infof("Driver: %v", linodeDriver.name)

	//Start the nonblocking GRPC
	s := NewNonBlockingGRPCServer()
	// TODO(#34): Only start specific servers based on a flag.
	// In the future have this only run specific combinations of servers depending on which version this is.
	// The schema for that was in util. basically it was just s.start but with some nil servers.

	s.Start(endpoint, linodeDriver.ids, linodeDriver.cs, linodeDriver.ns)
	s.Wait()
}
