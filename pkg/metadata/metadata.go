package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linodego"
)

type MetadataService interface {
	GetZone() string
	GetProject() string
	GetName() string
	GetNodeID() int
}

type metadataServiceManager struct {
	// Current zone the driver is running in
	region  string
	nodeID  int
	label   string
	project string
}

var _ MetadataService = &metadataServiceManager{}

func NewMetadataService(linodeClient linodeclient.LinodeClient, zone, host string) (metadata MetadataService, err error) {
	linode, err := getLinodeByName(linodeClient, host)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize Linode client: %s", err)
	}

	return &metadataServiceManager{
		region:  zone,
		nodeID:  linode.ID,
		label:   linode.Label,
		project: linode.Group,
	}, nil

}

func getLinodeByName(client linodeclient.LinodeClient, nodeName string) (*linodego.Instance, error) {
	jsonFilter, _ := json.Marshal(map[string]string{"label": nodeName})
	linodes, err := client.ListInstances(context.Background(), linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, err
	} else if len(linodes) != 1 {
		return nil, fmt.Errorf("Could not identify a Linode ID with label %q", nodeName)
	}

	for _, linode := range linodes {
		if linode.Label == string(nodeName) {
			return &linode, nil
		}
	}
	return nil, errors.New("instance not found")
}

func (manager *metadataServiceManager) GetZone() string {
	return manager.region
}

func (manager *metadataServiceManager) GetProject() string {
	return manager.project
}

func (manager *metadataServiceManager) GetName() string {
	return manager.label
}

func (manager *metadataServiceManager) GetNodeID() int {
	return manager.nodeID
}
