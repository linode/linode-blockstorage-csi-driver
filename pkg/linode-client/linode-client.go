package linodeclient

import (
	"context"

	"github.com/linode/linodego"
)

type LinodeClient interface {
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error) // Needed for metadata
	ListVolumes(context.Context, *linodego.ListOptions) ([]linodego.Volume, error)

	GetInstance(context.Context, int) (*linodego.Instance, error)
	GetVolume(context.Context, int) (*linodego.Volume, error)

	CreateVolume(context.Context, linodego.VolumeCreateOptions) (*linodego.Volume, error)

	AttachVolume(context.Context, int, *linodego.VolumeAttachOptions) (*linodego.Volume, error)
	DetachVolume(context.Context, int) error

	WaitForVolumeLinodeID(context.Context, int, *int, int) (*linodego.Volume, error)
	WaitForVolumeStatus(context.Context, int, linodego.VolumeStatus, int) (*linodego.Volume, error)
	DeleteVolume(context.Context, int) error

	ResizeVolume(context.Context, int, int) error
}

func NewLinodeClient(token, ua string, url string) *linodego.Client {
	// Use linodego built-in http client which supports setting root CA cert
	linodeClient := linodego.NewClient(nil)
	linodeClient.SetUserAgent(ua)
	linodeClient.SetToken(token)
	linodeClient.SetDebug(true)

	if url != "" {
		linodeClient.SetBaseURL(url)
	}

	return &linodeClient
}
