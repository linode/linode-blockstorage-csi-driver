/*
Copyright 2019 linkyard ag
Copyright cloudscale.ch
Copyright 2022 Akamai Technologies

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

*/

// luks utilities from https://github.com/cloudscale-ch/csi-cloudscale/blob/master/driver/luks_util.go with some modifications for this driver

package driver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"

	cryptsetupclient "github.com/linode/linode-blockstorage-csi-driver/pkg/cryptsetup-client"
	cryptsetup "github.com/martinjungblut/go-cryptsetup"

	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
)

type LuksContext struct {
	EncryptionEnabled bool
	EncryptionKey     string
	EncryptionCipher  string
	EncryptionKeySize string
	VolumeName        string
	VolumeLifecycle   VolumeLifecycle
}

const (
	// LuksEncryptedAttribute is used to pass the information if the volume should be
	// encrypted with luks to `NodeStageVolume`
	LuksEncryptedAttribute = Name + "/luks-encrypted"

	// LuksCipherAttribute is used to pass the information about the luks encryption
	// cipher to `NodeStageVolume`
	LuksCipherAttribute = Name + "/luks-cipher"

	// LuksKeySizeAttribute is used to pass the information about the luks key size
	// to `NodeStageVolume`
	LuksKeySizeAttribute = Name + "/luks-key-size"

	// LuksKeyAttribute is the key of the luks key used in the map of secrets passed from the CO
	LuksKeyAttribute = "luksKey"
)

func (ctx *LuksContext) validate() error {
	if !ctx.EncryptionEnabled {
		return nil
	}

	var err error
	if ctx.VolumeName == "" {
		err = errors.Join(err, errors.New("no volume name provided"))
	}
	if ctx.EncryptionKey == "" {
		err = errors.Join(err, errors.New("no encryption key provided"))
	}
	if ctx.EncryptionCipher == "" {
		err = errors.Join(err, errors.New("no encryption cipher provided"))
	}
	if ctx.EncryptionKeySize == "" {
		err = errors.Join(err, errors.New("no encryption key size provided"))
	}

	return err
}

type Encryption struct {
	Exec       Executor
	FileSystem mountmanager.FileSystem
}

func NewLuksEncryption(executor utilexec.Interface, fileSystem mountmanager.FileSystem) Encryption {
	return Encryption{
		Exec:       executor,
		FileSystem: fileSystem,
	}
}

func getLuksContext(secrets map[string]string, context map[string]string, lifecycle VolumeLifecycle) LuksContext {
	if context[LuksEncryptedAttribute] != "true" {
		return LuksContext{
			EncryptionEnabled: false,
			VolumeLifecycle:   lifecycle,
		}
	}

	luksKey := secrets[LuksKeyAttribute]
	luksCipher := context[LuksCipherAttribute]
	luksKeySize := context[LuksKeySizeAttribute]
	volumeName := context[PublishInfoVolumeName]

	return LuksContext{
		EncryptionEnabled: true,
		EncryptionKey:     luksKey,
		EncryptionCipher:  luksCipher,
		EncryptionKeySize: luksKeySize,
		VolumeName:        volumeName,
		VolumeLifecycle:   lifecycle,
	}
}

func (e *Encryption) luksFormat(ctx LuksContext, source string) (string, error) {
	luks2 := cryptsetup.LUKS2{SectorSize: 512}
	keySize, err := strconv.Atoi(ctx.EncryptionKeySize)
	if err != nil {
		return "", fmt.Errorf("line 127: %w", err)
	}
	cipherString := strings.SplitN(ctx.EncryptionCipher, "-", 2)
	genericParams := cryptsetup.GenericParams{
		Cipher:        cipherString[0],
		CipherMode:    cipherString[1],
		VolumeKeySize: keySize / 8,
	}
	device, err := cryptsetupclient.NewInitDevice(source)
	if err != nil {
		return "", fmt.Errorf("line 137: %w", err)
	}
	err = device.Format(luks2, genericParams)
	if err != nil {
		return "", fmt.Errorf("line 141: %w", err)
	}
	err = device.KeyslotAddByVolumeKey(0, "", "")
	if err != nil {
		return "", fmt.Errorf("line 145: %w", err)
	}
	defer device.Free()
	err = device.Load(nil)
	if err != nil {
		return "", fmt.Errorf("line 150: %w", err)
	}
	// err = device.ActivateByPassphrase(ctx.VolumeName, 0, "", 0)
	// if err != nil {
	// 	return "", fmt.Errorf("line 154: %w", err)
	// }
	klog.V(4).Info("The volume has been LUKS formatted ", ctx.VolumeName)
	return "/dev/mapper/" + ctx.VolumeName, nil
}

func (e *Encryption) luksClose(ctx context.Context, volumeName string) error {
	// Initialize the device by name
	klog.V(4).Info("Initalizing device to perform luks close ", volumeName)
	device, err := cryptsetupclient.NewInitByNameDevice(volumeName)
	if err != nil {
		return err
	}
	klog.V(4).Info("Initalized device to perform luks close ", volumeName)

	// Releasing/Freeing the device
	klog.V(4).Info("Releasing/Freeing the device ", volumeName)
	if !device.Free() {
		return errors.New("could not release/free the luks device")
	}
	klog.V(4).Info("Released/Freed the device ", volumeName)

	klog.V(4).Info("Deactivating and closing the volume ", volumeName)
	if err := device.Deactivate(volumeName); err != nil {
		return err
	}
	klog.V(4).Info("Released/Freed and Deactivated/Closed the volume ", volumeName)
	return nil
}
