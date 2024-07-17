package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	metadata "github.com/linode/go-metadata"
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
	client, err := metadata.NewClient(ctx)
	if err != nil {
		return Metadata{}, fmt.Errorf("new metadata client: %w", err)
	}

	data, err := client.GetInstance(ctx)
	if err != nil {
		return Metadata{}, fmt.Errorf("get instance data: %w", err)
	}

	return Metadata{
		ID:     data.ID,
		Label:  data.Label,
		Region: data.Region,
		Memory: memoryToBytes(data.Specs.Memory),
	}, nil
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
	if client == nil {
		return Metadata{}, errNilClient
	}

	if _, err := os.Stat(LinodeIDPath); err != nil {
		return Metadata{}, fmt.Errorf("stat %s: %w", LinodeIDPath, err)
	}
	f, err := os.Open(LinodeIDPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// Read in the file, but use a LimitReader to make sure we are not
	// reading in junk.
	data, err := io.ReadAll(io.LimitReader(f, 1<<10))
	if err != nil {
		return Metadata{}, fmt.Errorf("read all: %w", err)
	}

	linodeID, err := strconv.Atoi(string(data))
	if err != nil {
		return Metadata{}, fmt.Errorf("atoi: %w", err)
	}

	instance, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		return Metadata{}, fmt.Errorf("get instance: %w", err)
	}

	memory := minMemory
	if instance.Specs != nil {
		memory = memoryToBytes(instance.Specs.Memory)
	}

	return Metadata{
		ID:     linodeID,
		Label:  instance.Label,
		Region: instance.Region,
		Memory: memory,
	}, nil
}
