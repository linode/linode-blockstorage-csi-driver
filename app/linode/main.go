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
	"fmt"
	"math/rand"
	"os"
	"time"

	"flag"

	"github.com/golang/glog"
	driver "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-bs"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	metadataservice "github.com/linode/linode-blockstorage-csi-driver/pkg/metadata"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

const (
	driverName = "linodebs.csi.linode.com"
)

var (
	vendorVersion string
	endpoint      = flag.String("endpoint", "unix:/tmp/csi.sock", "CSI endpoint")
	token         = flag.String("token", "", "Linode API Token")
	url           = flag.String("url", "", "Linode API URL")
	node          = flag.String("node", "", "Linode Hostname")
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	handle()
	os.Exit(0)
}

func handle() {
	if vendorVersion == "" {
		glog.Fatalf("vendorVersion must be set at compile time")
	}
	glog.V(4).Infof("Driver vendor version %v", vendorVersion)

	linodeDriver := driver.GetLinodeDriver()

	//Initialize Linode Driver (Move setup to main?)
	uaPrefix := fmt.Sprintf("LinodeCSI/%s", vendorVersion)
	cloudProvider := linodeclient.NewLinodeClient(*token, uaPrefix, *url)

	mounter := mountmanager.NewSafeMounter()
	deviceUtils := mountmanager.NewDeviceUtils()

	ms, err := metadataservice.NewMetadataService(cloudProvider, *node)
	if err != nil {
		glog.Fatalf("Failed to set up metadata service: %v", err)
	}

	err = linodeDriver.SetupLinodeDriver(cloudProvider, mounter, deviceUtils, ms, driverName, vendorVersion)
	if err != nil {
		glog.Fatalf("Failed to initialize Linode CSI Driver: %v", err)
	}

	linodeDriver.Run(*endpoint)
}
