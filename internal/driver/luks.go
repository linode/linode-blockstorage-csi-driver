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
	"os/exec"
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

	// Set params
	keySize, err := strconv.Atoi(luksCtx.EncryptionKeySize)
	if err != nil {
		return "", fmt.Errorf("keysize str to int coversion: %w", err)
	}
	cipherString := strings.SplitN(luksCtx.EncryptionCipher, "-", 2)
	genericParams := cryptsetup.GenericParams{
		Cipher:        cipherString[0],
		CipherMode:    cipherString[1],
		VolumeKey:     luksCtx.EncryptionKey,
		VolumeKeySize: keySize / 8,
	}

	// Initialize the device using the path
	log.V(4).Info("Initializing device to perform luks format ", source)
	newLuksDevice, err := cryptsetupclient.NewLuksDevice(e.CryptSetup, source)
	if err != nil {
		return "", fmt.Errorf("initializing luks device to format: %w", err)
	}

	// Format the device
	log.V(4).Info("Formatting luks device ", newLuksDevice.Identifier)
	err = newLuksDevice.Device.Format(cryptsetup.LUKS2{SectorSize: 512}, genericParams)
	if err != nil {
		return "", fmt.Errorf("formatting luks device: %w", err)
	}

	// Add keysot by volumekey
	log.V(4).Info("Adding keysot by volumekey")
	if err := newLuksDevice.Device.KeyslotAddByVolumeKey(0, "", luksCtx.EncryptionKey); err != nil {
		return "", fmt.Errorf("adding keysot by volumekey: %w", err)
	}
	defer newLuksDevice.Device.Free()

	// Activate the device using the encryption key
	log.V(4).Info("Activating luks device using volumekey", "device", newLuksDevice.Identifier, "VolumeName", luksCtx.VolumeName)
	err = newLuksDevice.Device.ActivateByPassphrase(luksCtx.VolumeName, 0, luksCtx.EncryptionKey, 0)
	if err != nil {
		return "", fmt.Errorf("activating %s luks device %s volumekey %s: %w", newLuksDevice.Identifier, luksCtx.VolumeName, luksCtx.EncryptionKey, err)
	}
	log.V(4).Info("The LUKS volume is now ready ", luksCtx.VolumeName)

	// Return the mapper path
	return "/dev/mapper/" + luksCtx.VolumeName, nil
}

func (e *Encryption) luksOpen(ctx context.Context, luksCtx LuksContext, source string) (string, error) {
	log := logger.GetLogger(ctx)

	// Initialize the device using the path
	log.V(4).Info("Initializing device to perform luks open ", source)
	newLuksDevice, err := cryptsetupclient.NewLuksDevice(e.CryptSetup, source)
	if err != nil {
		return "", fmt.Errorf("initializing luks device to open: %w", err)
	}
	defer newLuksDevice.Device.Free()

	// Loading the device
	log.V(4).Info("Loading luks device", "device", newLuksDevice.Identifier, "VolumeName", luksCtx.VolumeName)
	err = newLuksDevice.Device.Load(cryptsetup.LUKS2{SectorSize: 512})
	if err != nil {
		return "", fmt.Errorf("Loading %s luks device %s: %w", newLuksDevice.Identifier, luksCtx.VolumeName, luksCtx.EncryptionKey, err)
	}

	// Activate the device using the encryption key
	log.V(4).Info("Activating luks device using volumekey", "device", newLuksDevice.Identifier, "VolumeName", luksCtx.VolumeName)
	if err := newLuksDevice.Device.ActivateByPassphrase(luksCtx.VolumeName, 0, luksCtx.EncryptionKey, 0); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return "/dev/mapper/" + luksCtx.VolumeName, nil
		}
		return "", fmt.Errorf("activating %s luks device %s volumekey %s: %w", newLuksDevice.Identifier, luksCtx.VolumeName, luksCtx.EncryptionKey, err)
	}

	log.V(4).Info("The LUKS volume is now ready ", luksCtx.VolumeName)

	// Return the mapper path
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

	// Deactivating the device
	log.V(4).Info("Deactivating and closing the volume ", volumeName)
	if err := newLuksDeviceByName.Device.Deactivate(volumeName); err != nil {
		return fmt.Errorf("deactivating %s luks device: %w", volumeName, err)
	}

	// Freeing the device
	log.V(4).Info("Releasing/Freeing the device ", volumeName)
	if !newLuksDeviceByName.Device.Free() {
		return errors.New("could not release/free the luks device")
	}
	log.V(4).Info("Released/Freed the device ", volumeName)

	log.V(4).Info("Released/Freed and Deactivated/Closed the volume ", volumeName)
	return nil
}

func (e *Encryption) blkidValid(source string) (bool, error) {
	if source == "" {
		return false, errors.New("invalid source")
	}

	blkidCmd := "blkid"
	_, err := e.Exec.LookPath(blkidCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("%q executable invalid", blkidCmd)
		}
		return false, err
	}

	blkidArgs := []string{source}

	exitCode := 0
	cmd := e.Exec.Command(blkidCmd, blkidArgs...)
	err = cmd.Run()
	if err != nil {
		exitError, ok := err.(utilexec.ExitError)
		if !ok {
			return false, fmt.Errorf("checking blkdid failed: %w cmd: %q, args: %q", err, blkidCmd, blkidArgs)
		}
		// ws := exitError.Sys().(syscall.WaitStatus)
		exitCode = exitError.ExitStatus()
		if exitCode == 2 {
			return false, nil
		}
		return false, errors.New("checking blkdid failed")
	}

	return true, nil
}
