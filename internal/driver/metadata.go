package driver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	metadata "github.com/linode/go-metadata"
	"github.com/linode/linodego"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

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

type KubeClient interface {
	GetNode(ctx context.Context, name string) (*corev1.Node, error)
}

type kubeClient struct {
	client kubernetes.Interface
}

func (k *kubeClient) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	return k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

var newKubeClient = func(ctx context.Context) (KubeClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &kubeClient{client: client}, nil
}

// GetNodeMetadata retrieves metadata about the current node/instance.
func GetNodeMetadata(ctx context.Context, cloudProvider linodeclient.LinodeClient, nodeName string, fileSystem filesystem.FileSystem) (Metadata, error) {
	log := logger.GetLogger(ctx)
	var instance *linodego.Instance

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

	// New Step 3: Try product_serial file
	log.V(4).Info("Attempting to read metadata from product_serial file")
	linodeID, err := getLinodeIDFromProductSerial(ctx, fileSystem)
	if err == nil {
		instance, err = cloudProvider.GetInstance(ctx, linodeID)
		if err != nil {
			log.Error(err, "Failed to get instance details from cloud provider")
		} else {
			nodeMetadata = Metadata{
				ID:     linodeID,
				Label:  instance.Label,
				Region: instance.Region,
				Memory: memoryToBytes(instance.Specs.Memory),
			}

			log.V(4).Info("Successfully obtained node metadata from product_serial",
				"ID", nodeMetadata.ID,
				"Label", nodeMetadata.Label,
				"Region", nodeMetadata.Region,
				"Memory", nodeMetadata.Memory,
			)
			return nodeMetadata, nil
		}
	} else {
		log.V(4).Info("Product_serial fallback failed", "error", err.Error())
	}

	// Step 4: Fall back to Kubernetes API
	log.V(4).Info("Attempting to get metadata from Kubernetes API")
	if nodeName == "" {
		return Metadata{}, fmt.Errorf("NODE_NAME environment variable not set")
	}

	// Replace the direct k8s client creation with the new interface
	k8sClient, err := newKubeClient(ctx)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Get node information using the interface
	node, err := k8sClient.GetNode(ctx, nodeName)
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

	linodeID, err = strconv.Atoi(strings.TrimPrefix(providerID, "linode://"))
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to parse Linode ID from provider ID: %w", err)
	}

	instance, err = cloudProvider.GetInstance(ctx, linodeID)
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

var errNilClient = errors.New("nil client")

// Add this new helper function
func getLinodeIDFromProductSerial(ctx context.Context, fs filesystem.FileSystem) (int, error) {
	log := logger.GetLogger(ctx)

	filePath := "/sys/devices/virtual/dmi/id/product_serial"
	file, err := fs.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open product_serial file: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Error(closeErr, "Failed to close product_serial file")
		}
	}()

	buf := make([]byte, 64)
	n, err := file.Read(buf)
	if err != nil {
		return 0, fmt.Errorf("failed to read product_serial file: %w", err)
	}

	idStr := strings.TrimSpace(string(buf[:n]))
	linodeID, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("invalid linode ID format in product_serial: %w", err)
	}

	log.V(4).Info("Successfully parsed Linode ID from product_serial", "linodeID", linodeID)
	return linodeID, nil
}
