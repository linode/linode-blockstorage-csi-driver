package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	metadata "github.com/linode/go-metadata"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"github.com/linode/linodego"
)

// Metadata contains metadata about the node/instance the CSI node plugin
// is running on.
type Metadata struct {
	ID     int    // Instance ID.
	Label  string // The label assigned to the instance.
	Region string // Region the instance is running in.
	Memory uint   // Amount of memory the instance has, in bytes.
}

// GetMetadata retrieves information about the current node/instance from the
// Linode Metadata Service. If the Metadata Service is unavailable, or this
// function otherwise returns a non-nil error, callers should call
// [GetMetadataFromAPI].
func GetMetadata(ctx context.Context) (Metadata, error) {
	log := logger.GetLogger(ctx)

	log.V(2).Info("Processing request")

	log.V(4).Info("Creating new metadata client")
	client, err := metadata.NewClient(ctx)
	if err != nil {
		log.Error(err, "Failed to create new metadata client")
		return Metadata{}, fmt.Errorf("new metadata client: %w", err)
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

	metadata := Metadata{
		ID:     data.ID,
		Label:  data.Label,
		Region: data.Region,
		Memory: memoryToBytes(data.Specs.Memory),
	}

	log.V(2).Info("Successfully completed")
	return metadata, nil
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
func GetMetadataFromAPI(ctx context.Context, client *linodego.Client) (Metadata, error) {
	log, ctx, done := logger.GetLogger(ctx).WithMethod("GetMetadataFromAPI")
	defer done()

	log.V(2).Info("Processing request")

	if client == nil {
		return Metadata{}, errNilClient
	}

	log.V(4).Info("Checking LinodeIDPath", "path", LinodeIDPath)
	if _, err := os.Stat(LinodeIDPath); err != nil {
		return Metadata{}, fmt.Errorf("stat %s: %w", LinodeIDPath, err)
	}

	log.V(4).Info("Opening LinodeIDPath", "path", LinodeIDPath)
	f, err := os.Open(LinodeIDPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	log.V(4).Info("Reading LinodeID from file")
	// Read in the file, but use a LimitReader to make sure we are not
	// reading in junk.
	data, err := io.ReadAll(io.LimitReader(f, 1<<10))
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

	metadata := Metadata{
		ID:     linodeID,
		Label:  instance.Label,
		Region: instance.Region,
		Memory: memory,
	}

	log.V(4).Info("Successfully retrieved metadata", 
		"instanceID", metadata.ID,
		"instanceLabel", metadata.Label,
		"region", metadata.Region,
		"memory", metadata.Memory)

	log.V(2).Info("Successfully completed")
	return metadata, nil
}
