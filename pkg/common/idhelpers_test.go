package common_test

import (
	"testing"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/common"
)

func TestGetNormalizedLabelWithPrefix(t *testing.T) {
	key := common.CreateLinodeVolumeKey(123, "foobar")
	prefixed := key.GetNormalizedLabelWithPrefix("prefix-")

	if prefixed != "prefix-foobar" {
		t.Errorf("Expected prefixed volume label, got %q", prefixed)
	}
}

func TestGetVolumeKey(t *testing.T) {
	key := common.CreateLinodeVolumeKey(123, "foobar")

	label := key.GetNormalizedLabel()

	if label != "foobar" {
		t.Errorf("Unexpected volume label, got %q", label)
	}
}

func TestParseLinodeVolumeKey(t *testing.T) {
	strKey := "123-foobar"
	key, err := common.ParseLinodeVolumeKey(strKey)
	if err != nil {
		t.Errorf("Error parsing volume key: %s", err)
	}

	volID := key.GetVolumeID()
	if volID != 123 {
		t.Errorf("Unexpected volume id, got %q", volID)
	}

	volLabel := key.GetVolumeLabel()
	if volLabel != "foobar" {
		t.Errorf("Unexpected volume label, got %q", volLabel)
	}
}
