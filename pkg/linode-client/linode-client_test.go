package linodeclient

import (
	"reflect"
	"testing"

	"github.com/linode/linodego"
)

func TestNewLinodeClient(t *testing.T) {
	type args struct {
		token  string
		ua     string
		apiURL string
	}
	tests := []struct {
		name    string
		args    args
		want    *linodego.Client
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLinodeClient(tt.args.token, tt.args.ua, tt.args.apiURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLinodeClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLinodeClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getAPIURLComponents(t *testing.T) {
	type args struct {
		apiURL string
	}
	tests := []struct {
		name        string
		args        args
		wantHost    string
		wantVersion string
		wantErr     bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotVersion, err := getAPIURLComponents(tt.args.apiURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAPIURLComponents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotHost != tt.wantHost {
				t.Errorf("getAPIURLComponents() gotHost = %v, want %v", gotHost, tt.wantHost)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("getAPIURLComponents() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}
