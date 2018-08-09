/*
Copyright 2018 Digital Ocean

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

package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"net/http"

	"encoding/json"
	"strconv"

	"github.com/chiefy/linodego"
	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
)

const (
	driverName    = "com.linode.csi.linodebs"
	vendorVersion = "0.0.1"
)

// Driver implements the following CSI interfaces:
//
//   csi.IdentityServer
//   csi.ControllerServer
//   csi.NodeServer
//
type Driver struct {
	endpoint string
	nodeId   string
	region   string

	srv          *grpc.Server
	linodeClient *linodego.Client
	mounter      Mounter
	log          *logrus.Entry
}

// NewDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managaing Linode Block Storage
func NewDriver(ep, token, zone, host string) (*Driver, error) {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token,
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetDebug(true)

	linode, err := getlinodeByName(&linodeClient, host)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize Linode client: %s", err)
	}

	nodeId := strconv.Itoa(linode.ID)
	return &Driver{
		endpoint:     ep,
		nodeId:       nodeId,
		region:       zone,
		linodeClient: &linodeClient,
		mounter:      &mounter{},
		log: logrus.New().WithFields(logrus.Fields{
			"region":  zone,
			"node_id": nodeId,
		}),
	}, nil
}

// Run starts the CSI plugin by communication over the given endpoint
func (d *Driver) Run() error {
	u, err := url.Parse(d.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %q", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		addr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have: %s", u.Scheme)
	} else {
		// remove the socket if it's already there. This can happen if we
		// deploy a new version and the socket was created from the old running
		// plugin.
		d.log.WithField("socket", addr).Info("removing socket")
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove unix domain socket file %s, error: %s", addr, err)
		}
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			d.log.WithError(err).WithField("method", info.FullMethod).Error("method failed")
		}
		return resp, err
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	d.log.WithField("addr", addr).Info("server started")
	return d.srv.Serve(listener)
}

// Stop stops the plugin
func (d *Driver) Stop() {
	d.log.Info("server stopped")
	d.srv.Stop()
}

// getMacAddr gets the local MAC addresses
// https://stackoverflow.com/a/44859459/156674
func getMacAddr() ([]string, error) {
	ifas, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	return as, nil
}

func getlinodeByName(client *linodego.Client, nodeName string) (*linodego.Instance, error) {
	jsonFilter, _ := json.Marshal(map[string]string{"label": nodeName})
	linodes, err := client.ListInstances(context.TODO(), linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, err
	} else if len(linodes) != 1 {
		return nil, fmt.Errorf("Could not determine Linode instance ID from Linode label %s", nodeName)
	}

	for _, linode := range linodes {
		if linode.Label == string(nodeName) {
			return linode, nil
		}
	}
	return nil, InstanceNotFound
}
