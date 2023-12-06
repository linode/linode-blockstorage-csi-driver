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
	"errors"
	"flag"
	"fmt"
	"os"

	driver "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-bs"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	metadataservice "github.com/linode/linode-blockstorage-csi-driver/pkg/metadata"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"k8s.io/klog/v2"
)

const (
	driverName = "linodebs.csi.linode.com"
)

var (
	vendorVersion string
	endpoint      = flag.String("endpoint", "unix:/tmp/csi.sock", "CSI endpoint")
	token         = flag.String("token", "", "Linode API Token")
	url           = flag.String("url", "", "Linode API URL")
	node          = flag.String("node", "", "Node name")
	bsPrefix      = flag.String("bs-prefix", "", "Linode BlockStorage Volume label prefix")
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()
	if err := handle(); err != nil {
		klog.Fatal(err)
	}
	os.Exit(0)
}

func handle() error {
	if vendorVersion == "" {
		return errors.New("vendorVersion must be set at compile time")
	}
	if *token == "" {
		return errors.New("linode token required")
	}
	klog.V(4).Infof("Driver vendor version %v", vendorVersion)

	linodeDriver := driver.GetLinodeDriver()

	//Initialize Linode Driver (Move setup to main?)
	uaPrefix := fmt.Sprintf("LinodeCSI/%s", vendorVersion)
	cloudProvider, err := linodeclient.NewLinodeClient(*token, uaPrefix, *url)
	if err != nil {
		return fmt.Errorf("failed to set up linode client: %s", err)
	}

	mounter := mountmanager.NewSafeMounter()
	deviceUtils := mountmanager.NewDeviceUtils()

	ms, err := metadataservice.NewMetadataService(cloudProvider, *node)
	if err != nil {
		return fmt.Errorf("failed to set up metadata service: %v", err)
	}

	prefix := ""
	if bsPrefix != nil {
		prefix = *bsPrefix
	}

	if err := linodeDriver.SetupLinodeDriver(
		cloudProvider, mounter, deviceUtils, ms, driverName, vendorVersion, prefix); err != nil {
		return fmt.Errorf("failed to initialize Linode CSI Driver: %v", err)
	}

	linodeDriver.Run(*endpoint)
	return nil
}
