/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/ianschenck/envflag"
	"github.com/linode/linodego"
	"go.uber.org/automaxprocs/maxprocs"
	"k8s.io/klog/v2"

	"github.com/linode/linode-blockstorage-csi-driver/internal/driver"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

var vendorVersion string // set by the linker

type configuration struct {
	// The UNIX socket to listen on for RPC requests.
	csiEndpoint string

	// Linode personal access token, used to make requests to the Linode
	// API.
	linodeToken string

	// Linode API URL.
	linodeURL string

	// Optional label prefix to use when creating new Linode Block Storage
	// Volumes.
	volumeLabelPrefix string

	// Name of the current node, when running as the node plugin.
	//
	// deprecated: This is not needed as the CSI driver now uses the Linode
	// Metadata Service to source information about the current
	// node/instance. It will be removed in a future change.
	nodeName string
}

func loadConfig() configuration {
	var cfg configuration
	envflag.StringVar(&cfg.csiEndpoint, "CSI_ENDPOINT", "unix:/tmp/csi.sock", "Path to the CSI endpoint socket")
	envflag.StringVar(&cfg.linodeToken, "LINODE_TOKEN", "", "Linode API token")
	envflag.StringVar(&cfg.linodeURL, "LINODE_URL", linodego.APIHost, "Linode API URL")
	envflag.StringVar(&cfg.volumeLabelPrefix, "LINODE_VOLUME_LABEL_PREFIX", "", "Linode Block Storage volume label prefix")
	envflag.StringVar(&cfg.nodeName, "NODE_NAME", "", "Name of the current node") // deprecated
	envflag.Parse()
	return cfg
}

func main() {
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	flag.Parse()

	// Create a base context with the logger
	ctx := context.Background()
	log := logger.NewLogger(ctx)
	ctx = context.WithValue(ctx, logger.LoggerKey{}, log)
	undoMaxprocs, maxprocsError := maxprocs.Set(maxprocs.Logger(func(msg string, keysAndValues ...interface{}) {
		log.Klogr.WithValues("component", "maxprocs", "version", maxprocs.Version).V(2).Info(fmt.Sprintf(msg, keysAndValues...))
	}))
	defer undoMaxprocs()

	if maxprocsError != nil {
		log.Error(maxprocsError, "Failed to set GOMAXPROCS")
	}

	if err := handle(ctx); err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}

	os.Exit(0)
}

func handle(ctx context.Context) error {
	log := logger.GetLogger(ctx)

	if vendorVersion == "" {
		return errors.New("vendorVersion must be set at compile time")
	}
	log.V(4).Info("Driver vendor version", "version", vendorVersion)

	cfg := loadConfig()
	if cfg.linodeToken == "" {
		return errors.New("linode token required")
	}

	linodeDriver := driver.GetLinodeDriver(ctx)

	// Initialize Linode Driver (Move setup to main?)
	uaPrefix := fmt.Sprintf("LinodeCSI/%s", vendorVersion)
	cloudProvider, err := linodeclient.NewLinodeClient(cfg.linodeToken, uaPrefix, cfg.linodeURL)
	if err != nil {
		return fmt.Errorf("failed to set up linode client: %s", err)
	}

	mounter := mountmanager.NewSafeMounter()
	deviceUtils := mountmanager.NewDeviceUtils()
	fileSystem := mountmanager.NewFileSystem()
	cryptSetup := driver.NewCryptSetup()
	encrypt := driver.NewLuksEncryption(mounter.Exec, fileSystem, cryptSetup)

	metadata, err := driver.GetMetadata(ctx)
	if err != nil {
		log.Error(err, "Metadata service not available, falling back to API")
		if metadata, err = driver.GetMetadataFromAPI(ctx, cloudProvider); err != nil {
			return fmt.Errorf("get metadata from api: %w", err)
		}
	}

	if err := linodeDriver.SetupLinodeDriver(
		ctx,
		cloudProvider,
		mounter,
		deviceUtils,
		metadata,
		driver.Name,
		vendorVersion,
		cfg.volumeLabelPrefix,
		encrypt,
	); err != nil {
		return fmt.Errorf("setup driver: %v", err)
	}

	linodeDriver.Run(ctx, cfg.csiEndpoint)
	return nil
}
