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

	utilexec "k8s.io/utils/exec"

	cryptsetup "github.com/martinjungblut/go-cryptsetup"

	cryptsetupclient "github.com/linode/linode-blockstorage-csi-driver/pkg/cryptsetup-client"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
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
	CryptSetup cryptsetupclient.CryptSetupClient
}

func NewLuksEncryption(executor utilexec.Interface, fileSystem mountmanager.FileSystem, cryptSetup cryptsetupclient.CryptSetupClient) Encryption {
	return Encryption{
		Exec:       executor,
		FileSystem: fileSystem,
		CryptSetup: cryptSetup,
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

func (e *Encryption) luksFormat(ctx context.Context, luksCtx LuksContext, source string) (string, error) {
	log := logger.GetLogger(ctx)
	luks2 := cryptsetup.LUKS2{SectorSize: 512}
	keySize, err := strconv.Atoi(luksCtx.EncryptionKeySize)
	if err != nil {
		return "", fmt.Errorf("keysize str to int coversion: %w", err)
	}
	cipherString := strings.SplitN(luksCtx.EncryptionCipher, "-", 2)
	genericParams := cryptsetup.GenericParams{
		Cipher:        cipherString[0],
		CipherMode:    cipherString[1],
		VolumeKeySize: keySize / 8,
	}
	log.V(4).Info("Initalizing device to perform luks format ", source)

	newLuksDevice, err := cryptsetupclient.NewLuksDevice(e.CryptSetup, source)
	if err != nil {
		return "", fmt.Errorf("initializing luks device to format: %w", err)
	}

	log.V(4).Info("Check if the device is already formatted ", newLuksDevice.Identifier)
	if err := newLuksDevice.Device.Load(luks2); err == nil {
		log.V(4).Info("Device is already formatted ", newLuksDevice.Identifier)
		return "/dev/mapper/" + luksCtx.VolumeName, nil
	}
	log.V(4).Info("Device is not formatted yet... Lets format it ", newLuksDevice.Identifier)

	log.V(4).Info("Formatting luks device ", newLuksDevice.Identifier)
	err = newLuksDevice.Device.Format(luks2, genericParams)
	if err != nil {
		return "", fmt.Errorf("formatting luks device: %w", err)
	}
	log.V(4).Info("Add keyslot to luks device ", newLuksDevice.Identifier)
	err = newLuksDevice.Device.KeyslotAddByVolumeKey(0, "", "")
	if err != nil {
		return "", fmt.Errorf("adding luks keyslot: %w", err)
	}
	defer newLuksDevice.Device.Free()
	log.V(4).Info("Loading luks device ", newLuksDevice.Identifier)
	err = newLuksDevice.Device.Load(luks2)
	if err != nil {
		return "", fmt.Errorf("loading luks device: %w", err)
	}
	log.V(4).Info("Activating luks device ", "device", newLuksDevice.Identifier, "VolumeName", luksCtx.VolumeName)
	err = newLuksDevice.Device.ActivateByPassphrase(luksCtx.VolumeName, 0, "", 0)
	if err != nil {
		return "", fmt.Errorf("activating %s luks device %s by passphrase: %w", newLuksDevice.Identifier, luksCtx.VolumeName, err)
	}
	log.V(4).Info("The volume has been LUKS formatted ", luksCtx.VolumeName)
	return "/dev/mapper/" + luksCtx.VolumeName, nil
}

func (e *Encryption) luksClose(ctx context.Context, volumeName string) error {
	log := logger.GetLogger(ctx)
	// Initialize the device by name
	log.V(4).Info("Initalizing device to perform luks close ", volumeName)
	newLuksDeviceByName, err := cryptsetupclient.NewLuksDeviceByName(e.CryptSetup, volumeName)
	if err != nil {
		log.V(4).Info("device is no longer active ", volumeName)
		return nil
	}
	log.V(4).Info("Initalized device to perform luks close ", volumeName)

	// Releasing/Freeing the device
	log.V(4).Info("Releasing/Freeing the device ", volumeName)
	if !newLuksDeviceByName.Device.Free() {
		return errors.New("could not release/free the luks device")
	}
	log.V(4).Info("Released/Freed the device ", volumeName)

	log.V(4).Info("Deactivating and closing the volume ", volumeName)
	if err := newLuksDeviceByName.Device.Deactivate(volumeName); err != nil {
		return fmt.Errorf("deactivating %s luks device: %w", volumeName, err)
	}
	log.V(4).Info("Released/Freed and Deactivated/Closed the volume ", volumeName)
	return nil
}
