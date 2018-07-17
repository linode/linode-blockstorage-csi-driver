package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// GetPluginInfo provides the name and version of the plugin
func (d *Driver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	resp := &csi.GetPluginInfoResponse{
		Name:          driverName,
		VendorVersion: vendorVersion,
	}

	return resp, nil
}
