package linodevolumes

import (
	"testing"
)

func Test_hashStringToInt(t *testing.T) {
	tests := []struct {
		name string
		b    string
		want int
	}{
		{
			name: "Empty string",
			b:    "",
			want: 2166136261,
		},
		{
			name: "Single character",
			b:    "a",
			want: 3826002220,
		},
		{
			name: "Multiple characters",
			b:    "abc",
			want: 440920331,
		},
		{
			name: "Long string",
			b:    "This is a long string with various characters!@#$%^&*()",
			want: 1883843634,
		},
		{
			name: "Unicode characters",
			b:    "こんにちは世界",
			want: 3937201063,
		},
		{
			name: "Numeric string",
			b:    "12345",
			want: 1136836824,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashStringToInt(tt.b)
			if got != tt.want {
				t.Errorf("hashStringToInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeIdAsInt(t *testing.T) {
	type args struct {
		caller string
		w      withVolume
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "Invalid numeric volume ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithVolume{
					volumeID: "12345",
				},
			},
			want:    hashStringToInt("12345"),
			wantErr: false,
		},
		{
			name: "Valid string volume ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithVolume{
					volumeID: "123-pvc23232",
				},
			},
			want:    123,
			wantErr: false,
		},
		{
			name: "Empty volume ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithVolume{
					volumeID: "",
				},
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "Invalid volume ID (non-numeric and non-string)",
			args: args{
				caller: "TestCaller",
				w: &mockWithVolume{
					volumeID: "invalid-id-!@#$%",
				},
			},
			want:    hashStringToInt("invalid-id-!@#$%"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := VolumeIdAsInt(tt.args.caller, tt.args.w)
			if (err != nil) != tt.wantErr {
				t.Errorf("VolumeIdAsInt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("VolumeIdAsInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockWithVolume struct {
	volumeID string
}

func (m *mockWithVolume) GetVolumeId() string {
	return m.volumeID
}

func TestNodeIdAsInt(t *testing.T) {
	type args struct {
		caller string
		w      withNode
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{
			name: "Valid numeric node ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithNode{
					nodeID: "123",
				},
			},
			want:    123,
			wantErr: false,
		},
		{
			name: "Empty node ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithNode{
					nodeID: "",
				},
			},
			want:    0,
			wantErr: true,
		},
		{
			name: "Non-numeric node ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithNode{
					nodeID: "non-numeric",
				},
			},
			want:    hashStringToInt("non-numeric"),
			wantErr: false,
		},
		{
			name: "Large numeric node ID",
			args: args{
				caller: "TestCaller",
				w: &mockWithNode{
					nodeID: "9223372036854775807", // Max int64
				},
			},
			want:    9223372036854775807,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NodeIdAsInt(tt.args.caller, tt.args.w)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeIdAsInt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NodeIdAsInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockWithNode struct {
	nodeID string
}

func (m *mockWithNode) GetNodeId() string {
	return m.nodeID
}

func TestCreateLinodeVolumeKey(t *testing.T) {
	key := CreateLinodeVolumeKey(123, "test-volume")
	if key.VolumeID != 123 {
		t.Errorf("Expected VolumeID 123, got %d", key.VolumeID)
	}
	if key.Label != "test-volume" {
		t.Errorf("Expected Label 'test-volume', got '%s'", key.Label)
	}
}

func TestGetVolumeID(t *testing.T) {
	key := LinodeVolumeKey{VolumeID: 456, Label: "sample-volume"}
	if key.GetVolumeID() != 456 {
		t.Errorf("Expected VolumeID 456, got %d", key.GetVolumeID())
	}
}

func TestGetVolumeLabel(t *testing.T) {
	key := LinodeVolumeKey{VolumeID: 789, Label: "another-volume"}
	if key.GetVolumeLabel() != "another-volume" {
		t.Errorf("Expected Label 'another-volume', got '%s'", key.GetVolumeLabel())
	}
}

func TestGetNormalizedLabel(t *testing.T) {
	testCases := []struct {
		name     string
		label    string
		expected string
	}{
		{
			name:     "Short label",
			label:    "short-label",
			expected: "short-label",
		},
		{
			name:     "Exact length label",
			label:    "exactly-32-characters-long-label",
			expected: "exactly-32-characters-long-label",
		},
		{
			name:     "Long label",
			label:    "this-label-is-definitely-longer-than-32-characters",
			expected: "this-label-is-definitely-longer-",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := LinodeVolumeKey{VolumeID: 1, Label: tc.label}
			result := key.GetNormalizedLabel()
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestGetNormalizedLabelWithPrefix(t *testing.T) {
	testCases := []struct {
		name     string
		label    string
		prefix   string
		expected string
	}{
		{
			name:     "Short label with prefix",
			label:    "short-label",
			prefix:   "prefix-",
			expected: "prefix-short-label",
		},
		{
			name:     "Long label with short prefix",
			label:    "this-label-is-definitely-longer-than-32-characters",
			prefix:   "px-",
			expected: "px-this-label-is-definitely-long",
		},
		{
			name:     "Short label with long prefix",
			label:    "short",
			prefix:   "very-long-prefix-that-exceeds-",
			expected: "very-long-prefix-that-exceeds-sh",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := LinodeVolumeKey{VolumeID: 1, Label: tc.label}
			result := key.GetNormalizedLabelWithPrefix(tc.prefix)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestGetVolumeKey(t *testing.T) {
	testCases := []struct {
		name     string
		volumeID int
		label    string
		expected string
	}{
		{
			name:     "Short label",
			volumeID: 123,
			label:    "short-label",
			expected: "123-short-label",
		},
		{
			name:     "Long label",
			volumeID: 456,
			label:    "this-label-is-definitely-longer-than-32-characters",
			expected: "456-this-label-is-definitely-longer-",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := LinodeVolumeKey{VolumeID: tc.volumeID, Label: tc.label}
			result := key.GetVolumeKey()
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}
