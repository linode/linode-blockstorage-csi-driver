// Copyright 2024 Linode LLC
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package sanity_test

import (
	"sync"
	"sync/atomic"

	"github.com/linode/linodego"
)

// volumeStore provides stateful storage for mock LinodeClient
type volumeStore struct {
	mu                sync.RWMutex
	volumes           map[int]*linodego.Volume
	publishedCapTypes map[int]string // Track capability type per volume (mount/block) - currently unused
	nextID            atomic.Int64
	instance          *linodego.Instance
}

func newVolumeStore() *volumeStore {
	vs := &volumeStore{
		volumes:           make(map[int]*linodego.Volume),
		publishedCapTypes: make(map[int]string),
		instance: &linodego.Instance{
			ID:    instanceID,
			Label: "linode12345",
			Specs: &linodego.InstanceSpec{
				Memory: 4096, // 4 GiB in MB
			},
			Region: region,
		},
	}
	vs.nextID.Store(1000)
	return vs
}
