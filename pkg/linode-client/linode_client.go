package linodeclient

import (
	"context"

	"github.com/linode/linodego/v2"
)

// DefaultListPageSize is the page size used for internal Linode list requests.
const DefaultListPageSize = 500

// NewListOptions creates list options with the driver's standard page size.
func NewListOptions(page int, filter string) *linodego.ListOptions {
	options := linodego.NewListOptions(page, filter)
	options.PageSize = DefaultListPageSize
	return options
}

type LinodeClient interface {
	ListInstances(context.Context, *linodego.ListOptions) ([]linodego.Instance, error) // Needed for metadata
	ListVolumes(context.Context, *linodego.ListOptions) ([]linodego.Volume, error)
	ListInstanceVolumes(ctx context.Context, instanceID int, options *linodego.ListOptions) ([]linodego.Volume, error)
	ListInstanceDisks(ctx context.Context, instanceID int, options *linodego.ListOptions) ([]linodego.InstanceDisk, error)

	GetRegion(ctx context.Context, regionID string) (*linodego.Region, error)
	GetInstance(context.Context, int) (*linodego.Instance, error)
	GetVolume(context.Context, int) (*linodego.Volume, error)

	CreateVolume(context.Context, linodego.VolumeCreateOptions) (*linodego.Volume, error)
	CloneVolume(context.Context, int, linodego.VolumeCloneOptions) (*linodego.Volume, error)

	AttachVolume(context.Context, int, *linodego.VolumeAttachOptions) (*linodego.Volume, error)
	DetachVolume(context.Context, int) error

	WaitForVolumeLinodeID(context.Context, int, *int) (*linodego.Volume, error)
	WaitForVolumeStatus(context.Context, int, linodego.VolumeStatus) (*linodego.Volume, error)
	DeleteVolume(context.Context, int) error

	ResizeVolume(context.Context, int, linodego.VolumeResizeOptions) error

	NewEventPoller(context.Context, any, linodego.EntityType, linodego.EventAction) (*linodego.EventPoller, error)
}

// linodego.Client implements LinodeClient
var _ LinodeClient = (*linodego.Client)(nil)

func NewLinodeClient(token, ua, apiURL string) (*linodego.Client, error) {
	// Use linodego built-in http client which supports setting root CA cert
	linodeClient, err := linodego.NewClient(nil)
	if err != nil {
		return nil, err
	}
	// Only override the base URL if a custom API URL is provided. Recent versions
	// of linodego return an error when given an empty string to UseURL.
	var (
		client *linodego.Client
	)
	if apiURL != "" {
		client, err = linodeClient.UseURL(apiURL)
		if err != nil {
			return nil, err
		}
	} else {
		client = &linodeClient
	}
	client.SetUserAgent(ua)
	client.SetToken(token)

	return client, nil
}
