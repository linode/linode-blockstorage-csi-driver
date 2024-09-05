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
)

// IdentityServer implements the CSI Identity service for the Linode Block Storage CSI Driver.
type IdentityServer struct {
	driver *LinodeDriver

	csi.UnimplementedIdentityServer
}

// NewIdentityServer creates and initializes a new IdentityServer.
// It takes a context and a LinodeDriver as input and returns a pointer to IdentityServer and an error.
func NewIdentityServer(ctx context.Context, linodeDriver *LinodeDriver) (*IdentityServer, error) {
	log := logger.GetLogger(ctx)

	log.V(4).Info("Creating new IdentityServer")

	if linodeDriver == nil {
		return nil, fmt.Errorf("linodeDriver cannot be nil")
	}

	identityServer := &IdentityServer{
		driver: linodeDriver,
	}

	log.V(4).Info("IdentityServer created successfully")
	return identityServer, nil
}

// GetPluginInfo returns information about the CSI plugin.
// This method is REQUIRED for the Identity service as per the CSI spec.
// It returns the name and version of the CSI plugin.
func (linodeIdentity *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("GetPluginInfo")
	defer done()

	log.V(2).Info("Processing request")

	if linodeIdentity.driver.name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	return &csi.GetPluginInfoResponse{
		Name:          linodeIdentity.driver.name,
		VendorVersion: linodeIdentity.driver.vendorVersion,
	}, nil
}

// GetPluginCapabilities returns the capabilities of the CSI plugin.
// This method is REQUIRED for the Identity service as per the CSI spec.
// It informs the CO of the supported features by this plugin.
func (linodeIdentity *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("GetPluginCapabilities")
	defer done()

	log.V(2).Info("Processing request")

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

// Probe checks if the plugin is ready to serve requests.
// This method is REQUIRED for the Identity service as per the CSI spec.
// It allows the CO to check the readiness of the plugin.
func (linodeIdentity *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("Probe")
	defer done()

	log.V(2).Info("Processing request")

	linodeIdentity.driver.readyMu.Lock()
	defer linodeIdentity.driver.readyMu.Unlock()

	return &csi.ProbeResponse{
		Ready: &wrapperspb.BoolValue{
			Value: linodeIdentity.driver.ready,
		},
	}, nil
}
