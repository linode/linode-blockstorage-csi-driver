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

type CryptSetupClient interface {
	Init(string) (Device, error)
	InitByName(string) (Device, error)
}
