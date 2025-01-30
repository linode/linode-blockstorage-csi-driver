package driver

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	metadata "github.com/linode/go-metadata"
	"github.com/linode/linodego"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
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

func TestGetNodeMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataClient := mocks.NewMockMetadataClient(ctrl)

	tests := []struct {
		name               string
		setupMocks         func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem)
		expectedMetadata   Metadata
		expectedErr        string
		metadataClientFunc func(context.Context) (MetadataClient, error)
		nodeName           string
	}{
		{
			name: "Successful retrieval from metadata service",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(&metadata.InstanceData{
					ID:     123,
					Label:  "test-instance",
					Region: "us-east",
					Specs: metadata.InstanceSpecsData{
						Memory: 2048,
					},
				}, nil)
			},
			expectedMetadata: Metadata{
				ID:     123,
				Label:  "test-instance",
				Region: "us-east",
				Memory: 2 << 30, // 2 GB
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			nodeName: "test-node",
		},
		{
			name: "Failure to create metadata client, successful retrieval from kube API",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(&corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "linode://456",
					},
				}, nil).Times(1)
				mockCloudProvider.EXPECT().GetInstance(gomock.Any(), gomock.Eq(456)).Return(&linodego.Instance{
					ID:     456,
					Label:  "api-instance",
					Region: "eu-west",
					Specs:  &linodego.InstanceSpec{Memory: 4096},
				}, nil)
			},
			expectedMetadata: Metadata{
				ID:     456,
				Label:  "api-instance",
				Region: "eu-west",
				Memory: 4 << 30, // 4 GB
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return nil, errors.New("failed to create metadata client")
			},
			nodeName: "test-node",
		},
		{
			name: "Failure from metadata service & product_serial file, successful retrieval from kube API",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(&corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "linode://789",
					},
				}, nil).Times(1)
				mockCloudProvider.EXPECT().GetInstance(gomock.Any(), gomock.Eq(789)).Return(&linodego.Instance{
					ID:     789,
					Label:  "fallback-instance",
					Region: "ap-south",
					Specs:  &linodego.InstanceSpec{Memory: 8192},
				}, nil)
			},
			expectedMetadata: Metadata{
				ID:     789,
				Label:  "fallback-instance",
				Region: "ap-south",
				Memory: 8 << 30, // 8 GB
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			nodeName: "test-node",
		},
		{
			name: "Failure from metadata service & product_serial file, successful retrieval from kube API but no provider ID found",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(&corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "",
					},
				}, nil).Times(1)
			},
			expectedMetadata: Metadata{},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			nodeName:    "test-node",
			expectedErr: "provider ID not found for node test-node",
		},
		{
			name: "Successful retrieval from Kubernetes API but unsuccessful retrieval from Linode API",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(&corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "linode://789",
					},
				}, nil).Times(1)
				mockCloudProvider.EXPECT().GetInstance(gomock.Any(), gomock.Eq(789)).Return(nil, errors.New("linode API error"))
			},
			expectedMetadata: Metadata{
				ID:     789,
				Label:  "fallback-instance",
				Region: "ap-south",
				Memory: 8 << 30, // 8 GB
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			expectedErr: "failed to get instance details: linode API error",
			nodeName:    "test-node",
		},
		{
			name: "Successful retrieval from Kubernetes API, but invalid provider id string format",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(&corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "linode:///789",
					},
				}, nil).Times(1)
			},
			expectedMetadata: Metadata{
				ID:     789,
				Label:  "fallback-instance",
				Region: "ap-south",
				Memory: 8 << 30, // 8 GB
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			expectedErr: "failed to parse Linode ID from provider ID: strconv.Atoi: parsing \"/789\": invalid syntax",
			nodeName:    "test-node",
		},
		{
			name: "Successful retrieval from Kubernetes API, but invalid provider id string format",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(&corev1.Node{
					Spec: corev1.NodeSpec{
						ProviderID: "linde://789",
					},
				}, nil).Times(1)
			},
			expectedMetadata: Metadata{
				ID:     789,
				Label:  "fallback-instance",
				Region: "ap-south",
				Memory: 8 << 30, // 8 GB
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			expectedErr: "invalid provider ID format: linde://789",
			nodeName:    "test-node",
		},
		{
			name: "Failure from both metadata service, product_serial file, and kube API",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
				mockKubeClient.EXPECT().GetNode(gomock.Any(), gomock.Eq("test-node")).Return(nil, errors.New("kube client error"))
			},
			expectedErr: "failed to get node test-node: kube client error",
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			nodeName: "test-node",
		},
		{
			name: "Failure for metadata service and no node name provided",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFileSystem *mocks.MockFileSystem) {
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))
				mockFileSystem.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(nil, os.ErrNotExist)
			},
			expectedErr: "NODE_NAME environment variable not set",
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			nodeName: "",
		},
		{
			name: "Fallback to product_serial after metadata service failure",
			setupMocks: func(mockKubeClient *mocks.MockKubeClient, mockCloudProvider *mocks.MockLinodeClient, mockFS *mocks.MockFileSystem) {
				// Metadata service failure
				mockMetadataClient.EXPECT().GetInstance(gomock.Any()).Return(nil, errors.New("metadata service error"))

				// Successful product_serial read
				mockFile := mocks.NewMockFileInterface(ctrl)
				mockFS.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(mockFile, nil)
				mockFile.EXPECT().Read(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
					copy(b, "654321")
					return 6, nil
				})
				mockFile.EXPECT().Close().Return(nil)

				// Cloud provider returns instance
				mockCloudProvider.EXPECT().GetInstance(gomock.Any(), 654321).Return(&linodego.Instance{
					ID:     654321,
					Label:  "serial-instance",
					Region: "us-central",
					Specs:  &linodego.InstanceSpec{Memory: 4096},
				}, nil)
			},
			expectedMetadata: Metadata{
				ID:     654321,
				Label:  "serial-instance",
				Region: "us-central",
				Memory: 4 << 30,
			},
			metadataClientFunc: func(context.Context) (MetadataClient, error) {
				return mockMetadataClient, nil
			},
			nodeName: "test-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKubeClient := mocks.NewMockKubeClient(ctrl)
			mockCloudProvider := mocks.NewMockLinodeClient(ctrl)
			mockFileSystem := mocks.NewMockFileSystem(ctrl)
			// Setup mocks
			tt.setupMocks(mockKubeClient, mockCloudProvider, mockFileSystem)

			// Override the metadata.NewClient function for testing
			oldNewClient := NewMetadataClient
			oldNewKubeClient := newKubeClient

			NewMetadataClient = tt.metadataClientFunc
			newKubeClient = func(ctx context.Context) (KubeClient, error) {
				return mockKubeClient, nil
			}

			defer func() {
				NewMetadataClient = oldNewClient
				newKubeClient = oldNewKubeClient
			}()

			// Execute the function under test
			nodeMetadata, err := GetNodeMetadata(context.Background(), mockCloudProvider, tt.nodeName, mockFileSystem)

			// Check results
			if tt.expectedErr != "" {
				if err == nil || err.Error() != tt.expectedErr {
					t.Errorf("Expected error: %v, got: %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !reflect.DeepEqual(tt.expectedMetadata, nodeMetadata) {
					t.Errorf("Expected metadata: %+v, got: %+v", tt.expectedMetadata, nodeMetadata)
				}
			}
		})
	}
}

func TestGetLinodeIDFromProductSerial(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(*mocks.MockFileSystem, *mocks.MockFileInterface)
		expectedID  int
		expectedErr string
	}{
		{
			name: "Successfully read valid ID",
			setup: func(fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(file, nil)
				file.EXPECT().Read(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
					copy(b, "123456\n")
					return 7, nil
				})
				file.EXPECT().Close().Return(nil)
			},
			expectedID: 123456,
		},
		{
			name: "File not found",
			setup: func(fs *mocks.MockFileSystem, _ *mocks.MockFileInterface) {
				fs.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").
					Return(nil, os.ErrNotExist)
			},
			expectedErr: "failed to open product_serial file: file does not exist",
		},
		{
			name: "Read error",
			setup: func(fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(file, nil)
				file.EXPECT().Read(gomock.Any()).Return(0, errors.New("read error"))
				file.EXPECT().Close().Return(nil)
			},
			expectedErr: "failed to read product_serial file: read error",
		},
		{
			name: "Invalid ID format",
			setup: func(fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(file, nil)
				file.EXPECT().Read(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
					copy(b, "invalid_id")
					return 10, nil
				})
				file.EXPECT().Close().Return(nil)
			},
			expectedErr: "invalid linode ID format in product_serial: strconv.Atoi: parsing \"invalid_id\": invalid syntax",
		},
		{
			name: "Empty file content",
			setup: func(fs *mocks.MockFileSystem, file *mocks.MockFileInterface) {
				fs.EXPECT().Open("/sys/devices/virtual/dmi/id/product_serial").Return(file, nil)
				file.EXPECT().Read(gomock.Any()).Return(0, nil)
				file.EXPECT().Close().Return(nil)
			},
			expectedErr: "invalid linode ID format in product_serial: strconv.Atoi: parsing \"\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFS := mocks.NewMockFileSystem(ctrl)
			mockFile := mocks.NewMockFileInterface(ctrl)
			tt.setup(mockFS, mockFile)

			id, err := getLinodeIDFromProductSerial(ctx, mockFS)

			if tt.expectedErr != "" {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedErr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if id != tt.expectedID {
					t.Errorf("Expected ID %d, got %d", tt.expectedID, id)
				}
			}
		})
	}
}
