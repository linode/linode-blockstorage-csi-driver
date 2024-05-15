package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linodego"
)

type MetadataService interface {
	GetZone() string
	GetProject() string
	GetName() string
	GetNodeID() int
	Memory() uint
}

type metadataServiceManager struct {
	// Current zone the driver is running in
	region  string
	nodeID  int
	label   string
	project string
	memory  uint // Amount of memory, in bytes
}

var _ MetadataService = &metadataServiceManager{}

// NewMetadataService retrieves information about the linode where the
// application is currently running on.
func NewMetadataService(linodeClient linodeclient.LinodeClient, nodeName string) (metadata MetadataService, err error) {
	// Get information about the Linode this pod is executing in.
	// Assume that the Linode ID file does not exist.
	linodeInfo := nodeName
	isID := false

	// Attempt to get the Linode ID information
	data, err := os.ReadFile("/linode-info/linode-id")
	if err == nil {
		// File read was successful, use Linode ID to create metadata service
		linodeInfo = string(data)
		isID = true
	}

	// Linode instance
	var linode *linodego.Instance
	if isID {
		// Search for Linode instance by ID
		linode, err = getLinodeByID(linodeClient, linodeInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get linode by id: %s", err)
		}
	} else {
		// Search for Linode instance by label
		linode, err = getLinodeByLabel(linodeClient, linodeInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get linode by label: %s", err)
		}
	}

	// Figure out how much memory this instance has. The API returns the
	// amount of memory in MB (megabytes, not mebibytes) and even then, it
	// is a strange number.
	//
	// If we cannot determine how much memory the instance has, we are
	// simply going to assume 1GiB, which is the smallest amount of memory
	// available across all instance types as of 2024-05-16.
	var memory uint = 1 << 30
	if linode.Specs != nil {
		// Store the amount of memory as number of bytes.
		memory = uint(linode.Specs.Memory) << 20
	}

	return &metadataServiceManager{
		region:  linode.Region,
		nodeID:  linode.ID,
		label:   linode.Label,
		project: linode.Group,
		memory:  memory,
	}, nil
}

func getLinodeByID(client linodeclient.LinodeClient, id string) (*linodego.Instance, error) {
	linodeID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("Error processing ID %s: %v", id, err)
	}

	return client.GetInstance(context.Background(), linodeID)
}

func getLinodeByLabel(client linodeclient.LinodeClient, label string) (*linodego.Instance, error) {
	jsonFilter, _ := json.Marshal(map[string]string{"label": label})
	linodes, err := client.ListInstances(context.Background(), linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, fmt.Errorf("failed to list instances with filter %s: %s", string(jsonFilter), err)
	} else if len(linodes) != 1 {
		return nil, fmt.Errorf("Could not identify a Linode with label %q", label)
	}

	for _, linode := range linodes {
		if linode.Label == string(label) {
			return &linode, nil
		}
	}
	return nil, errors.New("User has no Linode instances with the given label")
}

func (manager *metadataServiceManager) GetZone() string    { return manager.region }
func (manager *metadataServiceManager) GetProject() string { return manager.project }
func (manager *metadataServiceManager) GetName() string    { return manager.label }
func (manager *metadataServiceManager) GetNodeID() int     { return manager.nodeID }
func (m *metadataServiceManager) Memory() uint             { return m.memory }
