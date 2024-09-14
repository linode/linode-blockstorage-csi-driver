package cryptsetupclient

import "github.com/martinjungblut/go-cryptsetup"

type Device interface {
	Format(cryptsetup.DeviceType, cryptsetup.GenericParams) error
	KeyslotAddByVolumeKey(int, string, string) error
	ActivateByPassphrase(deviceName string, keyslot int, passphrase string, flags int) error
	Load(cryptsetup.DeviceType) error
	Free() bool
	Dump() int
	Deactivate(string) error
}

func NewInitDevice(path string) (Device, error) {
	d, err := cryptsetup.Init(path)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func NewInitByNameDevice(path string) (Device, error) {
	d, err := cryptsetup.InitByName(path)
	if err != nil {
		return nil, err
	}
	return d, nil
}
