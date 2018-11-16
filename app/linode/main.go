/*
Copyright 2017 The Kubernetes Authors.
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

package main

import (
	"os"

	"flag"

	"github.com/spf13/pflag"

	logs "github.com/appscode/go/log/golog"
	"github.com/golang/glog"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/linode-bs"
	"github.com/spf13/cobra"
)

// Config Linode Client Config
type Config struct {
	Endpoint string
	Token    string
	URL      string
	Region   string
	NodeName string
}

func NewConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		Endpoint: "unix:///var/lib/kubelet/plugins/linodebs.csi.linode.com/csi.sock",
		URL:      "https://api.linode.com/v4",
		Token:    "",
		Region:   "",
		NodeName: hostname,
	}
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	if err := NewRootCmd(Version).Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func NewCmdInit() *cobra.Command {
	cfg := NewConfig()
	cmd := &cobra.Command{
		Use:               "init",
		Short:             "Initializes the driver.",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			drv, err := driver.NewDriver(cfg.Endpoint, cfg.Token, cfg.Region, cfg.NodeName, &cfg.URL)
			if err != nil {
				glog.Fatalln(err)
			}

			if err := drv.Run(); err != nil {
				glog.Fatalln(err)
			}
		},
	}
	cfg.AddFlags(cmd.Flags())
	return cmd
}

func NewRootCmd(version string) *cobra.Command {

	rootCmd := &cobra.Command{
		Use:               "linode-blockstorage-csi-driver [command]",
		Short:             `Linode CSI plugin`,
		DisableAutoGenTag: true,
		PersistentPreRun: func(c *cobra.Command, args []string) {
		},
	}
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	// ref: https://github.com/kubernetes/kubernetes/issues/17162#issuecomment-225596212
	flag.CommandLine.Parse([]string{})

	rootCmd.AddCommand(NewCmdInit())

	// rootCmd.AddCommand(v.NewCmdVersion())

	return rootCmd
}

func (c *Config) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Endpoint, "endpoint", c.Endpoint, "CSI endpoint")
	fs.StringVar(&c.Token, "token", c.Token, "Linode API Token")
	fs.StringVar(&c.URL, "url", c.URL, "Linode API URL")
	fs.StringVar(&c.Region, "region", c.Region, "Linode Region")
	fs.StringVar(&c.NodeName, "node", c.NodeName, "Linode Hostname")
}
