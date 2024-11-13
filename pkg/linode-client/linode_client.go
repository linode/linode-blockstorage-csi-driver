package linodeclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"

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
	linodeClient.SetUserAgent(ua)
	linodeClient.SetToken(token)

	if apiURL != "" {
		host, version, err := getAPIURLComponents(apiURL)
		if err != nil {
			return nil, err
		}

		linodeClient.SetBaseURL(host)

		if version != "" {
			linodeClient.SetAPIVersion(version)
		}
	}

	return &linodeClient, nil
}

// getAPIURLComponents returns the API URL components (base URL, api version) given an input URL.
// This is necessary due to some recent changes with how linodego handles
// client.SetBaseURL(...) and client.SetAPIVersion(...)
func getAPIURLComponents(apiURL string) (host, version string, err error) {
	urlObj, err := url.Parse(apiURL)
	if err != nil {
		return "", "", err
	}

	version = ""
	host = fmt.Sprintf("%s://%s", urlObj.Scheme, urlObj.Host)

	// Check if the URL path is not empty
	if strings.TrimPrefix(urlObj.Path, "/") != "" {
		// Split the path into segments
		pathSegments := strings.Split(strings.TrimPrefix(urlObj.Path, "/"), "/")
		// If there are any segments, the first one is the API version
		if len(pathSegments) > 0 {
			version = pathSegments[0]
		}
	}

	return host, version, nil
}
