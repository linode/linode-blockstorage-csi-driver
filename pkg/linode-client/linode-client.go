package linodeclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/linode/linodego"
	"golang.org/x/oauth2"
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
}

func NewLinodeClient(token, uaPrefix string, url string) *linodego.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token,
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	ua := fmt.Sprintf("%s linodego/%s", uaPrefix, linodego.Version)
	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetUserAgent(ua)
	linodeClient.SetDebug(true)

	if url != "" {
		linodeClient.SetBaseURL(url)
	}

	return &linodeClient
}
