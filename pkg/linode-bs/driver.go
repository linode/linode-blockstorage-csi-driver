package driver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"net/http"

	"encoding/json"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/linode/linodego"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
)

const (
	driverName    = "linodebs.csi.linode.com"
	vendorVersion = "0.0.1"
)

type Driver struct {
	*csicommon.DefaultNodeServer
	*csicommon.DefaultIdentityServer
	*csicommon.DefaultControllerServer

	endpoint string
	nodeID   string
	region   string

	srv          *grpc.Server
	linodeClient *linodego.Client
	mounter      Mounter
	log          *logrus.Entry
}

func NewDriver(ep, token, zone, host string, url *string) (*Driver, error) {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token,
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	ua := fmt.Sprintf("LinodeCSI/%s linodego/%s", vendorVersion, linodego.Version)
	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetUserAgent(ua)
	linodeClient.SetDebug(true)

	if url != nil {
		linodeClient.SetBaseURL(*url)
	}

	linode, err := getLinodeByName(&linodeClient, host)
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

func (d *Driver) Run() error {
	u, err := url.Parse(d.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %q", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		addr = filepath.FromSlash(u.Path)
	}

	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have: %s", u.Scheme)
	}

	// clear stale socket files (especially for upgrades)
	d.log.WithField("socket", addr).Info("removing socket")
	if err = os.Remove(addr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %s", addr, err)
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

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

func (d *Driver) Stop() {
	d.log.Info("server stopped")
	d.srv.Stop()
}

func getLinodeByName(client *linodego.Client, nodeName string) (*linodego.Instance, error) {
	jsonFilter, _ := json.Marshal(map[string]string{"label": nodeName})
	linodes, err := client.ListInstances(context.Background(), linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		return nil, err
	} else if len(linodes) != 1 {
		return nil, fmt.Errorf("Could not identify a Linode ID with label %q", nodeName)
	}

	for _, linode := range linodes {
		if linode.Label == string(nodeName) {
			return &linode, nil
		}
	}
	return nil, errors.New("instance not found")
}
