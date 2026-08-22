package plugin

import (
	"strconv"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var portsShort = "Shows ports exposed by the containers in a pod"

var portsDescription = ` Print a details of service ports exposed by containers in a pod. Details include the container 
name, port number and protocol type. Port name and host port are only show if available. If no
name is specified the container port details of all pods in the current namespace are shown.

The T column in the table output denotes S for Standard and I for init containers`

var portsExample = `  # List containers port info from pods
  %[1]s ports

  # List container port info from pods output in JSON format
  %[1]s ports -o json

  # List container port info from a single pod
  %[1]s ports my-pod-4jh36

  # List port info for all containers named web-container searching all 
  # pods in the current namespace
  %[1]s ports -c web-container

  # List port info for all containers called web-container searching all pods in current
  # namespace sorted by container name in descending order (notice the ! charator)
  %[1]s ports -c web-container --sort '!CONTAINER'

  # List port info for all containers called web-container searching all pods in current
  # namespace sorted by pod name in ascending order
  %[1]s ports -c web-container --sort PODNAME

  # List container port info from all pods where label app matches web
  %[1]s ports -l app=web

  # List container port info from all pods where the pod label app is either web or mail
  %[1]s ports -l "app in (web,mail)"`

// Ports show the port infor for each container
//
//		runip - true = show ip details only
//	          - false = show all port info except ip info
func Ports(cmd *cobra.Command, kubeFlags *genericclioptions.ConfigFlags, args []string, runip bool) error {
	loopinfo := ports{}

	return runSubCommand(cmd, kubeFlags, args, subCommand{
		loop:               &loopinfo,
		loopSpec:           true,
		showInitContainers: true,
		// the ip subcommand shares this Looper but lists one row per pod
		dontListContainers: runip,
		configure: func(run runContext) error {
			loopinfo.DontListContainers = runip
			if run.cmd.Flag("show-ip") != nil {
				loopinfo.ShowIPAddress = run.cmd.Flag("show-ip").Value.String() == "true"
			}

			return nil
		},
	})
}

type ports struct {
	DontListContainers bool
	ShowIPAddress      bool
}

func (s *ports) Headers() []string {
	return []string{
		"PORTNAME", "PORT", "PROTO", "HOSTPORT", "IP",
	}
}

func (s *ports) BuildContainerStatus(container v1.ContainerStatus, info BuilderInformation) ([][]Cell, error) {
	return [][]Cell{}, nil
}

func (s *ports) BuildEphemeralContainerStatus(container v1.ContainerStatus, info BuilderInformation) ([][]Cell, error) {
	return [][]Cell{}, nil
}

func (s *ports) HideColumns(info BuilderInformation) []int {
	if s.ShowIPAddress {
		return []int{}
	}
	if s.DontListContainers {
		return []int{0, 1, 2, 3}
	} else {
		return []int{4}
	}
}

func (s *ports) BuildBranch(info BuilderInformation, rows [][]Cell) ([]Cell, error) {
	out := []Cell{
		NewCellText(""),
		NewCellText(""),
		NewCellText(""),
		NewCellText(""),
		NewCellText(""),
	}
	return out, nil
}

func (s *ports) BuildContainerSpec(container v1.Container, info BuilderInformation) ([][]Cell, error) {
	out := [][]Cell{}
	for _, port := range container.Ports {
		out = append(out, s.portsBuildRow(info, port))
	}
	return out, nil
}

func (s *ports) BuildEphemeralContainerSpec(container v1.EphemeralContainer, info BuilderInformation) ([][]Cell, error) {
	out := [][]Cell{}
	for _, port := range container.Ports {
		out = append(out, s.portsBuildRow(info, port))
	}
	return out, nil
}

func (s *ports) portsBuildRow(info BuilderInformation, port v1.ContainerPort) []Cell {
	var cellList []Cell

	var hostPort Cell

	if port.HostPort > 0 {
		hostPort = NewCellInt(strconv.Itoa(int(port.HostPort)), int64(port.HostPort))
	} else {
		hostPort = NewCellText("")
	}

	// if info.TreeView {
	// 	cellList = info.BuildTreeCell(cellList)
	// }

	cellList = append(cellList,
		NewCellText(port.Name),
		NewCellInt(strconv.Itoa(int(port.ContainerPort)), int64(port.ContainerPort)),
		NewCellText(string(port.Protocol)),
		hostPort,
		NewCellText(info.Data.pod.Status.PodIP),
	)
	return cellList
}

func (s *ports) BuildPodRow(pod v1.Pod, info BuilderInformation) ([][]Cell, error) {
	out := make([][]Cell, 1)
	out[0] = append([]Cell{},
		NewCellEmpty(),
		NewCellEmpty(),
		NewCellEmpty(),
		NewCellEmpty(),
		NewCellText(info.Data.pod.Status.PodIP),
	)
	return out, nil
}
