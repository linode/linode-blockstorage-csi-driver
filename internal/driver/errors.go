package driver

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errNotImplemented is a placeholder used to indicate something is not
// currently implemented.
//
// Ideally, this should never be used, but can be helpful for development.
var errNotImplemented = status.Error(codes.Unimplemented, "not implemented")

// Errors that are returned from RPC methods.
// They are defined here so they can be reused, and checked against in tests.
var (
	errNilDriver            = status.Error(codes.Internal, "nil driver")
	errNoVolumeName         = status.Error(codes.InvalidArgument, "volume name is required")
	errNoVolumeCapabilities = status.Error(codes.InvalidArgument, "volume capabilities are required")
	errVolumeInUse          = status.Error(codes.FailedPrecondition, "volume is in use")
	errNoVolumeCapability   = status.Error(codes.InvalidArgument, "no volume capability set")

	// errNilInstance is a general-purpose error used to indicate a nil
	// [github.com/linode/linodego.Instance] was passed as an argument to a
	// function.
	errNilInstance = errInternal("nil instance")

	// errMaxAttachments is used to indicate that the maximum number of attachments
	// for a Linode instance has already been reached.
	//
	// If you want to return an error that includes the maximum number of
	// attachments allowed for the instance, call errMaxVolumeAttachments.
	errMaxAttachments = status.Error(codes.ResourceExhausted, "max number of volumes already attached to instance")

	// errResizeDown indicates a request would result in a volume being resized
	// to be smaller than it currently is.
	//
	// The Linode API currently does not support resizing block storage volumes
	// to be smaller.
	errResizeDown = errInternal("volume cannot be resized to be smaller")

	// errUnsupportedVolumeContentSource indicates an invalid volume content
	// source was specified in a request.
	//
	// Currently, the only supported volume content source is "VOLUME".
	// The Linode API does not support block storage volume snapshots, and by
	// proxy, neither does this CSI driver.
	errUnsupportedVolumeContentSource = status.Error(codes.InvalidArgument, "unsupported volume content source type")

	// errNoSourceVolume indicates the source volume information for a clone
	// operation was not specified, despite indicating a new volume should be
	// created by cloning an existing one.
	errNoSourceVolume = status.Error(codes.InvalidArgument, "no volume content source specified")

	// errSmallVolumeCapacity indicates the caller specified a capacity range
	// for a volume that is smaller than the minimum allowed size of a Linode
	// block storage volume.
	//
	// The minimum allowed size for a volume is set by [MinVolumeSizeBytes].
	errSmallVolumeCapacity = status.Errorf(codes.OutOfRange, "specified volume capacity is less than the minimum of %d bytes", MinVolumeSizeBytes)

	// errSnapshot is returned to a caller when they call
	// [ControllerServer.CreateVolume] and specify a snapshot as the new
	// volume's content source.
	//
	// Linode does not support snapshot operations on block storage volumes.
	errSnapshot = status.Error(codes.InvalidArgument, "creating a volume from a snapshot is not supported")
)

// errRegionMismatch returns an error indicating a volume is in gotRegion, but
// should be in wantRegion.
func errRegionMismatch(gotRegion, wantRegion string) error {
	return status.Errorf(codes.InvalidArgument, "source volume is in region %q, needs to be in region %q", gotRegion, wantRegion)
}

func errMaxVolumeAttachments(numAttachments int) error {
	return status.Errorf(codes.ResourceExhausted, "max number of volumes (%d) already attached to instance", numAttachments)
}

func errInstanceNotFound(linodeID int) error {
	return status.Errorf(codes.NotFound, "linode instance %d not found", linodeID)
}

func errVolumeAttached(volumeID, linodeID int) error {
	return status.Errorf(codes.AlreadyExists, "volume %d is already attached to linode %d", volumeID, linodeID)
}

func errVolumeNotFound(volumeID int) error {
	return status.Errorf(codes.NotFound, "volume not found: %d", volumeID)
}

func errInvalidVolumeCapability(capability *csi.VolumeCapability) error {
	return status.Errorf(codes.InvalidArgument, "invalid volume capability: %s", capability)
}

// errInternal is a convenience function to return a gRPC error with an
// INTERNAL status code.
func errInternal(format string, args ...any) error {
	return status.Errorf(codes.Internal, format, args...)
}

// errInvalidArgument is a convenience function for returning RPC errors with
// the INVALID_ARGUMENT status code.
func errInvalidArgument(format string, args ...any) error {
	return status.Errorf(codes.InvalidArgument, format, args...)
}
