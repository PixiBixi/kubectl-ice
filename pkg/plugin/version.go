package plugin

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var versionsShort = "Display container versions and mount points"

var helpTemplate = `
{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
More information and examples at: https://github.com/PixiBixi/kubectl-ice

`

func Version(cmd *cobra.Command, kubeFlags *genericclioptions.ConfigFlags, args []string) error {
	// 1234567890123456789012345678901234567890123456789012345678901234567890123456789
	fmt.Printf(`kubectl-ice kubernetes container viewer

version %s

the latest version can be found at:
	https://github.com/PixiBixi/kubectl-ice/releases

to raise issues:
	https://github.com/PixiBixi/kubectl-ice/issues

this is a PixiBixi maintained fork of NimbleArchitect/kubectl-ice, Apache 2.0


if your just after the version string use: kubectl-ice -v

`, cmd.Parent().Version)
	return nil
}
