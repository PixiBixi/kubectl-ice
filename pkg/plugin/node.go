package plugin

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var nodeShort = "Show node resource allocation and pod bin-packing"

var nodeDescription = ` Prints per-node resource allocation: how much CPU and memory is requested and
limited by all pods scheduled on each node, expressed as a percentage of the
node's allocatable capacity. Useful for assessing bin-packing efficiency.

Use --usage to also display actual resource consumption from the metrics-server.`

var nodeExample = `  # Show resource allocation for all nodes
  %[1]s node

  # Show allocation and live usage (requires metrics-server)
  %[1]s node --usage

  # Show a specific node
  %[1]s node my-node-1

  # Show nodes matching a label selector
  %[1]s node -l node-role.kubernetes.io/worker=

  # Output as JSON
  %[1]s node -o json`

// nodeAllocation holds the summed requests/limits for pods on a single node.
type nodeAllocation struct {
	cpuRequested int64
	cpuLimit     int64
	memRequested int64
	memLimit     int64
	podCount     int
}

func NodeResources(cmd *cobra.Command, kubeFlags *genericclioptions.ConfigFlags, args []string) error {
	log := logger{location: "NodeResources"}
	log.Debug("Start")

	connect := Connector{}
	if err := connect.LoadConfig(cmd.Context(), kubeFlags); err != nil {
		return err
	}

	commonFlagList, err := processCommonFlags(cmd)
	if err != nil {
		return err
	}
	connect.Flags = commonFlagList

	showUsage := cmd.Flag("usage").Value.String() == "true"
	if showUsage {
		if err := connect.LoadMetricConfig(kubeFlags); err != nil {
			return fmt.Errorf("--usage requires metrics-server: %w", err)
		}
	}

	showComputeClass := cmd.Flag("compute-class").Value.String() == "true"
	showOverallocated := cmd.Flag("overallocated").Value.String() == "true"
	filterComputeClass := cmd.Flag("class").Value.String()

	nodes, err := connect.GetNodes(args)
	if err != nil {
		return err
	}

	if filterComputeClass != "" {
		filtered := nodes[:0]
		for _, n := range nodes {
			if n.Labels["cloud.google.com/compute-class"] == filterComputeClass {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	// Always use all namespaces for allocation computation so node totals are accurate.
	allPods, err := connect.GetAllPodsAllNamespaces()
	if err != nil {
		return err
	}
	allocations := computeNodeAllocations(allPods)

	// Build a map of node metrics if requested.
	nodeMetrics := map[string]v1.ResourceList{}
	if showUsage {
		metrics, err := connect.GetMetricNodes()
		if err != nil {
			log.Tell(err)
		} else {
			for _, m := range metrics {
				nodeMetrics[m.Name] = m.Usage
			}
		}
	}

	table := Table{}
	// For node, force errors-only coloring regardless of the requested color mode:
	// column-wheel rainbow on 14 columns is visually noisy and hides the signal.
	// Semantic colors (STATUS green/red, % green/orange/red) are sufficient.
	if commonFlagList.outputAsColour != COLOUR_NONE {
		table.ColourOutput = COLOUR_ERRORS
	}
	table.CustomColours = commonFlagList.useTheseColours

	headers := []string{
		"NODENAME",
		"STATUS", "ROLES", "PODS",
		"CPU-ALLOC", "CPU-REQ", "%CPU-REQ", "CPU-LIM", "%CPU-LIM",
		"MEM-ALLOC", "MEM-REQ", "%MEM-REQ", "MEM-LIM", "%MEM-LIM",
	}
	if showUsage {
		headers = append(headers, "CPU-USED", "%CPU-USED", "MEM-USED", "%MEM-USED")
	}
	if showComputeClass {
		headers = append(headers, "COMPUTE-CLASS")
	}
	table.SetHeader(headers...)

	for _, node := range nodes {
		alloc := allocations[node.Name]

		cpuAllocatable := node.Status.Allocatable.Cpu().MilliValue()
		memAllocatable := node.Status.Allocatable.Memory().Value()
		maxPods := node.Status.Allocatable.Pods().Value()

		status := nodeReadyStatus(node)
		roles := nodeRoles(node)
		pods := fmt.Sprintf("%d/%d", alloc.podCount, maxPods)

		cpuAllocStr := fmt.Sprintf("%dm", cpuAllocatable)
		cpuReqStr := fmt.Sprintf("%dm", alloc.cpuRequested)
		cpuLimStr := fmt.Sprintf("%dm", alloc.cpuLimit)
		memAllocStr := memoryHumanReadable(memAllocatable, "Gi")
		memReqStr := memoryHumanReadable(alloc.memRequested, "Gi")
		memLimStr := memoryHumanReadable(alloc.memLimit, "Gi")

		cpuReqPct, cpuReqPctRaw := pctOf(alloc.cpuRequested, cpuAllocatable)
		cpuLimPct, cpuLimPctRaw := pctOf(alloc.cpuLimit, cpuAllocatable)
		memReqPct, memReqPctRaw := pctOf(alloc.memRequested, memAllocatable)
		memLimPct, memLimPctRaw := pctOf(alloc.memLimit, memAllocatable)

		statusColour := colourOk
		if status != "Ready" {
			statusColour = colourBad
		}

		row := []Cell{
			NewCellText(node.Name),
			NewCellColourText(statusColour, status),
			NewCellText(roles),
			NewCellText(pods),
			NewCellInt(cpuAllocStr, cpuAllocatable),
			NewCellInt(cpuReqStr, alloc.cpuRequested),
			NewCellColourFloat(setColourValue(int(cpuReqPctRaw)), cpuReqPct, cpuReqPctRaw),
			NewCellInt(cpuLimStr, alloc.cpuLimit),
			NewCellColourFloat(setColourValue(int(cpuLimPctRaw)), cpuLimPct, cpuLimPctRaw),
			NewCellInt(memAllocStr, memAllocatable),
			NewCellInt(memReqStr, alloc.memRequested),
			NewCellColourFloat(setColourValue(int(memReqPctRaw)), memReqPct, memReqPctRaw),
			NewCellInt(memLimStr, alloc.memLimit),
			NewCellColourFloat(setColourValue(int(memLimPctRaw)), memLimPct, memLimPctRaw),
		}

		if showComputeClass {
			cc := node.Labels["cloud.google.com/compute-class"]
			if cc == "" {
				cc = "-"
			}
			row = append(row, NewCellText(cc))
		}

		if showUsage {
			usage := nodeMetrics[node.Name]
			cpuUsed := int64(0)
			memUsed := int64(0)
			cpuUsedStr := "-"
			memUsedStr := "-"
			cpuUsedPct := "-"
			memUsedPct := "-"
			cpuUsedPctRaw := 0.0
			memUsedPctRaw := 0.0

			if usage != nil {
				if usage.Cpu() != nil {
					cpuUsed = usage.Cpu().MilliValue()
					cpuUsedStr = fmt.Sprintf("%dm", cpuUsed)
					cpuUsedPct, cpuUsedPctRaw = pctOf(cpuUsed, cpuAllocatable)
				}
				if usage.Memory() != nil {
					memUsed = usage.Memory().Value()
					memUsedStr = memoryHumanReadable(memUsed, "Gi")
					memUsedPct, memUsedPctRaw = pctOf(memUsed, memAllocatable)
				}
			}

			row = append(row,
				NewCellInt(cpuUsedStr, cpuUsed),
				NewCellColourFloat(setColourValue(int(cpuUsedPctRaw)), cpuUsedPct, cpuUsedPctRaw),
				NewCellInt(memUsedStr, memUsed),
				NewCellColourFloat(setColourValue(int(memUsedPctRaw)), memUsedPct, memUsedPctRaw),
			)
		}

		if showOverallocated && cpuLimPctRaw <= 100 && memLimPctRaw <= 100 {
			continue
		}

		table.AddRow(row...)
	}

	if err := table.SortByNames(commonFlagList.sortList...); err != nil {
		return err
	}

	outputTableAs(table, commonFlagList.outputAs)
	return nil
}

// computeNodeAllocations sums the effective CPU/memory requests and limits of
// every non-completed pod, grouped by the node it is scheduled on.
func computeNodeAllocations(pods []v1.Pod) map[string]nodeAllocation {
	allocs := map[string]nodeAllocation{}

	for _, pod := range pods {
		// Skip pods that are not consuming node resources.
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}

		a := allocs[pod.Spec.NodeName]
		a.podCount++

		cpu := func(q apiresource.Quantity) int64 { return q.MilliValue() }
		mem := func(q apiresource.Quantity) int64 { return q.Value() }
		a.cpuRequested += podEffective(&pod, v1.ResourceCPU, false, cpu)
		a.cpuLimit += podEffective(&pod, v1.ResourceCPU, true, cpu)
		a.memRequested += podEffective(&pod, v1.ResourceMemory, false, mem)
		a.memLimit += podEffective(&pod, v1.ResourceMemory, true, mem)

		allocs[pod.Spec.NodeName] = a
	}

	return allocs
}

// podEffective is the scheduler's per pod value (k8s.io/component-helpers
// resource.PodRequests/PodLimits, not imported to keep the binary small): app
// containers plus native sidecars, or the largest init container plus the
// sidecars started before it, whichever is higher, plus the pod overhead.
func podEffective(pod *v1.Pod, name v1.ResourceName, limits bool, value func(apiresource.Quantity) int64) int64 {
	get := func(r v1.ResourceRequirements) (int64, bool) {
		list := r.Requests
		if limits {
			list = r.Limits
		}
		q, ok := list[name]
		if !ok {
			return 0, false
		}
		return value(q), true
	}

	// pod level resources replace the container aggregate when set
	if pod.Spec.Resources != nil {
		if v, ok := get(*pod.Spec.Resources); ok {
			return v + podOverhead(pod, name, value)
		}
	}

	var total, sidecars, initMax int64
	for i := range pod.Spec.Containers {
		v, _ := get(pod.Spec.Containers[i].Resources)
		total += v
	}
	for i := range pod.Spec.InitContainers {
		c := &pod.Spec.InitContainers[i]
		v, _ := get(c.Resources)
		if c.RestartPolicy != nil && *c.RestartPolicy == v1.ContainerRestartPolicyAlways {
			sidecars += v
			v = sidecars
		} else {
			v += sidecars
		}
		initMax = max(initMax, v)
	}

	v := max(total+sidecars, initMax)
	// like PodLimits, overhead is not added to an unset limit
	if limits && v == 0 {
		return 0
	}
	return v + podOverhead(pod, name, value)
}

func podOverhead(pod *v1.Pod, name v1.ResourceName, value func(apiresource.Quantity) int64) int64 {
	if q, ok := pod.Spec.Overhead[name]; ok {
		return value(q)
	}
	return 0
}

// nodeReadyStatus returns the human-readable status of a node.
func nodeReadyStatus(node v1.Node) string {
	if node.Spec.Unschedulable {
		return "SchedulingDisabled"
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			if cond.Status == v1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

// nodeRoles extracts the node roles from its labels.
func nodeRoles(node v1.Node) string {
	var roles []string
	for label := range node.Labels {
		if role, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok {
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		if v, ok := node.Labels["kubernetes.io/role"]; ok && v != "" {
			return v
		}
		return "<none>"
	}
	return strings.Join(roles, ",")
}

// pctOf returns (formatted string, float64) for value/total*100.
// Returns "-" if total is 0.
func pctOf(value, total int64) (string, float64) {
	if total == 0 {
		return "-", 0
	}
	pct := float64(value) / float64(total) * 100
	return fmt.Sprintf("%.1f", pct), pct
}
