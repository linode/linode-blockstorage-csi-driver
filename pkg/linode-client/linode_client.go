package linodeclient

import (
	"context"

	"github.com/linode/linodego"
)

type LinodeClient interface {
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error) // Needed for metadata
	ListVolumes(context.Context, *linodego.ListOptions) ([]linodego.Volume, error)
	ListInstanceVolumes(ctx context.Context, instanceID int, options *linodego.ListOptions) ([]linodego.Volume, error)
	ListInstanceDisks(ctx context.Context, instanceID int, options *linodego.ListOptions) ([]linodego.InstanceDisk, error)

	GetRegion(ctx context.Context, regionID string) (*linodego.Region, error)
	GetInstance(context.Context, int) (*linodego.Instance, error)
	GetVolume(context.Context, int) (*linodego.Volume, error)

	CreateVolume(context.Context, linodego.VolumeCreateOptions) (*linodego.Volume, error)
	CloneVolume(context.Context, int, string) (*linodego.Volume, error)

	AttachVolume(context.Context, int, *linodego.VolumeAttachOptions) (*linodego.Volume, error)
	DetachVolume(context.Context, int) error

	WaitForVolumeLinodeID(context.Context, int, *int, int) (*linodego.Volume, error)
	WaitForVolumeStatus(context.Context, int, linodego.VolumeStatus, int) (*linodego.Volume, error)
	DeleteVolume(context.Context, int) error

	ResizeVolume(context.Context, int, int) error

	NewEventPoller(context.Context, any, linodego.EntityType, linodego.EventAction) (*linodego.EventPoller, error)
}

func NewLinodeClient(token, ua, apiURL string) (*linodego.Client, error) {
	// Use linodego built-in http client which supports setting root CA cert
	linodeClient := linodego.NewClient(nil)
	client, err := linodeClient.UseURL(apiURL)
	if err != nil {
		return nil, err
	}
	client.SetUserAgent(ua)
	client.SetToken(token)

	return client, nil
}
