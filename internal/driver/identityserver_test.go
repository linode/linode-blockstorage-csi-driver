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
	"context"
	"reflect"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

func TestNewIdentityServer(t *testing.T) {
	tests := []struct {
		name         string
		linodeDriver *LinodeDriver
		wantServer   *IdentityServer
		wantErr      bool
	}{
		{
			name:         "Successfully create IdentityServer",
			linodeDriver: &LinodeDriver{},
			wantServer: &IdentityServer{
				driver: &LinodeDriver{},
			},
			wantErr: false,
		},
		{
			name:         "Fail to create IdentityServer with nil driver",
			linodeDriver: nil,
			wantServer:   nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotServer, err := NewIdentityServer(context.Background(), tt.linodeDriver)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewIdentityServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotServer, tt.wantServer) {
				t.Errorf("NewIdentityServer() = %v, want %v", gotServer, tt.wantServer)
			}
		})
	}
}

func TestIdentityServer_GetPluginInfo(t *testing.T) {
	tests := []struct {
		name          string
		driverName    string
		driverVersion string
		wantResponse  *csi.GetPluginInfoResponse
		wantErr       bool
	}{
		{
			name:          "Successfully get plugin info",
			driverName:    "test-driver",
			driverVersion: "v1.0.0",
			wantResponse: &csi.GetPluginInfoResponse{
				Name:          "test-driver",
				VendorVersion: "v1.0.0",
			},
			wantErr: false,
		},
		{
			name:          "Fail to get plugin info with empty driver name",
			driverName:    "",
			driverVersion: "v1.0.0",
			wantResponse:  nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linodeIdentity := &IdentityServer{
				driver: &LinodeDriver{
					name:          tt.driverName,
					vendorVersion: tt.driverVersion,
				},
			}
			gotResponse, err := linodeIdentity.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})

			if (err != nil) != tt.wantErr {
				t.Errorf("IdentityServer.GetPluginInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResponse, tt.wantResponse) {
				t.Errorf("IdentityServer.GetPluginInfo() = %v, want %v", gotResponse, tt.wantResponse)
			}
		})
	}
}

func TestIdentityServer_GetPluginCapabilities(t *testing.T) {
	wantCapabilities := []*csi.PluginCapability{
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
				VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
					Type: csi.PluginCapability_VolumeExpansion_ONLINE,
				},
			},
		},
	}

	linodeIdentity := &IdentityServer{driver: &LinodeDriver{}}
	gotResponse, err := linodeIdentity.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})

	if err != nil {
		t.Errorf("IdentityServer.GetPluginCapabilities() unexpected error: %v", err)
	}

	if !reflect.DeepEqual(gotResponse.GetCapabilities(), wantCapabilities) {
		t.Errorf("IdentityServer.GetPluginCapabilities() = %v, want %v", gotResponse.GetCapabilities(), wantCapabilities)
	}
}

func TestIdentityServer_Probe(t *testing.T) {
	tests := []struct {
		name        string
		driverReady bool
		wantReady   bool
	}{
		{
			name:        "Driver is ready",
			driverReady: true,
			wantReady:   true,
		},
		{
			name:        "Driver is not ready",
			driverReady: false,
			wantReady:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linodeIdentity := &IdentityServer{
				driver: &LinodeDriver{
					ready: tt.driverReady,
				},
			}
			gotResponse, err := linodeIdentity.Probe(context.Background(), &csi.ProbeRequest{})

			if err != nil {
				t.Errorf("IdentityServer.Probe() unexpected error: %v", err)
				return
			}

			if gotResponse.GetReady().GetValue() != tt.wantReady {
				t.Errorf("IdentityServer.Probe() ready = %v, want %v", gotResponse.GetReady().GetValue(), tt.wantReady)
			}
		})
	}
}
