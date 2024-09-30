package driver

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/filesystem"
)

func TestMemoryToBytes(t *testing.T) {
	tests := []struct {
		input int
		want  uint
	}{
		{input: 1024, want: 1 << 30},     // 1GiB
		{input: 2048, want: 2 << 30},     // 2GiB
		{input: 32768, want: 32 << 30},   // 32GiB
		{input: 196608, want: 192 << 30}, // 192GiB
		{input: 131072, want: 128 << 30}, // 128GiB
		{input: -1, want: minMemory},
	}

	for _, tt := range tests {
		if got := memoryToBytes(tt.input); tt.want != got {
			t.Errorf("%d: want=%d got=%d", tt.input, tt.want, got)
		}
	}
}

func TestGetMetadataFromAPI(t *testing.T) {
	// Make sure GetMetadataFromAPI fails when it's given a nil client.
	t.Run("NilClient", func(t *testing.T) {
		_, err := GetMetadataFromAPI(context.Background(), nil, filesystem.OSFileSystem{})
		if err == nil {
			t.Fatal("should have failed")
		}
		if !errors.Is(err, errNilClient) {
			t.Errorf("wrong error returned\n\twanted=%q\n\tgot=%q", errNilClient, err)
		}
	})

	// GetMetadataFromAPI depends on the LinodeIDPath file to exist, so we
	// should skip the rest of this test if it cannot be found.
	if _, err := os.Stat(LinodeIDPath); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("%s does not exist", LinodeIDPath)
	} else if err != nil {
		t.Fatal(err)
	}
}
