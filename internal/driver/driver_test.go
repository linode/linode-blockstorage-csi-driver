package driver

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	drivertest "github.com/linode/linode-blockstorage-csi-driver/pkg/driver-test"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"k8s.io/mount-utils"

	"github.com/linode/linodego"

	"go.uber.org/mock/gomock"
)

var (
	driver        = "linodebs.csi.linode.com"
	vendorVersion = "test-vendor"
)

func TestDriverSuite(t *testing.T) {
	socket := "/tmp/csi.sock"
	endpoint := "unix://" + socket
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove unix domain socket file %s, error: %s", socket, err)
	}

	bsPrefix := "test-"

	// mock Linode Server, not working yet ...
	fake := &drivertest.FakeAPI{
		T:       t,
		Volumes: map[string]linodego.Volume{},
		Instance: &linodego.Instance{
			Label:      "linode123",
			Region:     "us-east",
			Image:      "linode/debian9",
			Type:       "g6-standard-2",
			Group:      "Linode-Group",
			ID:         123,
			Status:     "running",
			Hypervisor: "kvm",
		},
	}

	ts := httptest.NewServer(fake)
	defer ts.Close()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mounter := &mount.SafeFormatAndMount{
		Interface: mocks.NewMockMounter(mockCtrl), 
		Exec: mocks.NewMockExecutor(mockCtrl),
	}
	deviceUtils := mocks.NewMockDeviceUtils(mockCtrl)
	fileSystem := mocks.NewMockFileSystem(mockCtrl)
	encrypt := NewLuksEncryption(mounter.Exec, fileSystem)

	fakeCloudProvider, err := linodeclient.NewLinodeClient("dummy", fmt.Sprintf("LinodeCSI/%s", vendorVersion), ts.URL)
	if err != nil {
		t.Fatalf("Failed to setup Linode client: %s", err)
	}

	// TODO fake metadata
	md := Metadata{
		ID:     123,
		Label:  fake.Instance.Label,
		Region: fake.Instance.Region,
		Memory: 4 << 30, // 4GiB
	}
	linodeDriver := GetLinodeDriver()
	if err := linodeDriver.SetupLinodeDriver(fakeCloudProvider, mounter, deviceUtils, md, driver, vendorVersion, bsPrefix, encrypt); err != nil {
		t.Fatalf("Failed to setup Linode Driver: %v", err)
	}

	go linodeDriver.Run(context.Background(), endpoint)

	// TODO: fix sanity checks for e2e, disable for ci
	// cfg := sanity.NewTestConfig()
	// cfg.Address = endpoint
	// sanity.Test(t, cfg)
}

