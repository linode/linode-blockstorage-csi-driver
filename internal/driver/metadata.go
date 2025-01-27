package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	metadata "github.com/linode/go-metadata"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
)

// Metadata contains metadata about the node/instance the CSI node plugin
// is running on.
type Metadata struct {
	ID     int    // Instance ID.
	Label  string // The label assigned to the instance.
	Region string // Region the instance is running in.
	Memory uint   // Amount of memory the instance has, in bytes.
}

type MetadataClient interface {
	GetInstance(ctx context.Context) (*metadata.InstanceData, error)
}

var NewMetadataClient = func(ctx context.Context) (MetadataClient, error) {
	return metadata.NewClient(ctx)
}

// GetNodeMetadata retrieves metadata about the current node/instance.
func GetNodeMetadata(ctx context.Context, cloudProvider linodeclient.LinodeClient, fileSystem filesystem.FileSystem) (Metadata, error) {
	log := logger.GetLogger(ctx)

	// Step 1: Attempt to create the metadata client
	log.V(4).Info("Attempting to create metadata client")
	linodeMetadataClient, err := NewMetadataClient(ctx)
	if err != nil {
		log.Error(err, "Failed to create metadata client")
		linodeMetadataClient = nil
	}

	// Step 2: Try to get metadata from metadata service
	var nodeMetadata Metadata
	if linodeMetadataClient != nil {
		log.V(4).Info("Attempting to get metadata from metadata service")
		nodeMetadata, err = GetMetadata(ctx, linodeMetadataClient)
		if err != nil {
			log.Error(err, "Failed to get metadata from metadata service")
		}
	}
	if nodeMetadata.ID != 0 {
		return nodeMetadata, nil
	}

	// Step 3: Fall back to Kubernetes API
	log.V(4).Info("Attempting to get metadata from Kubernetes API")
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		return Metadata{}, fmt.Errorf("NODE_NAME environment variable not set")
	}

	// Create kubernetes client using in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to get cluster config: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Get node information
	node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	providerID := node.Spec.ProviderID
	if providerID == "" {
		return Metadata{}, fmt.Errorf("provider ID not found for node %s", nodeName)
	}

	// Extract Linode ID from provider ID (format: linode://12345)
	if !strings.HasPrefix(providerID, "linode://") {
		return Metadata{}, fmt.Errorf("invalid provider ID format: %s", providerID)
	}

	linodeID, err := strconv.Atoi(strings.TrimPrefix(providerID, "linode://"))
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to parse Linode ID from provider ID: %w", err)
	}

	instance, err := cloudProvider.GetInstance(ctx, linodeID)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to get instance details: %w", err)
	}

	nodeMetadata = Metadata{
		ID:     linodeID,
		Label:  instance.Label,
		Region: instance.Region,
		Memory: memoryToBytes(instance.Specs.Memory),
	}

	log.V(4).Info("Successfully obtained node metadata",
		"ID", nodeMetadata.ID,
		"Label", nodeMetadata.Label,
		"Region", nodeMetadata.Region,
		"Memory", nodeMetadata.Memory,
	)

	return nodeMetadata, nil
}

// GetMetadata retrieves information about the current node/instance from the
// Linode Metadata Service. If the Metadata Service is unavailable, or this
// function otherwise returns a non-nil error, callers should call
// [GetMetadataFromAPI].
func GetMetadata(ctx context.Context, client MetadataClient) (Metadata, error) {
	log := logger.GetLogger(ctx)

	log.V(2).Info("Processing request")
	if client == nil {
		return Metadata{}, errNilClient
	}

	log.V(4).Info("Retrieving instance data from metadata service")
	data, err := client.GetInstance(ctx)
	if err != nil {
		log.Error(err, "Failed to get instance data from metadata service")
		return Metadata{}, fmt.Errorf("get instance data: %w", err)
	}

	log.V(4).Info("Successfully retrieved metadata",
		"instanceID", data.ID,
		"instanceLabel", data.Label,
		"region", data.Region,
		"memory", data.Specs.Memory)

	nodeMetadata := Metadata{
		ID:     data.ID,
		Label:  data.Label,
		Region: data.Region,
		Memory: memoryToBytes(data.Specs.Memory),
	}

	log.V(2).Info("Successfully completed")
	return nodeMetadata, nil
}

// memoryToBytes converts the given amount of memory in MB, to bytes.
// If sizeMB is less than 1024, [minMemory] is returned as it is the smallest
// amount of memory available on any Linode instance type.
func memoryToBytes(sizeMB int) uint {
	if sizeMB < 1<<10 {
		return minMemory
	}
	return uint(sizeMB) << 20
}

// minMemory is the smallest amount of memory, in bytes, available on any
// Linode instance type.
const minMemory uint = 1 << 30

// LinodeIDPath is the path to a file containing only the ID of the Linode
// instance the CSI node plugin is currently running on.
// This file is expected to be placed into the Linode by the init container
// provided with the CSI node plugin.
const LinodeIDPath = "/linode-info/linode-id"

var errNilClient = errors.New("nil client")

// GetMetadataFromAPI attempts to retrieve metadata about the current
// node/instance directly from the Linode API.
func GetMetadataFromAPI(ctx context.Context, client linodeclient.LinodeClient, fs filesystem.FileSystem) (Metadata, error) {
	log, _, done := logger.GetLogger(ctx).WithMethod("GetMetadataFromAPI")
	defer done()

	log.V(2).Info("Processing request")

	if client == nil {
		return Metadata{}, errNilClient
	}

	log.V(4).Info("Checking LinodeIDPath", "path", LinodeIDPath)
	if _, err := fs.Stat(LinodeIDPath); err != nil {
		return Metadata{}, fmt.Errorf("stat %s: %w", LinodeIDPath, err)
	}

	log.V(4).Info("Opening LinodeIDPath", "path", LinodeIDPath)
	fileObj, err := fs.Open(LinodeIDPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("open: %w", err)
	}
	defer func() {
		err = fileObj.Close()
	}()

	log.V(4).Info("Reading LinodeID from file")
	// Read in the file, but use a LimitReader to make sure we are not
	// reading in junk.
	data, err := io.ReadAll(io.LimitReader(fileObj, 1<<10))
	if err != nil {
		return Metadata{}, fmt.Errorf("read all: %w", err)
	}

	log.V(4).Info("Parsing LinodeID")
	linodeID, err := strconv.Atoi(string(data))
	if err != nil {
		return Metadata{}, fmt.Errorf("atoi: %w", err)
	}

	log.V(4).Info("Retrieving instance data from API", "linodeID", linodeID)
	instance, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		return Metadata{}, fmt.Errorf("get instance: %w", err)
	}

	memory := minMemory
	if instance.Specs != nil {
		memory = memoryToBytes(instance.Specs.Memory)
	}

	nodeMetadata := Metadata{
		ID:     linodeID,
		Label:  instance.Label,
		Region: instance.Region,
		Memory: memory,
	}

	log.V(4).Info("Successfully retrieved metadata",
		"instanceID", nodeMetadata.ID,
		"instanceLabel", nodeMetadata.Label,
		"region", nodeMetadata.Region,
		"memory", nodeMetadata.Memory)

	log.V(2).Info("Successfully completed")
	return nodeMetadata, nil
}
