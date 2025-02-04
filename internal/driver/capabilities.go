package driver

import "github.com/container-storage-interface/spec/lib/go/csi"

// ControllerServiceCapabilities returns the list of capabilities supported by
// this driver's controller service.
func ControllerServiceCapabilities() []*csi.ControllerServiceCapability {
	capabilities := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_PUBLISH_READONLY,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
		csi.ControllerServiceCapability_RPC_VOLUME_CONDITION,
		csi.ControllerServiceCapability_RPC_GET_VOLUME,
	}

	cc := make([]*csi.ControllerServiceCapability, 0, len(capabilities))
	for _, c := range capabilities {
		cc = append(cc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: c,
				},
			},
		})
	}
	return cc
}

// NodeServiceCapabilities returns the list of capabilities supported by this
// driver's node service.
func NodeServiceCapabilities() []*csi.NodeServiceCapability {
	capabilities := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
	}

	cc := make([]*csi.NodeServiceCapability, 0, len(capabilities))
	for _, c := range capabilities {
		cc = append(cc, &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: c,
				},
			},
		})
	}
	return cc
}

// VolumeCapabilityAccessModes returns the allowed access modes for a volume
// created by the driver.
func VolumeCapabilityAccessModes() []*csi.VolumeCapability_AccessMode {
	modes := []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}

	mm := make([]*csi.VolumeCapability_AccessMode, 0, len(modes))
	for _, m := range modes {
		mm = append(mm, &csi.VolumeCapability_AccessMode{
			Mode: m,
		})
	}
	return mm
}
