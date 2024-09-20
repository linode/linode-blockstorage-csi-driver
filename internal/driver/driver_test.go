package driver

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	"k8s.io/mount-utils"

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

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mounter := &mount.SafeFormatAndMount{
		Interface: mocks.NewMockMounter(mockCtrl),
		Exec:      mocks.NewMockExecutor(mockCtrl),
	}
	deviceUtils := mocks.NewMockDeviceUtils(mockCtrl)
	fileSystem := mocks.NewMockFileSystem(mockCtrl)
	cryptSetup := mocks.NewMockCryptSetupClient(mockCtrl)
	encrypt := NewLuksEncryption(mounter.Exec, fileSystem, cryptSetup)

	fakeCloudProvider, err := linodeclient.NewLinodeClient("dummy", fmt.Sprintf("LinodeCSI/%s", vendorVersion), "")
	if err != nil {
		t.Fatalf("Failed to setup Linode client: %s", err)
	}

	// TODO fake metadata
	md := Metadata{
		ID:     123,
		Label:  "linode123",
		Region: "us-east",
		Memory: 4 << 30, // 4GiB
	}
	linodeDriver := GetLinodeDriver(context.Background())
	if err := linodeDriver.SetupLinodeDriver(context.Background(), fakeCloudProvider, mounter, deviceUtils, md, driver, vendorVersion, bsPrefix, encrypt); err != nil {
		t.Fatalf("Failed to setup Linode Driver: %v", err)
	}

	go linodeDriver.Run(context.Background(), endpoint)

	// TODO: fix sanity checks for e2e, disable for ci
	// cfg := sanity.NewTestConfig()
	// cfg.Address = endpoint
	// sanity.Test(t, cfg)
}
