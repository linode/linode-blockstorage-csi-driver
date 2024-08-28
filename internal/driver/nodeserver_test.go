//go:build linux && elevated
// +build linux,elevated

package driver

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

func newSafeMounter() *mount.SafeFormatAndMount {
	realMounter := mount.New("")
	realExec := exec.New()
	return &mount.SafeFormatAndMount{
		Interface: realMounter,
		Exec:      realExec,
	}
}

var (
	defaultNodeServer = NodeServer{mounter: mountmanager.NewSafeMounter()}

	defaultTeardownFunc = func(t *testing.T, mount string) {
		_, err := os.Stat(mount)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}

			t.Errorf("failed to stat the '%s': %v", mount, err)
			return
		}

		// best effort call, no need to check error
		_ = defaultNodeServer.mounter.Unmount(mount)

		err = os.RemoveAll(path.Dir(mount))
		if err != nil {
			t.Errorf("failed to remove the '%s': %v", path.Dir(mount), err)
		}
	}

	defaultPrepareFunc = func() (string, func(*testing.T, string), error) {
		root, err := os.MkdirTemp("", "")
		if err != nil {
			return "", nil, fmt.Errorf("mkdir temp failed: %w", err)
		}

		source := path.Join(root, "source")
		target := path.Join(root, "target")

		err = os.Mkdir(source, 0o755)
		if err != nil {
			return "", nil, fmt.Errorf("mkdir '%s' failed: %w", source, err)
		}

		err = os.Mkdir(target, 0o755)
		if err != nil {
			return "", nil, fmt.Errorf("mkdir '%s' failed: %w", target, err)
		}

		defaultNodeServer.mounter.Mount(source, target, "ext4", []string{"bind"})

		return target, defaultTeardownFunc, nil
	}
)

func TestNodeUnstageUnpublishVolume(t *testing.T) {
	for _, tc := range []struct {
		name        string
		prepareFunc func() (string, func(*testing.T, string), error)
		call        func(string) error
	}{
		{
			name:        "unstage_bind_mount_regression",
			prepareFunc: defaultPrepareFunc,
			call: func(target string) error {
				req := &csi.NodeUnstageVolumeRequest{
					VolumeId:          "test",
					StagingTargetPath: target,
				}

				_, err := defaultNodeServer.NodeUnstageVolume(context.TODO(), req)
				return err
			},
		},
		{
			name:        "unpublish_bind_mount_regression",
			prepareFunc: defaultPrepareFunc,
			call: func(target string) error {
				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId:   "test",
					TargetPath: target,
				}

				_, err := defaultNodeServer.NodeUnpublishVolume(context.TODO(), req)
				return err
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			target, teardownFunc, err := tc.prepareFunc()
			if err != nil {
				t.Errorf("failed to prepare test: %v", err)
				return
			}
			defer teardownFunc(t, target)

			if err = tc.call(target); err != nil {
				t.Errorf("failed to unstage volume: %v", err)
			}
		})
	}
}
