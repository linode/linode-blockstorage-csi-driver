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
	nodeID   string
	region   string

	srv          *grpc.Server
	linodeClient *linodego.Client
	mounter      Mounter
	log          *logrus.Entry
}

// NewDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managaing Linode Block Storage
func NewDriver(ep, token, zone, host string, url *string) (*Driver, error) {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token,
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	ua := fmt.Sprintf("LinodeCSI/%s (linodego %s)", vendorVersion, linodego.Version)
	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetUserAgent(ua)
	linodeClient.SetDebug(true)

	if url != nil {
		linodeClient.SetBaseURL(*url)
	}

	linode, err := getlinodeByName(&linodeClient, host)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize Linode client: %s", err)
	}

	nodeID := strconv.Itoa(linode.ID)
	return &Driver{
		endpoint:     ep,
		nodeID:       nodeID,
		region:       zone,
		linodeClient: &linodeClient,
		mounter:      &mounter{},
		log: logrus.New().WithFields(logrus.Fields{
			"region":  zone,
			"node_id": nodeID,
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
	}

	// remove the socket if it's already there. This can happen if we
	// deploy a new version and the socket was created from the old running
	// plugin.
	d.log.WithField("socket", addr).Info("removing socket")
	if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %s", addr, err)
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

func getlinodeByName(client *linodego.Client, nodeName string) (*linodego.Instance, error) {
	jsonFilter, _ := json.Marshal(map[string]string{"label": nodeName})
	linodes, err := client.ListInstances(context.Background(), linodego.NewListOptions(0, string(jsonFilter)))
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
