package driver

import (
	"context"
	"reflect"
	"testing"

	"github.com/linode/linodego"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"

	linodeclient "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-client"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

func TestNewNodeServer(t *testing.T) {
	type args struct {
		linodeDriver *LinodeDriver
		mounter      *mount.SafeFormatAndMount
		deviceUtils  mountmanager.DeviceUtils
		client       linodeclient.LinodeClient
		metadata     Metadata
		encrypt      Encryption
	}
	tests := []struct {
		name    string
		args    args
		want    *NodeServer
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  mountmanager.NewDeviceUtils(mountmanager.NewFileSystem(), exec.New()),
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want: &NodeServer{
				driver:      &LinodeDriver{},
				mounter:     &mount.SafeFormatAndMount{},
				deviceutils: mountmanager.NewDeviceUtils(mountmanager.NewFileSystem(), exec.New()),
				client:      &linodego.Client{},
				metadata:    Metadata{},
				encrypt:     Encryption{},
			},
			wantErr: false,
		},
		{
			name: "nil linodeDriver",
			args: args{
				linodeDriver: nil,
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  mountmanager.NewDeviceUtils(mountmanager.NewFileSystem(), exec.New()),
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil mounter",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      nil,
				deviceUtils:  mountmanager.NewDeviceUtils(mountmanager.NewFileSystem(), exec.New()),
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil deviceUtils",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  nil,
				client:       &linodego.Client{},
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil linode client",
			args: args{
				linodeDriver: &LinodeDriver{},
				mounter:      &mount.SafeFormatAndMount{},
				deviceUtils:  mountmanager.NewDeviceUtils(mountmanager.NewFileSystem(), exec.New()),
				client:       nil,
				metadata:     Metadata{},
				encrypt:      Encryption{},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNodeServer(context.Background(), tt.args.linodeDriver, tt.args.mounter, tt.args.deviceUtils, tt.args.client, tt.args.metadata, tt.args.encrypt)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNodeServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewNodeServer() = %v, want %v", got, tt.want)
			}
		})
	}
}
