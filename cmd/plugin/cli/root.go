package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/PixiBixi/kubectl-ice/pkg/plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"
)

// auto updated version via gorelaser
var version = "0.0.0"

var rootShort = "View pod information at the container level"

var rootDescription = ` Ice lets you view configuration and settings of the containers that run inside pods.

	Suggestions and improvements can be made by raising an issue here:
    https://github.com/PixiBixi/kubectl-ice/issues

`

func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "kubectl-ice",
		Short:         rootShort,
		Long:          fmt.Sprintf("%s\n\n%s", rootShort, rootDescription),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cobra.OnInitialize(initConfig)

	if strings.ToLower(os.Getenv("ICE_LOG")) == "debug" {
		plugin.LogDebug = true
	}

	plugin.InitSubCommands(cmd)

	return cmd
}

func InitAndExecute() {
	os.Exit(run())
}

// run is separate from InitAndExecute so the deferred stop actually runs: os.Exit
// skips defers.
func run() int {
	// ExecuteContext rather than Execute so an interrupt cancels the request in
	// flight instead of abandoning it server side. NotifyContext stops relaying
	// after the first signal, so a second ctrl-c still kills the process.
	// --request-timeout continues to bound each request on top of this.
	// client-go logs "Unexpected error when reading response body: context
	// canceled" through klog on a cancelled request, which is exactly what an
	// interrupt causes. Our own errors are wrapped and returned, so nothing worth
	// reading is lost by silencing it.
	silenceKlog()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := RootCmd().ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

func initConfig() {
	viper.AutomaticEnv()
}

// silenceKlog sends client-go's internal logging to nowhere. It is transport
// level noise that a plugin user cannot act on.
func silenceKlog() {
	set := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(set)
	_ = set.Set("logtostderr", "false")
	klog.SetOutput(io.Discard)
}
