package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/golang/glog"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linodego"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const providerIDPrefixIBM = "ibm://"

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

// NewMetadataService retrieves information about the linode where the
// application is currently running on.
func NewMetadataService(linodeClient linodeclient.LinodeClient, nodeName string) (metadata MetadataService, err error) {
	// Get information about the Linode this pod is executing in.
	// Assume that the Linode ID file does not exist.
	linodeInfo := nodeName
	isID := false

	// Attempt to get the Linode ID information
	data, err := ioutil.ReadFile("/linode-info/linode-id")
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
			return nil, err
		}
	} else {
		// Search for Linode instance by label
		linode, err = getLinodeByLabel(linodeClient, linodeInfo)
		if err != nil {
			// check for IBM cloud Satellite environment
			glog.Warningf("Unable to get the linodeID by label [%s] Getting linode for IBM cloud satellite", linodeInfo)
			linode, err = getLinodeIDforSatellite(linodeClient, linodeInfo)
			if err != nil {
				return nil, err
			}
		}
	}

	return &metadataServiceManager{
		region:  linode.Region,
		nodeID:  linode.ID,
		label:   linode.Label,
		project: linode.Group,
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
		return nil, err
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

func getLinodeIDforSatellite(client linodeclient.LinodeClient, nodeName string) (*linodego.Instance, error) {

	// Get the provider ID for ginen node
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Error loading in-cluster config: %v\n", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error creating Kubernetes client: %v\n", err)
	}

	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error listing pods: %v\n", err)
	}

	providerID := node.Spec.ProviderID

	//Get the linodeID for IBM satellite only if the providerID starts with ibm://

	if strings.HasPrefix(providerID, providerIDPrefixIBM) {

		//The node name for IBM cloud satelllte is in this formant --> 192-168-78-34
		//Convert the name to IPv4 and find the linode instance by IPv4

		ipaddress := strings.ReplaceAll(nodeName, "-", ".")
		instances, err := client.ListInstances(context.Background(), &linodego.ListOptions{Filter: "{\"ipv4\": \"" + ipaddress + "\"}"})
		if err != nil {
			return nil, fmt.Errorf("Error getting instances: %s", err.Error())
		}
		if len(instances) != 1 {
			return nil, fmt.Errorf("Error finding a single instance with IP address %s. Found %d instances", ipaddress, len(instances))
		}
		return &instances[0], nil
	} else {
		return nil, fmt.Errorf("Not a IBM Cloud Satellite")
	}
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
