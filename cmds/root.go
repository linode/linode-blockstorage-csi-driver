package cmds

import (
	"flag"

	v "github.com/appscode/go/version"
	"github.com/spf13/cobra"
)

// NewRootCmd func
func NewRootCmd(version string) *cobra.Command {

	rootCmd := &cobra.Command{
		Use:               "csi-linode [command]",
		Short:             `Linode CSI plugin`,
		DisableAutoGenTag: true,
		PersistentPreRun: func(c *cobra.Command, args []string) {
		},
	}
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	// ref: https://github.com/kubernetes/kubernetes/issues/17162#issuecomment-225596212
	flag.CommandLine.Parse([]string{})

	rootCmd.AddCommand(NewCmdInit())

	rootCmd.AddCommand(v.NewCmdVersion())

	return rootCmd
}
