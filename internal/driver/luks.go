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
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"

	mountmanager "github.com/linode/linode-blockstorage-csi-driver/pkg/mount-manager"
	cryptsetup "github.com/martinjungblut/go-cryptsetup"
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
	args := []string{""}
	s, err := e.Exec.Command("lsblk", args...).CombinedOutput()
	klog.V(2).Info("Command output ", s)
	luks2 := cryptsetup.LUKS2{SectorSize: 512}
	keySize, err := strconv.Atoi(ctx.EncryptionKeySize)
	if err != nil {
		return "", err
	}
	cipherString := strings.SplitN(ctx.EncryptionCipher, "-", 2)
	genericParams := cryptsetup.GenericParams{
		Cipher:        cipherString[0],
		CipherMode:    cipherString[1],
		VolumeKeySize: keySize / 8,
	}
	device, err := cryptsetup.Init(source)
	if err != nil {
		return "", err
	}
	err = device.Format(luks2, genericParams)
	if err != nil {
		return "", err
	}
	if device.Dump() == 0 {
		klog.V(4).Info("The volume is already LUKS formatted ", ctx.VolumeName)
		return "/dev/mapper/" + ctx.VolumeName, nil
	}
	err = device.KeyslotAddByVolumeKey(0, "", "")
	if err != nil {
		return "", err
	}
	defer device.Free()
	err = device.Load(nil)
	if err != nil {
		return "", err
	}
	err = device.ActivateByPassphrase(ctx.VolumeName, 0, "", 0)
	if err != nil {
		return "", err
	}
	klog.V(4).Info("The volume has been LUKS formatted ", ctx.VolumeName)
	return "/dev/mapper/" + ctx.VolumeName, nil
}

func (e *Encryption) luksClose(volume string) error {
	cryptsetupCmd, err := e.getCryptsetupCmd()
	if err != nil {
		return err
	}
	cryptsetupArgs := []string{"--batch-mode", "close", volume}

	klog.V(4).Info("executing cryptsetup close command")

	out, err := e.Exec.Command(cryptsetupCmd, cryptsetupArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing luks mapping failed: %w cmd: '%s %s' output: %q",
			err, cryptsetupCmd, strings.Join(cryptsetupArgs, " "), string(out))
	}
	return nil
}

// check is a given mapping under /dev/mapper is a luks volume
func (e *Encryption) isLuksMapping(volume string) (bool, string, error) {
	if strings.HasPrefix(volume, "/dev/mapper/") {
		mappingName := volume[len("/dev/mapper/"):]
		cryptsetupCmd, err := e.getCryptsetupCmd()
		if err != nil {
			return false, mappingName, err
		}
		cryptsetupArgs := []string{"status", mappingName}

		out, err := e.Exec.Command(cryptsetupCmd, cryptsetupArgs...).CombinedOutput()
		if err != nil {
			return false, mappingName, nil
		}
		for _, statusLine := range strings.Split(string(out), "\n") {
			if strings.Contains(statusLine, "type:") {
				if strings.Contains(strings.ToLower(statusLine), "luks") {
					return true, mappingName, nil
				}
				return false, mappingName, nil
			}
		}

	}
	return false, "", nil
}

func (e *Encryption) getCryptsetupCmd() (string, error) {
	cryptsetupCmd := "cryptsetup"
	_, err := e.Exec.LookPath(cryptsetupCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return "", fmt.Errorf("%q executable not found in $PATH", cryptsetupCmd)
		}
		return "", err
	}
	return cryptsetupCmd, nil
}
