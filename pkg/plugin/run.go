package plugin

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// runContext is what the hooks below get to work with. Passing a struct rather
// than four parameters means adding something later does not touch every
// subcommand.
type runContext struct {
	cmd     *cobra.Command
	builder *RowBuilder
	connect *Connector
	flags   commonFlags
	args    []string
}

// subCommand is everything a subcommand contributes to the run sequence. The
// rest of that sequence used to be copied into all thirteen of them, and the
// copies had drifted: one dropped the error from Build, one passed its Looper by
// value, and two kept the --oddities filter in two places that could disagree.
type subCommand struct {
	// loop builds the rows.
	loop Looper

	loopSpec           bool
	loopStatus         bool
	showInitContainers bool
	dontListContainers bool

	// configure reads the subcommand's own flags, after the common flags have
	// been applied to the builder. SetFlagsFrom only ever turns fields on, so
	// running afterwards cannot lose a choice made here.
	configure func(runContext) error

	// filterRows runs on the built table before rendering, for --oddities. It is
	// applied on both the watch and the non watch path from one place.
	filterRows func(runContext) error
}

// runSubCommand wires the connection, flags, table and builder, then builds and
// renders. Every subcommand goes through here.
func runSubCommand(cmd *cobra.Command, kubeFlags *genericclioptions.ConfigFlags, args []string, sub subCommand) error {
	connect := Connector{}
	if err := connect.LoadConfig(cmd.Context(), kubeFlags); err != nil {
		return err
	}

	return runWithConnector(cmd, &connect, args, sub)
}

// runWithConnector is runSubCommand once the connection exists. Split out so a
// test can hand it a Connector backed by client-go's fake and exercise the whole
// sequence, which every subcommand goes through, without an api server.
func runWithConnector(cmd *cobra.Command, connect *Connector, args []string, sub subCommand) error {
	flags, err := processCommonFlags(cmd)
	if err != nil {
		return err
	}
	connect.Flags = flags

	table := Table{
		ColourOutput:  flags.outputAsColour,
		CustomColours: flags.useTheseColours,
	}

	builder := RowBuilder{
		LoopSpec:           sub.loopSpec,
		LoopStatus:         sub.loopStatus,
		ShowInitContainers: sub.showInitContainers,
		DontListContainers: sub.dontListContainers,
		// PodName has to be set before SetFlagsFrom, which reads it to decide
		// whether the pod name column is worth showing.
		PodName:    args,
		Table:      &table,
		Connection: connect,
	}
	builder.SetFlagsFrom(flags)

	run := runContext{cmd: cmd, builder: &builder, connect: connect, flags: flags, args: args}

	if sub.configure != nil {
		if err := sub.configure(run); err != nil {
			return err
		}
	}

	// finish is everything between a built table and a printed one. Both paths go
	// through it, so --sort and --oddities apply identically whether the table
	// was built once or is being rebuilt on a pod event.
	finish := func() error {
		if err := builder.Table.SortByNames(flags.sortList...); err != nil {
			return err
		}
		if sub.filterRows != nil {
			return sub.filterRows(run)
		}

		return nil
	}

	render := func() (string, error) {
		if err := finish(); err != nil {
			return "", err
		}

		return sprintTableAs(*builder.Table, flags.outputAs), nil
	}

	if flags.watch {
		return builder.WatchBuild(sub.loop, render)
	}

	if err := builder.Build(sub.loop); err != nil {
		return err
	}

	if err := finish(); err != nil {
		return err
	}

	outputTableAs(*builder.Table, flags.outputAs)

	return nil
}

// hideOutOfRange is the --oddities filter, used by the subcommands that offer
// it. columnID is absolute: restarts uses a fixed column while status and
// resources offset from DefaultHeaderLen, which only exists after Build.
func hideOutOfRange(run runContext, columnID int) error {
	if !run.flags.showOddities {
		return nil
	}

	rows, err := run.builder.Table.ListOutOfRange(columnID)
	if err != nil {
		return err
	}
	run.builder.Table.HideRows(rows)

	return nil
}
