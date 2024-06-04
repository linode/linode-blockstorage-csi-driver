package driver

import (
	"fmt"
	"testing"
)

func TestMaxVolumeAttachments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		memory uint
		want   int
	}{
		{memory: 1 << 30, want: maxPersistentAttachments},
		{memory: 2 << 30, want: maxPersistentAttachments},
		{memory: 4 << 30, want: maxPersistentAttachments},
		{memory: 8 << 30, want: maxPersistentAttachments},
		{memory: 16 << 30, want: 16},
		{memory: 32 << 30, want: 32},
		{memory: 64 << 30, want: maxAttachments},
		{memory: 96 << 30, want: maxAttachments},
		{memory: 128 << 30, want: maxAttachments},
		{memory: 150 << 30, want: maxAttachments},
		{memory: 256 << 30, want: maxAttachments},
		{memory: 300 << 30, want: maxAttachments},
		{memory: 512 << 30, want: maxAttachments},
	}

	for _, tt := range tests {
		tname := fmt.Sprintf("%dGB", tt.memory>>30)
		t.Run(tname, func(t *testing.T) {
			got := maxVolumeAttachments(tt.memory)
			if got != tt.want {
				t.Errorf("want=%d got=%d", tt.want, got)
			}
		})
	}
}
