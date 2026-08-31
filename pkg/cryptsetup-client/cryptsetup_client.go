package cryptsetupclient

import (
	"fmt"

	"github.com/martinjungblut/go-cryptsetup"
)

type Device interface {
	Format(cryptsetup.DeviceType, cryptsetup.GenericParams) error
	KeyslotAddByVolumeKey(int, string, string) error
	ActivateByVolumeKey(deviceName string, volumeKey string, volumeKeySize int, flags int) error
	ActivateByPassphrase(deviceName string, keyslot int, passphrase string, flags int) error
	VolumeKeyGet(keyslot int, passphrase string) ([]byte, int, error)
	Load(cryptsetup.DeviceType) error
	Free() bool
	Dump() int
	Type() string
	Deactivate(string) error
}

type CryptSetupClient interface {
	Init(string) (Device, error)
	InitByName(string) (Device, error)
}

// CryptSetup manages encrypted devices.
type CryptSetup struct {
	_ CryptSetupClient
}

// Init opens a crypt device by device path.
func (c CryptSetup) Init(devicePath string) (Device, error) {
	device, err := cryptsetup.Init(devicePath)
	if err != nil {
		return nil, fmt.Errorf("init cryptsetup by device path %q: %w", devicePath, err)
	}
	return device, nil
}

// InitByName opens an active crypt device using its mapped name.
func (c CryptSetup) InitByName(name string) (Device, error) {
	device, err := cryptsetup.InitByName(name)
	if err != nil {
		return nil, fmt.Errorf("init cryptsetup by name %q: %w", name, err)
	}
	return device, nil
}

type LuksDevice struct {
	Identifier string
	Device     Device
}

func NewLuksDevice(crypt CryptSetupClient, path string) (LuksDevice, error) {
	dev, err := crypt.Init(path)
	if err != nil {
		return LuksDevice{}, err
	}
	return LuksDevice{Identifier: path, Device: dev}, nil
}

func NewLuksDeviceByName(crypt CryptSetupClient, name string) (LuksDevice, error) {
	dev, err := crypt.InitByName(name)
	if err != nil {
		return LuksDevice{}, err
	}
	return LuksDevice{Identifier: name, Device: dev}, nil
}

func NewCryptSetup() CryptSetup {
	return CryptSetup{}
}
