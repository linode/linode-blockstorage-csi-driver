// Copyright 2024 Linode LLC
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package sanity_test

import (
	"io"
	"os"
)

func createDir(p string) (string, error) {
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// fakeCmd implements exec.Cmd for mock executor
type fakeCmd struct{}

func (f *fakeCmd) CombinedOutput() ([]byte, error)    { return nil, nil }
func (f *fakeCmd) Output() ([]byte, error)            { return nil, nil }
func (f *fakeCmd) SetDir(dir string)                  {}
func (f *fakeCmd) SetStdin(in io.Reader)              {}
func (f *fakeCmd) SetStdout(out io.Writer)            {}
func (f *fakeCmd) SetStderr(out io.Writer)            {}
func (f *fakeCmd) SetEnv(env []string)                {}
func (f *fakeCmd) StdoutPipe() (io.ReadCloser, error) { return io.NopCloser(nil), nil }
func (f *fakeCmd) StderrPipe() (io.ReadCloser, error) { return io.NopCloser(nil), nil }
func (f *fakeCmd) Start() error                       { return nil }
func (f *fakeCmd) Wait() error                        { return nil }
func (f *fakeCmd) Run() error                         { return nil }
func (f *fakeCmd) Stop()                              {}
