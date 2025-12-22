// Copyright 2024 Linode LLC
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package sanity_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sanity "github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/mount-utils"

	"github.com/linode/linode-blockstorage-csi-driver/internal/driver"
	"github.com/linode/linode-blockstorage-csi-driver/mocks"
	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

const (
	driverName    = "linodebs.csi.linode.com"
	vendorVersion = "test"
	instanceID    = 12345
	region        = "us-east"
	// Minimum volume size for Linode is 10 GiB
	minVolumeSize = 10 << 30
)

func TestSanity(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Test panicked: %v", r)
		}
	}()

	ctx := context.Background()
	tmpDir := t.TempDir()

	endpoint := fmt.Sprintf("unix://%s/csi.sock", tmpDir)
	targetPath := filepath.Join(tmpDir, "target")
	stagingPath := filepath.Join(tmpDir, "staging")

	// Setup gomock controller
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Create volume store for stateful mock behavior
	store := newVolumeStore()

	// Create all mocks
	mockLinodeClient := mocks.NewMockLinodeClient(mockCtrl)
	mockMounter := mocks.NewMockMounter(mockCtrl)
	mockExecutor := mocks.NewMockExecutor(mockCtrl)
	mockFormater := mocks.NewMockFormater(mockCtrl)
	mockDeviceUtils := mocks.NewMockDeviceUtils(mockCtrl)
	mockResizeFS := mocks.NewMockResizeFSer(mockCtrl)
	mockFileSystem := mocks.NewMockFileSystem(mockCtrl)
	mockCryptSetup := mocks.NewMockCryptSetupClient(mockCtrl)
	mockFsStatter := mocks.NewMockFilesystemStatter(mockCtrl)

	mockHwInfo := mocks.NewMockHardwareInfo(mockCtrl)

	// Setup mock expectations
	setupLinodeClientExpectations(mockLinodeClient, store)
	setupMockExpectations(mockCtrl, mockMounter, mockExecutor, mockFormater, mockDeviceUtils, mockResizeFS, mockFileSystem, mockCryptSetup, mockFsStatter)
	setupHardwareInfoExpectations(mockHwInfo)

	// Create SafeFormatAndMount with mocks
	mounter := &mountmanager.SafeFormatAndMount{
		SafeFormatAndMount: &mount.SafeFormatAndMount{
			Interface: mockMounter,
			Exec:      mockExecutor,
		},
		Formater: mockFormater,
	}

	// Create encryption with mocks
	encryption := driver.NewLuksEncryption(mockExecutor, mockFileSystem, mockCryptSetup)

	// Setup metadata
	metadata := driver.Metadata{
		ID:     instanceID,
		Label:  fmt.Sprintf("linode%d", instanceID),
		Region: region,
		Memory: 4 << 30, // 4 GiB
	}

	// Create and setup the driver
	linodeDriver := driver.GetLinodeDriver(ctx)

	// Setup driver
	err := linodeDriver.SetupLinodeDriver(
		ctx,
		mockLinodeClient,
		mounter,
		mockDeviceUtils,
		mockResizeFS,
		metadata,
		driverName,
		vendorVersion,
		"",
		encryption,
		"false", // enableMetrics
		"",
		"false", // enableTracing
		"",
		mockFsStatter,
		mockHwInfo,
	)
	if err != nil {
		t.Fatalf("Failed to setup driver: %v", err)
	}

	// Start the driver
	go linodeDriver.Run(ctx, endpoint)

	// Configure sanity tests
	config := sanity.TestConfig{
		TargetPath:                targetPath,
		StagingPath:               stagingPath,
		Address:                   endpoint,
		DialOptions:               []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		IDGen:                     sanity.DefaultIDGenerator{},
		TestVolumeSize:            minVolumeSize,
		TestVolumeAccessType:      "mount",
		CreateTargetDir:           createDir,
		CreateStagingDir:          createDir,
		RemoveTargetPath:          os.RemoveAll,
		RemoveStagingPath:         os.RemoveAll,
		TestNodeVolumeAttachLimit: true,
	}

	// Run sanity tests
	sanity.Test(t, config)
}
