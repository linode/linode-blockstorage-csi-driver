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
	"k8s.io/mount-utils"

	"github.com/linode/linode-blockstorage-csi-driver/internal/driver"
	cryptsetupclient "github.com/linode/linode-blockstorage-csi-driver/pkg/cryptsetup-client"
	devicemanager "github.com/linode/linode-blockstorage-csi-driver/pkg/device-manager"
	filesystem "github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
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
	// Deprecated: This is not needed as the CSI driver now uses the Linode
	// Metadata Service to source information about the current
	// node/instance. It will be removed in a future change.
	nodeName string

	// Flag to enable metrics
	enableMetrics string

	// Flag to specify the port on which the metrics http server will run
	metricsPort string

	// Flag to enable tracing
	enableTracing string

	// Flag to specify the port on which the tracing http server will run
	tracingPort string
}

func loadConfig() configuration {
	var cfg configuration
	envflag.StringVar(&cfg.csiEndpoint, "CSI_ENDPOINT", "unix:/tmp/csi.sock", "Path to the CSI endpoint socket")
	envflag.StringVar(&cfg.linodeToken, "LINODE_TOKEN", "", "Linode API token")
	envflag.StringVar(&cfg.linodeURL, "LINODE_URL", linodego.APIHost, "Linode API URL")
	envflag.StringVar(&cfg.volumeLabelPrefix, "LINODE_VOLUME_LABEL_PREFIX", "", "Linode Block Storage volume label prefix")
	envflag.StringVar(&cfg.nodeName, "NODE_NAME", "", "Name of the current node") // deprecated
	envflag.StringVar(&cfg.enableMetrics, "ENABLE_METRICS", "", "This flag conditionally runs the metrics servers")
	envflag.StringVar(&cfg.metricsPort, "METRICS_PORT", "8081", "This flag specifies the port on which the metrics https server will run")
	envflag.StringVar(&cfg.enableTracing, "OTEL_TRACING", "", "This flag conditionally enables tracing")
	envflag.StringVar(&cfg.tracingPort, "OTEL_TRACING_PORT", "4318", "This flag specifies the port on which the tracing https server will run")
	envflag.Parse()
	return cfg
}

func main() {
	// Create a base context with the logger
	ctx := context.Background()
	log := logger.NewLogger(ctx)
	ctx = context.WithValue(ctx, logger.LoggerKey{}, log)

	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		log.Error(err, "Fatal error")
		os.Exit(0)
	}
	flag.Parse()
	maxProcs(log)

	if err := handle(ctx); err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}

	os.Exit(0)
}

func maxProcs(log *logger.Logger) {
	undoMaxprocs, maxprocsError := maxprocs.Set(maxprocs.Logger(func(msg string, keysAndValues ...interface{}) {
		log.Klogr.WithValues("component", "maxprocs", "version", maxprocs.Version).V(2).Info(fmt.Sprintf(msg, keysAndValues...))
	}))
	defer undoMaxprocs()
	if maxprocsError != nil {
		log.Error(maxprocsError, "Failed to set GOMAXPROCS")
	}
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
		return fmt.Errorf("failed to set up linode client: %w", err)
	}

	mounter := mountmanager.NewSafeMounter()
	fileSystem := filesystem.NewFileSystem()
	deviceUtils := devicemanager.NewDeviceUtils(fileSystem, mounter.Exec)
	cryptSetup := cryptsetupclient.NewCryptSetup()
	encrypt := driver.NewLuksEncryption(mounter.Exec, fileSystem, cryptSetup)
	resizer := mount.NewResizeFs(mounter.Exec)

	nodeMetadata, err := driver.GetNodeMetadata(ctx, cloudProvider, cfg.nodeName, fileSystem)
	if err != nil {
		return fmt.Errorf("failed to get node metadata: %w", err)
	}

	if err := linodeDriver.SetupLinodeDriver(
		ctx,
		cloudProvider,
		mounter,
		deviceUtils,
		resizer,
		nodeMetadata,
		driver.Name,
		vendorVersion,
		cfg.volumeLabelPrefix,
		encrypt,
		cfg.enableMetrics,
		cfg.metricsPort,
		cfg.enableTracing,
		cfg.tracingPort,
	); err != nil {
		return fmt.Errorf("setup driver: %w", err)
	}

	linodeDriver.Run(ctx, cfg.csiEndpoint)
	return nil
}
