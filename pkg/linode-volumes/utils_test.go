package linodevolumes

import (
	"reflect"
	"testing"
)

func Test_hashStringToInt(t *testing.T) {
	type args struct {
		b string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hashStringToInt(tt.args.b); got != tt.want {
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
		// TODO: Add test cases.
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
		// TODO: Add test cases.
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

func TestParseLinodeVolumeKey(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		want    *LinodeVolumeKey
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLinodeVolumeKey(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLinodeVolumeKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLinodeVolumeKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
