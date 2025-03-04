package driver

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Errors that are returned from RPC methods.
// They are defined here so they can be reused, and checked against in tests.
var (
	errNilDriver            = status.Error(codes.Internal, "nil driver")
	errNoVolumeName         = status.Error(codes.InvalidArgument, "volume name is required")
	errNoVolumeCapabilities = status.Error(codes.InvalidArgument, "volume capabilities are required")
	errVolumeInUse          = status.Error(codes.FailedPrecondition, "volume is in use")
	errNoVolumeCapability   = status.Error(codes.InvalidArgument, "no volume capability set")
	errNoVolumeID           = status.Error(codes.InvalidArgument, "volume id is not set")
	errNoVolumePath         = status.Error(codes.InvalidArgument, "volume path is not set")
	errNoStagingTargetPath  = status.Error(codes.InvalidArgument, "staging target path is not set")
	errNoTargetPath         = status.Error(codes.InvalidArgument, "target path is not set")

	// errNilSource is a general-purpose error used to indicate a nil source the volume will be created from
	errNilSource = errInternal("nil source volume")

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
	return status.Errorf(codes.FailedPrecondition, "volume %d is already attached to linode %d", volumeID, linodeID)
}

func errVolumeNotFound(volumeID int) error {
	return status.Errorf(codes.NotFound, "volume not found: %d", volumeID)
}

func errInvalidVolumeCapability(capability []*csi.VolumeCapability) error {
	return status.Errorf(codes.InvalidArgument, "invalid volume capability: %v", capability)
}

// errInternal is a convenience function to return a gRPC error with an
// INTERNAL status code.
func errInternal(format string, args ...any) error {
	return status.Errorf(codes.Internal, format, args...)
}

// errNotFound returns a gRPC error with a NOT_FOUND status code.
// It formats the error message using the provided format and arguments.
func errNotFound(format string, args ...any) error {
	return status.Errorf(codes.NotFound, format, args...)
}

// errAlreadyExists returns a gRPC error for an already existing resource.
//
// Parameters: format (string), args (...any)
// Returns: error
func errAlreadyExists(format string, args ...any) error {
	return status.Errorf(codes.AlreadyExists, format, args...)
}
