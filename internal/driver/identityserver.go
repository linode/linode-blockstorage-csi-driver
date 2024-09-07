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

package driver

import (
	"fmt"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/klog/v2"
)

type IdentityServer struct {
	driver *LinodeDriver

	csi.UnimplementedIdentityServer
}

func NewIdentityServer(ctx context.Context, linodeDriver *LinodeDriver) (*IdentityServer, error) {
	log := logger.GetLogger(ctx)

	log.V(4).Info("Creating new IdentityServer")

	if linodeDriver == nil {
		log.Error(nil, "LinodeDriver is nil")
		return nil, fmt.Errorf("linodeDriver cannot be nil")
	}

	identityServer := &IdentityServer{
		driver: linodeDriver,
	}

	log.V(4).Info("IdentityServer created successfully")
	return identityServer, nil
}

// GetPluginInfo(context.Context, *GetPluginInfoRequest) (*GetPluginInfoResponse, error)
func (linodeIdentity *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	klog.V(5).Infof("Using default GetPluginInfo")

	if linodeIdentity.driver.name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	return &csi.GetPluginInfoResponse{
		Name:          linodeIdentity.driver.name,
		VendorVersion: linodeIdentity.driver.vendorVersion,
	}, nil
}

func (linodeIdentity *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	klog.V(5).Infof("Using default GetPluginCapabilities")
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
			{
				Type: &csi.PluginCapability_VolumeExpansion_{
					// We currently only support offline volume expansion
					// In order to use the feature:
					// 	1. Update your PersistentVolumeClaim k8s object to desired size(note that the size needs to be more than what it currently is)
					// 	2. Delete and recreate the pod that is using the PVC(or scale replicas accordingly)
					// 	3. This operation should detach and re-attach the volume to the newly created pod allowing you to use the updated size
					VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
						Type: csi.PluginCapability_VolumeExpansion_ONLINE,
					},
				},
			},
		},
	}, nil
}

func (linodeIdentity *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	klog.V(4).Infof("Probe called with args: %#v", req)
	linodeIdentity.driver.readyMu.Lock()
	defer linodeIdentity.driver.readyMu.Unlock()

	return &csi.ProbeResponse{
		Ready: &wrapperspb.BoolValue{
			Value: linodeIdentity.driver.ready,
		},
	}, nil
}
