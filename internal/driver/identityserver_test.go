/*
Copyright 2018 The Kubernetes Authors.

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

package driver

import (
	"context"
	"reflect"
	"testing"
)

func TestNewIdentityServer(t *testing.T) {
	type args struct {
		linodeDriver *LinodeDriver
	}
	tests := []struct {
		name    string
		args    args
		want    *IdentityServer
		wantErr bool
	}{
		{
			name: "Success",
			args: args{
				linodeDriver: &LinodeDriver{},
			},
			want: &IdentityServer{
				driver: &LinodeDriver{},
			},
			wantErr: false,
		},
		{
			name: "Nil linodeDriver",
			args: args{
				linodeDriver: nil,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewIdentityServer(context.Background(), tt.args.linodeDriver)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewIdentityServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewIdentityServer() = %v, want %v", got, tt.want)
			}
		})
	}
}
