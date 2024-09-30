package driver

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"reflect"
	"testing"

	metadata "github.com/linode/go-metadata"
	"github.com/linode/linodego"
	"go.uber.org/mock/gomock"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	filesystem "github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
)

func TestMemoryToBytes(t *testing.T) {
	tests := []struct {
		input int
		want  uint
	}{
		{input: 1024, want: 1 << 30},     // 1GiB
		{input: 2048, want: 2 << 30},     // 2GiB
		{input: 32768, want: 32 << 30},   // 32GiB
		{input: 196608, want: 192 << 30}, // 192GiB
		{input: 131072, want: 128 << 30}, // 128GiB
		{input: -1, want: minMemory},
	}

	for _, tt := range tests {
		if got := memoryToBytes(tt.input); tt.want != got {
			t.Errorf("%d: want=%d got=%d", tt.input, tt.want, got)
		}
	}
}

func TestGetMetadataFromAPINilCheck(t *testing.T) {
	// Make sure GetMetadataFromAPI fails when it's given a nil client.
	t.Run("NilClient", func(t *testing.T) {
		_, err := GetMetadataFromAPI(context.Background(), nil, filesystem.OSFileSystem{})
		if err == nil {
			t.Fatal("should have failed")
		}
		if !errors.Is(err, errNilClient) {
			t.Errorf("wrong error returned\n\twanted=%q\n\tgot=%q", errNilClient, err)
		}
	})

	// GetMetadataFromAPI depends on the LinodeIDPath file to exist, so we
	// should skip the rest of this test if it cannot be found.
	if _, err := os.Stat(LinodeIDPath); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("%s does not exist", LinodeIDPath)
	} else if err != nil {
		t.Fatal(err)
	}
}

func TestGetMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name    string
		setup   func(*mocks.MockMetadataClient)
		want    Metadata
		wantErr bool
	}{
		{
			name: "Successful retrieval",
			setup: func(mc *mocks.MockMetadataClient) {
				mc.EXPECT().GetInstance(gomock.Any()).Return(&metadata.InstanceData{
					ID:     123,
					Label:  "test-instance",
					Region: "us-east",
					Specs: metadata.InstanceSpecsData{
						Memory: 2048,
					},
				}, nil)
			},
			want: Metadata{
				ID:     123,
				Label:  "test-instance",
				Region: "us-east",
				Memory: 2 << 30, // 2GB in bytes
			},
			wantErr: false,
		},
		{
			name: "Error from metadata service",
			setup: func(mc *mocks.MockMetadataClient) {
				mc.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
			},
			want:    Metadata{},
			wantErr: true,
		},
		{
			name: "Minimum memory handling",
			setup: func(mc *mocks.MockMetadataClient) {
				mc.EXPECT().GetInstance(gomock.Any()).Return(&metadata.InstanceData{
					ID:     456,
					Label:  "small-instance",
					Region: "us-west",
					Specs: metadata.InstanceSpecsData{
						Memory: 512, // Less than 1024MB
					},
				}, nil)
			},
			want: Metadata{
				ID:     456,
				Label:  "small-instance",
				Region: "us-west",
				Memory: minMemory,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocks.NewMockMetadataClient(ctrl)
			tt.setup(mockClient)

			got, err := GetMetadata(context.Background(), mockClient)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMetadata() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetMetadataFromAPI(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := mocks.NewMockFileSystem(ctrl)
	mockFile := mocks.NewMockFileInterface(ctrl)
	mockClient := mocks.NewMockLinodeClient(ctrl)

	tests := []struct {
		name    string
		setup   func(*mocks.MockLinodeClient, *mocks.MockFileSystem, *mocks.MockFileInterface)
		want    Metadata
		wantErr bool
	}{
		{
			name: "Happy path",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, nil)
				fs.EXPECT().Open(LinodeIDPath).Return(file, nil)
				file.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
					return copy(p, "12345"), io.EOF
				})
				file.EXPECT().Close().Return(nil)
				client.EXPECT().GetInstance(gomock.Any(), 12345).Return(&linodego.Instance{
					ID:     12345,
					Label:  "test-instance",
					Region: "us-east",
					Specs:  &linodego.InstanceSpec{Memory: 2048},
				}, nil)
			},
			want: Metadata{
				ID:     12345,
				Label:  "test-instance",
				Region: "us-east",
				Memory: 2048 << 20,
			},
			wantErr: false,
		},
		{
			name: "Stat error",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, errors.New("stat error"))
			},
			want:    Metadata{},
			wantErr: true,
		},
		{
			name: "Open file error",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, nil)
				fs.EXPECT().Open(LinodeIDPath).Return(nil, errors.New("open error"))
			},
			want:    Metadata{},
			wantErr: true,
		},
		{
			name: "Read error",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, nil)
				fs.EXPECT().Open(LinodeIDPath).Return(file, nil)
				file.EXPECT().Read(gomock.Any()).Return(0, errors.New("read error"))
				file.EXPECT().Close().Return(nil)
			},
			want:    Metadata{},
			wantErr: true,
		},
		{
			name: "Invalid Linode ID",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, nil)
				fs.EXPECT().Open(LinodeIDPath).Return(file, nil)
				file.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
					return copy(p, "invalid"), io.EOF
				})
				file.EXPECT().Close().Return(nil)
			},
			want:    Metadata{},
			wantErr: true,
		},
		{
			name: "GetInstance error",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, nil)
				fs.EXPECT().Open(LinodeIDPath).Return(file, nil)
				file.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
					return copy(p, "12345"), io.EOF
				})
				file.EXPECT().Close().Return(nil)
				client.EXPECT().GetInstance(gomock.Any(), 12345).Return(nil, errors.New("API error"))
			},
			want:    Metadata{},
			wantErr: true,
		},
		{
			name: "Minimum memory handling",
			setup: func(client *mocks.MockLinodeClient, fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Stat(LinodeIDPath).Return(nil, nil)
				fs.EXPECT().Open(LinodeIDPath).Return(file, nil)
				file.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
					return copy(p, "12345"), io.EOF
				})
				file.EXPECT().Close().Return(nil)
				client.EXPECT().GetInstance(gomock.Any(), 12345).Return(&linodego.Instance{
					ID:     12345,
					Label:  "small-instance",
					Region: "us-west",
					Specs:  &linodego.InstanceSpec{Memory: 512},
				}, nil)
			},
			want: Metadata{
				ID:     12345,
				Label:  "small-instance",
				Region: "us-west",
				Memory: minMemory,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(mockClient, mockFS, mockFile)

			got, err := GetMetadataFromAPI(context.Background(), mockClient, mockFS)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMetadataFromAPI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMetadataFromAPI() = %v, want %v", got, tt.want)
			}
		})
	}
}
