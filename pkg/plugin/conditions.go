package plugin

import (
	"time"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	duration "k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var conditionsShort = "List pod conditions for each pod"

var conditionsDescription = ` Prints the status of all pod conditions (PodScheduled, Initialized,
ContainersReady, Ready, etc.) for each pod. Useful to quickly identify
why a pod is not ready.

The T column in the table output denotes P for Pod`

var conditionsExample = `  # List pod conditions from all pods
  %[1]s conditions

  # List conditions from a single pod
  %[1]s conditions my-pod-4jh36

  # List conditions output in JSON format
  %[1]s conditions -o json

  # List conditions from all pods where label app equals web
  %[1]s conditions -l app=web

  # Show only pods with a non-True condition (useful for debugging)
  %[1]s conditions -m "STATUS!=True"`

func Conditions(cmd *cobra.Command, kubeFlags *genericclioptions.ConfigFlags, args []string) error {
	log := logger{location: "Conditions"}
	log.Debug("Start")

	loopinfo := conditions{}
	builder := RowBuilder{}
	builder.DontListContainers = true
	builder.PodName = args

	connect := Connector{}
	if err := connect.LoadConfig(kubeFlags); err != nil {
		return err
	}

	commonFlagList, err := processCommonFlags(cmd)
	if err != nil {
		return err
	}
	connect.Flags = commonFlagList
	builder.Connection = &connect
	builder.SetFlagsFrom(commonFlagList)

	table := Table{}
	table.ColourOutput = commonFlagList.outputAsColour
	table.CustomColours = commonFlagList.useTheseColours

	builder.Table = &table
	builder.ShowTreeView = commonFlagList.showTreeView

	renderFn := func() (string, error) {
		return sprintTableAs(*builder.Table, commonFlagList.outputAs), nil
	}

	if commonFlagList.watch {
		return builder.WatchBuild(&loopinfo, renderFn)
	}

	if err := builder.Build(&loopinfo); err != nil {
		return err
	}

	if err := table.SortByNames(commonFlagList.sortList...); err != nil {
		return err
	}

	outputTableAs(table, commonFlagList.outputAs)
	return nil
}

type conditions struct{}

func (s *conditions) Headers() []string {
	return []string{
		"CONDITION", "STATUS", "REASON", "AGE", "MESSAGE",
	}
}

func (s *conditions) BuildContainerStatus(container v1.ContainerStatus, info BuilderInformation) ([][]Cell, error) {
	return [][]Cell{}, nil
}

func (s *conditions) BuildEphemeralContainerStatus(container v1.ContainerStatus, info BuilderInformation) ([][]Cell, error) {
	return [][]Cell{}, nil
}

func (s *conditions) HideColumns(info BuilderInformation) []int {
	return []int{}
}

func (s *conditions) BuildBranch(info BuilderInformation, rows [][]Cell) ([]Cell, error) {
	out := []Cell{
		NewCellText(""),
		NewCellText(""),
		NewCellText(""),
		NewCellText(""),
		NewCellText(""),
	}
	return out, nil
}

func (s *conditions) BuildContainerSpec(container v1.Container, info BuilderInformation) ([][]Cell, error) {
	return [][]Cell{}, nil
}

func (s *conditions) BuildEphemeralContainerSpec(container v1.EphemeralContainer, info BuilderInformation) ([][]Cell, error) {
	return [][]Cell{}, nil
}

func (s *conditions) BuildPodRow(pod v1.Pod, info BuilderInformation) ([][]Cell, error) {
	out := [][]Cell{}
	for _, cond := range pod.Status.Conditions {
		out = append(out, s.conditionBuildRow(cond))
	}
	return out, nil
}

func (s *conditions) conditionBuildRow(cond v1.PodCondition) []Cell {
	statusColour := colourOk
	switch cond.Status {
	case v1.ConditionFalse:
		statusColour = colourBad
	case v1.ConditionUnknown:
		statusColour = colourWarn
	}

	age := ""
	if !cond.LastTransitionTime.IsZero() {
		age = duration.HumanDuration(time.Since(cond.LastTransitionTime.Time))
	}

	return []Cell{
		NewCellText(string(cond.Type)),
		NewCellColourText(statusColour, string(cond.Status)),
		NewCellText(cond.Reason),
		NewCellText(age),
		NewCellText(cond.Message),
	}
}
