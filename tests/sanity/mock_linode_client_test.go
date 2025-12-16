// Copyright 2024 Linode LLC
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package sanity_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/linode/linodego"
	"go.uber.org/mock/gomock"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
)

// setupLinodeClientExpectations configures MockLinodeClient with stateful behavior
//
//nolint:gocognit // just for test setup
func setupLinodeClientExpectations(mock *mocks.MockLinodeClient, store *volumeStore) {
	// CreateVolume
	mock.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts linodego.VolumeCreateOptions) (*linodego.Volume, error) {
			store.mu.Lock()
			defer store.mu.Unlock()

			id := int(store.nextID.Add(1))
			vol := &linodego.Volume{
				ID:     id,
				Label:  opts.Label,
				Region: opts.Region,
				Size:   opts.Size,
				Status: linodego.VolumeActive,
			}
			if opts.LinodeID != 0 {
				vol.LinodeID = &opts.LinodeID
			}
			store.volumes[id] = vol
			return vol, nil
		}).AnyTimes()

	// GetVolume
	mock.EXPECT().GetVolume(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id int) (*linodego.Volume, error) {
			store.mu.RLock()
			defer store.mu.RUnlock()

			vol, ok := store.volumes[id]
			if !ok {
				return nil, &linodego.Error{Code: 404, Message: "volume not found"}
			}
			return vol, nil
		}).AnyTimes()

	// ListVolumes - must respect filter for label
	mock.EXPECT().ListVolumes(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts *linodego.ListOptions) ([]linodego.Volume, error) {
			store.mu.RLock()
			defer store.mu.RUnlock()

			vols := make([]linodego.Volume, 0, len(store.volumes))

			// Parse filter to check for label
			var labelFilter string
			if opts != nil && opts.Filter != "" {
				var filter map[string]string
				if err := json.Unmarshal([]byte(opts.Filter), &filter); err == nil {
					labelFilter = filter["label"]
				}
			}

			for _, v := range store.volumes {
				// If a label filter is specified, only return matching volumes
				if labelFilter != "" && v.Label != labelFilter {
					continue
				}
				vols = append(vols, *v)
			}
			return vols, nil
		}).AnyTimes()

	// DeleteVolume
	mock.EXPECT().DeleteVolume(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id int) error {
			store.mu.Lock()
			defer store.mu.Unlock()

			if _, ok := store.volumes[id]; !ok {
				return &linodego.Error{Code: 404, Message: "volume not found"}
			}
			delete(store.volumes, id)
			return nil
		}).AnyTimes()

	// AttachVolume
	mock.EXPECT().AttachVolume(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, volID int, opts *linodego.VolumeAttachOptions) (*linodego.Volume, error) {
			store.mu.Lock()
			defer store.mu.Unlock()

			vol, ok := store.volumes[volID]
			if !ok {
				return nil, &linodego.Error{Code: 404, Message: "volume not found"}
			}

			// Check if volume is already attached to a different instance
			if vol.LinodeID != nil && *vol.LinodeID != opts.LinodeID {
				return nil, &linodego.Error{Code: 400, Message: "volume already attached to different instance"}
			}

			// If already attached to this instance, this is idempotent - just return
			if vol.LinodeID != nil && *vol.LinodeID == opts.LinodeID {
				return vol, nil
			}

			vol.LinodeID = &opts.LinodeID
			vol.FilesystemPath = fmt.Sprintf("/dev/disk/by-id/scsi-0Linode_Volume_%s", vol.Label)
			return vol, nil
		}).AnyTimes()

	// DetachVolume
	mock.EXPECT().DetachVolume(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, volID int) error {
			store.mu.Lock()
			defer store.mu.Unlock()

			vol, ok := store.volumes[volID]
			if !ok {
				return &linodego.Error{Code: 404, Message: "volume not found"}
			}
			vol.LinodeID = nil
			return nil
		}).AnyTimes()

	// ResizeVolume
	mock.EXPECT().ResizeVolume(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, volID int, size int) error {
			store.mu.Lock()
			defer store.mu.Unlock()

			vol, ok := store.volumes[volID]
			if !ok {
				return &linodego.Error{Code: 404, Message: "volume not found"}
			}
			vol.Size = size
			return nil
		}).AnyTimes()

	// WaitForVolumeStatus
	mock.EXPECT().WaitForVolumeStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, volID int, _ linodego.VolumeStatus, _ int) (*linodego.Volume, error) {
			store.mu.RLock()
			defer store.mu.RUnlock()

			vol, ok := store.volumes[volID]
			if !ok {
				return nil, &linodego.Error{Code: 404, Message: "volume not found"}
			}
			return vol, nil
		}).AnyTimes()

	// WaitForVolumeLinodeID
	mock.EXPECT().WaitForVolumeLinodeID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, volID int, _ *int, _ int) (*linodego.Volume, error) {
			store.mu.RLock()
			defer store.mu.RUnlock()

			vol, ok := store.volumes[volID]
			if !ok {
				return nil, &linodego.Error{Code: 404, Message: "volume not found"}
			}
			return vol, nil
		}).AnyTimes()

	// GetInstance
	mock.EXPECT().GetInstance(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id int) (*linodego.Instance, error) {
			if id == instanceID {
				return store.instance, nil
			}
			return nil, &linodego.Error{Code: 404, Message: "instance not found"}
		}).AnyTimes()

	// ListInstances
	mock.EXPECT().ListInstances(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *linodego.ListOptions) ([]linodego.Instance, error) {
			return []linodego.Instance{*store.instance}, nil
		}).AnyTimes()

	// ListInstanceVolumes
	mock.EXPECT().ListInstanceVolumes(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, instID int, _ *linodego.ListOptions) ([]linodego.Volume, error) {
			store.mu.RLock()
			defer store.mu.RUnlock()

			vols := make([]linodego.Volume, 0)
			for _, v := range store.volumes {
				if v.LinodeID != nil && *v.LinodeID == instID {
					vols = append(vols, *v)
				}
			}
			return vols, nil
		}).AnyTimes()

	// ListInstanceDisks
	mock.EXPECT().ListInstanceDisks(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ int, _ *linodego.ListOptions) ([]linodego.InstanceDisk, error) {
			return []linodego.InstanceDisk{
				{ID: 1, Label: "boot", Size: 25600},
			}, nil
		}).AnyTimes()

	// CloneVolume
	mock.EXPECT().CloneVolume(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, volID int, label string) (*linodego.Volume, error) {
			store.mu.Lock()
			defer store.mu.Unlock()

			srcVol, ok := store.volumes[volID]
			if !ok {
				return nil, &linodego.Error{Code: 404, Message: "volume not found"}
			}

			id := int(store.nextID.Add(1))
			vol := &linodego.Volume{
				ID:     id,
				Label:  label,
				Region: srcVol.Region,
				Size:   srcVol.Size,
				Status: linodego.VolumeActive,
			}
			store.volumes[id] = vol
			return vol, nil
		}).AnyTimes()

	// GetRegion
	mock.EXPECT().GetRegion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, regionID string) (*linodego.Region, error) {
			return &linodego.Region{ID: regionID}, nil
		}).AnyTimes()

	// NewEventPoller
	mock.EXPECT().NewEventPoller(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
}
