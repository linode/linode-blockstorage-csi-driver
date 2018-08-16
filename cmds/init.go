package cmds

import (
	"github.com/displague/csi-linode/cmds/options"
	"github.com/displague/csi-linode/driver"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// NewCmdInit func
func NewCmdInit() *cobra.Command {
	cfg := options.NewConfig()
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
