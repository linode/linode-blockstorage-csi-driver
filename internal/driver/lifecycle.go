package driver

// VolumeLifecycle is a type used to indicate the phase a volume is at when
// it is published and/or staged to a node.
type VolumeLifecycle string

const (
	VolumeLifecycleNodeStageVolume     VolumeLifecycle = "NodeStageVolume"
	VolumeLifecycleNodePublishVolume   VolumeLifecycle = "NodePublishVolume"
	VolumeLifecycleNodeUnstageVolume   VolumeLifecycle = "NodeUnstageVolume"
	VolumeLifecycleNodeUnpublishVolume VolumeLifecycle = "NodeUnpublishVolume"
)
