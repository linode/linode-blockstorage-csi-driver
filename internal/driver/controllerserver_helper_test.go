package driver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	linodevolumes "github.com/linode/linode-blockstorage-csi-driver/pkg/linode-volumes"
)

func TestPrepareCreateVolumeResponse(t *testing.T) {
	testCases := []struct {
		name          string
		vol           *linodego.Volume
		size          int64
		context       map[string]string
		sourceInfo    *linodevolumes.LinodeVolumeKey
		contentSource *csi.VolumeContentSource
		expected      *csi.CreateVolumeResponse
	}{
		{
			name: "Basic volume without source",
			vol: &linodego.Volume{
				ID:     123,
				Label:  "testvolume",
				Region: "us-east",
			},
			size:    10 << 30, // 10 GiB
			context: map[string]string{"key": "value"},
			expected: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      "123-testvolume",
					CapacityBytes: 10 << 30,
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								VolumeTopologyRegion: "us-east",
							},
						},
					},
					VolumeContext: map[string]string{"key": "value"},
				},
			},
		},
		{
			name: "Volume with source",
			vol: &linodego.Volume{
				ID:     456,
				Label:  "clonedvolume",
				Region: "us-west",
			},
			size:    20 << 30, // 20 GiB
			context: map[string]string{"cloned": "true"},
			sourceInfo: &linodevolumes.LinodeVolumeKey{
				VolumeID: 789,
				Label:    "source-volume",
			},
			contentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Volume{
					Volume: &csi.VolumeContentSource_VolumeSource{
						VolumeId: "789-sourcevolume",
					},
				},
			},
			expected: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      "456-clonedvolume",
					CapacityBytes: 20 << 30,
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								VolumeTopologyRegion: "us-west",
							},
						},
					},
					VolumeContext: map[string]string{"cloned": "true"},
					ContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Volume{
							Volume: &csi.VolumeContentSource_VolumeSource{
								VolumeId: "789-sourcevolume",
							},
						},
					},
				},
			},
		},
		{
			name: "Volume with empty context",
			vol: &linodego.Volume{
				ID:     789,
				Label:  "emptycontextvolume",
				Region: "eu-west",
			},
			size:    5 << 30, // 5 GiB
			context: map[string]string{},
			expected: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      "789-emptycontextvolume",
					CapacityBytes: 5 << 30,
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								VolumeTopologyRegion: "eu-west",
							},
						},
					},
					VolumeContext: map[string]string{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cs := &ControllerServer{}
			ctx := context.Background()

			result := cs.prepareCreateVolumeResponse(ctx, tc.vol, tc.size, tc.context, tc.sourceInfo, tc.contentSource)

			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %+v, but got %+v", tc.expected, result)
			}
		})
	}
}

func TestCreateVolumeContext(t *testing.T) {
	vol := &linodego.Volume{
		Region: "us-east",
	}
	tests := []struct {
		name           string
		req            *csi.CreateVolumeRequest
		expectedResult map[string]string
	}{
		{
			name: "Non-encrypted volume",
			req: &csi.CreateVolumeRequest{
				Name:       "test-volume",
				Parameters: map[string]string{},
			},
			expectedResult: map[string]string{
				VolumeTopologyRegion: "us-east",
			},
		},
		{
			name: "Encrypted volume with all parameters",
			req: &csi.CreateVolumeRequest{
				Name: "encrypted-volume",
				Parameters: map[string]string{
					LuksEncryptedAttribute: "true",
					LuksCipherAttribute:    "aes-xts-plain64",
					LuksKeySizeAttribute:   "512",
				},
			},
			expectedResult: map[string]string{
				LuksEncryptedAttribute: "true",
				PublishInfoVolumeName:  "encrypted-volume",
				LuksCipherAttribute:    "aes-xts-plain64",
				LuksKeySizeAttribute:   "512",
				VolumeTopologyRegion:   "us-east",
			},
		},
		// IMPORTANT:Now sure if we want this behavior, but it's what the code currently does.
		{
			name: "Encrypted volume with missing cipher and key size",
			req: &csi.CreateVolumeRequest{
				Name: "partial-encrypted-volume",
				Parameters: map[string]string{
					LuksEncryptedAttribute: "true",
				},
			},
			expectedResult: map[string]string{
				LuksEncryptedAttribute: "true",
				PublishInfoVolumeName:  "partial-encrypted-volume",
				LuksCipherAttribute:    "",
				LuksKeySizeAttribute:   "",
				VolumeTopologyRegion:   "us-east",
			},
		},
		{
			name: "Non-encrypted volume with cipher and key size (should be ignored)",
			req: &csi.CreateVolumeRequest{
				Name: "non-encrypted-with-params",
				Parameters: map[string]string{
					LuksEncryptedAttribute: "false",
					LuksCipherAttribute:    "aes-xts-plain64",
					LuksKeySizeAttribute:   "512",
				},
			},
			expectedResult: map[string]string{
				VolumeTopologyRegion: "us-east",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &ControllerServer{}
			ctx := context.Background()
			result := cs.createVolumeContext(ctx, tt.req, vol)

			if !reflect.DeepEqual(result, tt.expectedResult) {
				t.Errorf("createVolumeContext() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestCreateVolumeContext_Encryption(t *testing.T) {
	vol := &linodego.Volume{
		Region: "us-east",
	}
	tests := []struct {
		name           string
		req            *csi.CreateVolumeRequest
		expectedResult map[string]string
	}{
		{
			name: "Encrypted volume with encryption enabled",
			req: &csi.CreateVolumeRequest{
				Name: "encrypted-volume",
				Parameters: map[string]string{
					VolumeEncryption: True,
				},
			},
			expectedResult: map[string]string{
				VolumeTopologyRegion: "us-east",
			},
		},
		{
			name: "Unencrypted volume",
			req: &csi.CreateVolumeRequest{
				Name:       "unencrypted-volume",
				Parameters: map[string]string{},
			},
			expectedResult: map[string]string{
				VolumeTopologyRegion: "us-east",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &ControllerServer{}
			ctx := context.Background()
			result := cs.createVolumeContext(ctx, tt.req, vol)

			// Use reflect.DeepEqual to compare maps; log specific missing keys if the test fails
			if !reflect.DeepEqual(result, tt.expectedResult) {
				t.Errorf("createVolumeContext() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestCreateAndWaitForVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLinodeClient(ctrl)
	cs := &ControllerServer{
		client: mockClient,
	}

	testCases := []struct {
		name           string
		volumeName     string
		sizeGB         int
		parameters     map[string]string
		sourceInfo     *linodevolumes.LinodeVolumeKey
		setupMocks     func()
		expectedVolume *linodego.Volume
		expectedError  error
	}{
		{
			name:       "Successful volume creation",
			volumeName: "test-volume",
			sizeGB:     20,
			parameters: map[string]string{
				VolumeTags: "tag1,tag2",
			},
			sourceInfo: nil,
			setupMocks: func() {
				mockClient.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				mockClient.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 123, Size: 20}, nil)
				mockClient.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 123, Size: 20, Status: linodego.VolumeActive}, nil)
			},
			expectedVolume: &linodego.Volume{ID: 123, Size: 20, Status: linodego.VolumeActive},
			expectedError:  nil,
		},
		{
			name:       "Volume creation fails",
			volumeName: "test-volume",
			sizeGB:     20,
			parameters: map[string]string{
				VolumeTags: "tag1,tag2",
			},
			sourceInfo: nil,
			setupMocks: func() {
				mockClient.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				mockClient.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Return(nil, errNoVolumeName)
			},
			expectedVolume: nil,
			expectedError:  errInternal("create volume: %v", errNoVolumeName),
		},
		{
			name:       "Volume exists with different size",
			volumeName: "existing-volume",
			sizeGB:     30,
			parameters: map[string]string{
				VolumeTags: "tag1,tag2",
			},
			sourceInfo: nil,
			setupMocks: func() {
				mockClient.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return([]linodego.Volume{
					{ID: 456, Size: 20, Status: linodego.VolumeActive},
				}, nil)
			},
			expectedVolume: nil,
			expectedError:  errAlreadyExists("volume 456 already exists with size 20"),
		},
		{
			name:       "Volume creation from source",
			volumeName: "cloned-volume",
			sizeGB:     40,
			parameters: map[string]string{
				VolumeTags: "tag1,tag2",
			},
			sourceInfo: &linodevolumes.LinodeVolumeKey{VolumeID: 789},
			setupMocks: func() {
				mockClient.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				mockClient.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 789, Size: 40, Status: linodego.VolumeActive}, nil)
				mockClient.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 789, Size: 40, Status: linodego.VolumeActive}, nil)
			},
			expectedVolume: &linodego.Volume{ID: 789, Size: 40, Status: linodego.VolumeActive},
			expectedError:  nil,
		},
		{
			name:       "Volume creation timeout",
			volumeName: "timeout-volume",
			sizeGB:     50,
			parameters: map[string]string{
				VolumeTags: "tag1,tag2",
			},
			sourceInfo: nil,
			setupMocks: func() {
				mockClient.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).Return(nil, nil)
				mockClient.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 101, Size: 50}, nil)
				mockClient.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&linodego.Volume{ID: 101, Size: 50}, fmt.Errorf("timed out"))
			},
			expectedVolume: nil,
			expectedError:  errInternal("Timed out waiting for volume 101 to be active: timed out"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()
			encryptionStatus := "disabled"
			volume, err := cs.createAndWaitForVolume(context.Background(), tc.volumeName, tc.parameters, encryptionStatus, tc.sizeGB, tc.sourceInfo, "us-east")

			if err != nil && !reflect.DeepEqual(tc.expectedError, err) {
				if tc.expectedError != nil {
					t.Errorf("expected error %v, got %v", tc.expectedError, err)
				} else {
					t.Errorf("expected no error but got %v", err)
				}
			}

			if !reflect.DeepEqual(tc.expectedVolume, volume) {
				t.Errorf("expected volume %v, got %v", tc.expectedVolume, volume)
			}
		})
	}
}

func TestPrepareVolumeParams(t *testing.T) {
	tests := []struct {
		name           string
		req            *csi.CreateVolumeRequest
		expectedName   string
		expectedSizeGB int
		expectedSize   int64
		expectedError  error
	}{
		{
			name: "Valid request with required bytes",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 15 << 30, // 15 GiB
				},
			},
			expectedName:   "csi-linode-pv-test-volume",
			expectedSizeGB: 15,
			expectedSize:   15 << 30,
			expectedError:  nil,
		},
		{
			name: "Valid request with limit bytes",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume-limit",
				CapacityRange: &csi.CapacityRange{
					LimitBytes: 20 << 30, // 20 GiB
				},
			},
			expectedName:   "csi-linode-pv-test-volume-limit",
			expectedSizeGB: 20,
			expectedSize:   20 << 30,
			expectedError:  nil,
		},
		{
			name: "Request with size less than minimum",
			req: &csi.CreateVolumeRequest{
				Name: "small-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 5 << 30, // 5 GiB
				},
			},
			expectedName:   "csi-linode-pv-small-volume",
			expectedSizeGB: 10, // Minimum size
			expectedSize:   10 << 30,
			expectedError:  nil,
		},
		{
			name: "Request with no capacity range",
			req: &csi.CreateVolumeRequest{
				Name: "default-volume",
			},
			expectedName:   "csi-linode-pv-default-volume",
			expectedSizeGB: 10, // Minimum size
			expectedSize:   10 << 30,
			expectedError:  nil,
		},
		{
			name: "Request with negative required bytes",
			req: &csi.CreateVolumeRequest{
				Name: "negative-volume",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: -10 << 30,
				},
			},
			expectedName:   "",
			expectedSizeGB: 0,
			expectedSize:   0,
			expectedError:  errors.New("RequiredBytes and LimitBytes must not be negative"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &ControllerServer{
				driver: &LinodeDriver{
					volumeLabelPrefix: "csi-linode-pv-",
				},
			}
			ctx := context.Background()

			params, err := cs.prepareVolumeParams(ctx, tt.req)

			// First, verify that the error matches the expectation
			if (err != nil && tt.expectedError == nil) || (err == nil && tt.expectedError != nil) {
				t.Errorf("expected error %v, got %v", tt.expectedError, err)
			} else if err != nil && tt.expectedError != nil && err.Error() != tt.expectedError.Error() {
				t.Errorf("expected error message %v, got %v", tt.expectedError.Error(), err.Error())
			}

			// Only check params fields if params is not nil
			if params != nil {
				if params.VolumeName != tt.expectedName {
					t.Errorf("Expected volume name: %s, but got: %s", tt.expectedName, params.VolumeName)
				}

				if params.TargetSizeGB != tt.expectedSizeGB {
					t.Errorf("Expected size in GB: %d, but got: %d", tt.expectedSizeGB, params.TargetSizeGB)
				}

				if params.Size != tt.expectedSize {
					t.Errorf("Expected size in bytes: %d, but got: %d", tt.expectedSize, params.Size)
				}
			} else if err == nil {
				// If params is nil and no error was expected, the test should fail
				t.Errorf("expected non-nil params, got nil")
			}
		})
	}
}

func TestPrepareVolumeParams_Encryption(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLinodeClient(ctrl)
	cs := &ControllerServer{
		client: mockClient,
		driver: &LinodeDriver{
			volumeLabelPrefix: "csi-linode-pv-",
		},
		metadata: Metadata{Region: "us-east"},
	}
	ctx := context.Background()

	tests := []struct {
		name            string
		req             *csi.CreateVolumeRequest
		setupMocks      func()
		expectedEncrypt string
		expectedError   error
	}{
		{
			name: "Encryption supported and enabled",
			req: &csi.CreateVolumeRequest{
				Name: "encrypted-volume",
				Parameters: map[string]string{
					VolumeEncryption: True,
				},
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10 << 30, // 10 GiB
				},
			},
			setupMocks: func() {
				mockClient.EXPECT().GetRegion(gomock.Any(), "us-east").Return(&linodego.Region{
					Capabilities: []string{"Block Storage Encryption"},
				}, nil)
			},
			expectedEncrypt: "enabled",
			expectedError:   nil,
		},
		{
			name: "Encryption not supported in region",
			req: &csi.CreateVolumeRequest{
				Name: "unencrypted-volume",
				Parameters: map[string]string{
					VolumeEncryption: True,
				},
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10 << 30, // 10 GiB
				},
			},
			setupMocks: func() {
				mockClient.EXPECT().GetRegion(gomock.Any(), "us-east").Return(&linodego.Region{
					Capabilities: []string{},
				}, nil)
			},
			expectedEncrypt: "disabled",
			expectedError:   errInternal("Volume encryption is not supported in the us-east region"),
		},
		{
			name: "Encryption disabled in parameters",
			req: &csi.CreateVolumeRequest{
				Name: "unencrypted-volume",
				Parameters: map[string]string{
					VolumeEncryption: "false",
				},
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 10 << 30, // 10 GiB
				},
			},
			setupMocks:      func() {},
			expectedEncrypt: "disabled",
			expectedError:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			// Call prepareVolumeParams and capture the result and error
			params, err := cs.prepareVolumeParams(ctx, tt.req)

			// Verify that the error matches the expected error
			if (err != nil && tt.expectedError == nil) || (err == nil && tt.expectedError != nil) {
				t.Errorf("expected error %v, got %v", tt.expectedError, err)
			} else if err != nil && tt.expectedError != nil && err.Error() != tt.expectedError.Error() {
				t.Errorf("expected error message %v, got %v", tt.expectedError.Error(), err.Error())
			}

			// Only proceed to check params fields if params is non-nil and no error was expected
			if params != nil && err == nil {
				if params.EncryptionStatus != tt.expectedEncrypt {
					t.Errorf("Expected encryption status %v, got %v", tt.expectedEncrypt, params.EncryptionStatus)
				}
			} else if params == nil && err == nil {
				// Fail the test if params is nil but no error was expected
				t.Errorf("expected non-nil params, got nil")
			}
		})
	}
}

func TestValidateCreateVolumeRequest(t *testing.T) {
	cs := &ControllerServer{}
	ctx := context.Background()

	testCases := []struct {
		name    string
		req     *csi.CreateVolumeRequest
		wantErr error
	}{
		{
			name: "Valid request",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "Empty volume name",
			req: &csi.CreateVolumeRequest{
				Name: "",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
			},
			wantErr: errNoVolumeName,
		},
		{
			name: "No volume capabilities",
			req: &csi.CreateVolumeRequest{
				Name:               "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{},
			},
			wantErr: errNoVolumeCapabilities,
		},
		{
			name: "Invalid volume capability",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
			},
			wantErr: errInvalidVolumeCapability([]*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			}),
		},
		{
			name: "Nil volume capability",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					nil,
				},
			},
			wantErr: errInvalidVolumeCapability([]*csi.VolumeCapability{nil}),
		},
		{
			name: "Nil access mode",
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessMode: nil,
					},
				},
			},
			wantErr: errInvalidVolumeCapability([]*csi.VolumeCapability{
				{
					AccessMode: nil,
				},
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := cs.validateCreateVolumeRequest(ctx, tc.req)
			if gotErr != nil && !reflect.DeepEqual(gotErr, tc.wantErr) {
				t.Errorf("validateCreateVolumeRequest() error = %v, wantErr %v", gotErr, tc.wantErr)
			}
		})
	}
}

func TestValidateControllerPublishVolumeRequest(t *testing.T) {
	cs := &ControllerServer{}
	ctx := context.Background()

	testCases := []struct {
		name           string
		req            *csi.ControllerPublishVolumeRequest
		expectedNodeID int
		expectedVolID  int
		expectedErr    error
	}{
		{
			name: "Valid request",
			req: &csi.ControllerPublishVolumeRequest{
				NodeId:   "12345",
				VolumeId: "67890-test-volume",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeContext: map[string]string{
					VolumeTopologyRegion: "us-east",
				},
			},
			expectedNodeID: 12345,
			expectedVolID:  67890,
			expectedErr:    nil,
		},
		{
			name: "missing node ID",
			req: &csi.ControllerPublishVolumeRequest{
				NodeId:   "",
				VolumeId: "67890-test-volume",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expectedNodeID: 0,
			expectedVolID:  0,
			expectedErr:    status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID must be provided"),
		},
		{
			name: "missing volume ID",
			req: &csi.ControllerPublishVolumeRequest{
				NodeId:   "12345",
				VolumeId: "",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expectedNodeID: 0,
			expectedVolID:  0,
			expectedErr:    status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided"),
		},
		{
			name: "Missing volume capability",
			req: &csi.ControllerPublishVolumeRequest{
				NodeId:           "12345",
				VolumeId:         "67890-test-volume",
				VolumeCapability: nil,
			},
			expectedNodeID: 0,
			expectedVolID:  0,
			expectedErr:    errNoVolumeCapability,
		},
		{
			name: "Invalid volume capability",
			req: &csi.ControllerPublishVolumeRequest{
				NodeId:   "12345",
				VolumeId: "67890-test-volume",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			expectedNodeID: 0,
			expectedVolID:  0,
			expectedErr:    errInvalidVolumeCapability([]*csi.VolumeCapability{{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}}}),
		},
		{
			name: "Nil access mode",
			req: &csi.ControllerPublishVolumeRequest{
				NodeId:   "12345",
				VolumeId: "67890-test-volume",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: nil,
				},
			},
			expectedNodeID: 0,
			expectedVolID:  0,
			expectedErr:    errInvalidVolumeCapability([]*csi.VolumeCapability{{AccessMode: nil}}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nodeID, volID, err := cs.validateControllerPublishVolumeRequest(ctx, tc.req)

			if err != nil && !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Expected error %v, but got %v", tc.expectedErr, err)
			}

			if nodeID != tc.expectedNodeID {
				t.Errorf("Expected node ID %d, but got %d", tc.expectedNodeID, nodeID)
			}

			if volID != tc.expectedVolID {
				t.Errorf("Expected volume ID %d, but got %d", tc.expectedVolID, volID)
			}
		})
	}
}

func TestGetAndValidateVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLinodeClient(ctrl)
	cs := &ControllerServer{
		client: mockClient,
	}

	testCases := []struct {
		name           string
		volumeID       int
		linode         *linodego.Instance
		setupMocks     func()
		expectedResult string
		expectedError  error
	}{
		{
			name:     "Volume found and attached to correct instance",
			volumeID: 123,
			linode: &linodego.Instance{
				ID: 456,
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 123).Return(&linodego.Volume{
					ID:             123,
					LinodeID:       &[]int{456}[0],
					FilesystemPath: "/dev/disk/by-id/scsi-0Linode_Volume_test-volume",
				}, nil)
			},
			expectedResult: "/dev/disk/by-id/scsi-0Linode_Volume_test-volume",
			expectedError:  nil,
		},
		{
			name:     "Volume found but not attached",
			volumeID: 123,
			linode: &linodego.Instance{
				ID:     456,
				Region: "us-east",
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 123).Return(&linodego.Volume{
					ID:       123,
					LinodeID: nil,
					Region:   "us-east",
				}, nil)
			},
			expectedResult: "",
			expectedError:  nil,
		},
		{
			name:     "Volume found but attached to different instance",
			volumeID: 123,
			linode: &linodego.Instance{
				ID: 456,
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 123).Return(&linodego.Volume{
					ID:       123,
					LinodeID: &[]int{789}[0],
				}, nil)
			},
			expectedResult: "",
			expectedError:  errVolumeAttached(123, 456),
		},
		{
			name:     "Volume not found",
			volumeID: 123,
			linode: &linodego.Instance{
				ID: 456,
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 123).Return(nil, &linodego.Error{
					Code:    http.StatusNotFound,
					Message: "Not Found",
				})
			},
			expectedResult: "",
			expectedError:  errVolumeNotFound(123),
		},
		{
			name:     "API error",
			volumeID: 123,
			linode: &linodego.Instance{
				ID: 456,
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 123).Return(nil, errors.New("API error"))
			},
			expectedResult: "",
			expectedError:  errInternal("get volume 123: API error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			result, err := cs.getAndValidateVolume(context.Background(), tc.volumeID, tc.linode)

			if err != nil && !reflect.DeepEqual(tc.expectedError, err) {
				t.Errorf("expected error %v, got %v", tc.expectedError, err)
			}

			if tc.expectedResult != result {
				t.Errorf("expected result %s, got %s", tc.expectedResult, result)
			}
		})
	}
}

func TestGetContentSourceVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLinodeClient(ctrl)
	cs := &ControllerServer{
		client: mockClient,
		metadata: Metadata{
			Region: "us-east",
		},
	}

	testCases := []struct {
		name           string
		req            *csi.CreateVolumeRequest
		setupMocks     func()
		expectedResult *linodevolumes.LinodeVolumeKey
		expectedError  error
	}{
		{
			name: "Nil content source",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: nil,
			},
			setupMocks:     func() {},
			expectedResult: nil,
			expectedError:  errNilSource,
		},
		{
			name: "Invalid content source type",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Snapshot{},
				},
			},
			setupMocks:     func() {},
			expectedResult: nil,
			expectedError:  errUnsupportedVolumeContentSource,
		},
		{
			name: "Nil volume",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: nil,
					},
				},
			},
			setupMocks:     func() {},
			expectedResult: nil,
			expectedError:  errNoSourceVolume,
		},
		{
			name: "Invalid volume ID",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "test-volume",
						},
					},
				},
			},
			setupMocks:     func() {},
			expectedResult: nil,
			expectedError:  errInternal("parse volume info from content source: invalid linode volume id: \"test\""),
		},
		{
			name: "Valid content source, matching region",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "123-testvolume",
						},
					},
				},
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 123).Return(&linodego.Volume{
					ID:     123,
					Region: "us-east",
				}, nil)
			},
			expectedResult: &linodevolumes.LinodeVolumeKey{
				VolumeID: 123,
				Label:    "testvolume",
			},
			expectedError: nil,
		},
		{
			name: "Valid content source, mismatched region",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "456-othervolume",
						},
					},
				},
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 456).Return(&linodego.Volume{
					ID:     456,
					Region: "us-west",
				}, nil)
			},
			expectedResult: nil,
			expectedError:  errRegionMismatch("us-west", "us-east"),
		},
		{
			name: "API error",
			req: &csi.CreateVolumeRequest{
				VolumeContentSource: &csi.VolumeContentSource{
					Type: &csi.VolumeContentSource_Volume{
						Volume: &csi.VolumeContentSource_VolumeSource{
							VolumeId: "789-errorvolume",
						},
					},
				},
			},
			setupMocks: func() {
				mockClient.EXPECT().GetVolume(gomock.Any(), 789).Return(nil, errors.New("API error"))
			},
			expectedResult: nil,
			expectedError:  errInternal("get volume 789: API error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			result, err := cs.getContentSourceVolume(context.Background(), tc.req.GetVolumeContentSource(), tc.req.GetAccessibilityRequirements())

			if err != nil && !reflect.DeepEqual(tc.expectedError, err) {
				t.Errorf("expected error %v, got %v", tc.expectedError, err)
			}

			if !reflect.DeepEqual(tc.expectedResult, result) {
				t.Errorf("expected result %+v, got %+v", tc.expectedResult, result)
			}
		})
	}
}

func TestAttachVolume(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLinodeClient(ctrl)
	cs := &ControllerServer{
		client: mockClient,
	}

	testCases := []struct {
		name          string
		volumeID      int
		linodeID      int
		setupMocks    func()
		expectedError error
	}{
		{
			name:     "Successful attachment",
			volumeID: 123,
			linodeID: 456,
			setupMocks: func() {
				mockClient.EXPECT().AttachVolume(gomock.Any(), 123, gomock.Any()).Return(&linodego.Volume{}, nil)
			},
			expectedError: nil,
		},
		{
			name:     "Volume already attached",
			volumeID: 789,
			linodeID: 101,
			setupMocks: func() {
				mockClient.EXPECT().AttachVolume(gomock.Any(), 789, gomock.Any()).Return(nil, &linodego.Error{Message: "Volume 789 is already attached"})
			},
			expectedError: errAlreadyAttached,
		},
		{
			name:     "API error",
			volumeID: 202,
			linodeID: 303,
			setupMocks: func() {
				mockClient.EXPECT().AttachVolume(gomock.Any(), 202, gomock.Any()).Return(nil, errors.New("API error"))
			},
			expectedError: errInternal("attach volume: API error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			err := cs.attachVolume(context.Background(), tc.volumeID, tc.linodeID)

			switch {
			case tc.expectedError == nil && err != nil:
				t.Errorf("expected no error, got %v", err)
			case tc.expectedError != nil && err == nil:
				t.Errorf("expected error %v, got nil", tc.expectedError)
			case tc.expectedError != nil && err != nil:
				if tc.expectedError.Error() != err.Error() {
					t.Errorf("expected error %v, got %v", tc.expectedError, err)
				}
			}
		})
	}
}

func TestGetInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockLinodeClient(ctrl)
	cs := &ControllerServer{
		client: mockClient,
	}

	testCases := []struct {
		name             string
		linodeID         int
		setupMocks       func()
		expectedInstance *linodego.Instance
		expectedError    error
	}{
		{
			name:     "Instance found",
			linodeID: 123,
			setupMocks: func() {
				mockClient.EXPECT().GetInstance(gomock.Any(), 123).Return(&linodego.Instance{
					ID:     123,
					Label:  "test-instance",
					Status: linodego.InstanceRunning,
				}, nil)
			},
			expectedInstance: &linodego.Instance{
				ID:     123,
				Label:  "test-instance",
				Status: linodego.InstanceRunning,
			},
			expectedError: nil,
		},
		{
			name:     "Instance not found",
			linodeID: 456,
			setupMocks: func() {
				mockClient.EXPECT().GetInstance(gomock.Any(), 456).Return(nil, &linodego.Error{
					Code:    http.StatusNotFound,
					Message: "Not Found",
				})
			},
			expectedInstance: nil,
			expectedError:    errInstanceNotFound(456),
		},
		{
			name:     "API error",
			linodeID: 789,
			setupMocks: func() {
				mockClient.EXPECT().GetInstance(gomock.Any(), 789).Return(nil, errors.New("API error"))
			},
			expectedInstance: nil,
			expectedError:    errInternal("get linode instance 789: API error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			instance, err := cs.getInstance(context.Background(), tc.linodeID)

			if err != nil && !reflect.DeepEqual(tc.expectedError, err) {
				t.Errorf("expected error %v, got %v", tc.expectedError, err)
			}

			if !reflect.DeepEqual(tc.expectedInstance, instance) {
				t.Errorf("expected instance %+v, got %+v", tc.expectedInstance, instance)
			}
		})
	}
}

func Test_getRegionFromTopology(t *testing.T) {
	tests := []struct {
		name         string
		requirements *csi.TopologyRequirement
		want         string
	}{
		{
			name:         "Nil requirements",
			requirements: nil,
			want:         "",
		},
		{
			name: "Empty preferred",
			requirements: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{},
			},
			want: "",
		},
		{
			name: "Single preferred topology with region",
			requirements: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							VolumeTopologyRegion: "us-east",
						},
					},
				},
			},
			want: "us-east",
		},
		{
			name: "Multiple preferred topologies",
			requirements: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							VolumeTopologyRegion: "us-east",
						},
					},
					{
						Segments: map[string]string{
							VolumeTopologyRegion: "us-west",
						},
					},
				},
			},
			want: "us-east",
		},
		{
			name: "Preferred topology without region",
			requirements: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"some-key": "some-value",
						},
					},
				},
			},
			want: "",
		},
		{
			name: "Empty preferred, non-empty requisite",
			requirements: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{},
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							VolumeTopologyRegion: "eu-west",
						},
					},
				},
			},
			want: "eu-west",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRegionFromTopology(tt.requirements)
			if got != tt.want {
				t.Errorf("getRegionFromTopology() = %v, want %v", got, tt.want)
			}
		})
	}
}
