package plugin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	a1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

const TypeIDContainer string = "C"
const TypeNameContainer string = "Container"
const TypeIDInitContainer string = "I"
const TypeNameInitContainer string = "InitContainer"
const TypeIDEphemeralContainer string = "E"
const TypeNameEphemeralContainer string = "EphemeralContainer"
const TypeIDPod string = "P"
const TypeNamePod string = "Pod"
const TypeIDNode string = "N"
const TypeNameNode string = "Node"
const TypeIDDeployment string = "D"
const TypeNameDeployment string = "Deployment"
const TypeIDReplicaSet string = "R"
const TypeNameReplicaSet string = "ReplicaSet"
const TypeIDDaemonSet string = "A"
const TypeNameDaemonSet string = "DaemonSet"
const TypeIDStatefulSet string = "S"
const TypeNameStatefulSet string = "StatefulSet"
const TypeIDJob string = "J"
const TypeNameJob string = "Job"
const TypeIDCronJob string = "O"
const TypeNameCronJob string = "CronJob"

// const TypeID string= ""
// const TypeName string = ""

type Connector struct {
	clientSet         kubernetes.Clientset
	metricSet         metricsclientset.Clientset
	Flags             commonFlags
	configFlags       *genericclioptions.ConfigFlags
	metricFlags       *genericclioptions.ConfigFlags
	configMapArray    map[string]map[string]string
	setNameSpace      string
	resolvedNamespace string                       // cached result of kubeconfig context lookup
	namespaceResolved bool                         // true once resolvedNamespace has been populated
	podList           []v1.Pod                     // List of Pods
	replicaList       map[string][]a1.ReplicaSet   // list of ReplicaSets
	daemonList        map[string][]a1.DaemonSet    // list of DaemonSets
	statefulList      map[string][]a1.StatefulSet  // list of StatefulSet
	deploymentList    map[string][]a1.Deployment   // list of Deployments
	jobList           map[string][]batchv1.Job     // list of k8s Jobs
	cronJobList       map[string][]batchv1.CronJob // list of k8s CronJobs
}

type ParentData struct {
	name          string
	kind          string
	kindIndicator string
	namespace     string
	deployment    a1.Deployment
	replica       a1.ReplicaSet
	stateful      a1.StatefulSet
	daemon        a1.DaemonSet
	job           batchv1.Job
	cronjob       batchv1.CronJob
	pod           v1.Pod
}

type LeafNode struct {
	childIndex    map[string]*LeafNode // O(1) lookup by name
	child         []*LeafNode
	name          string
	kind          string
	kindIndicator string
	namespace     string
	indent        int
	data          ParentData
}

func (n *LeafNode) getChild(name string) *LeafNode {
	if v, ok := n.childIndex[name]; ok {
		return v
	}

	child := &LeafNode{
		name:       name,
		child:      []*LeafNode{},
		childIndex: make(map[string]*LeafNode),
	}

	n.child = append(n.child, child)
	if n.childIndex == nil {
		n.childIndex = make(map[string]*LeafNode)
	}
	n.childIndex[name] = child

	return child
}

// load config for the k8s endpoint
func (c *Connector) LoadConfig(configFlags *genericclioptions.ConfigFlags) error {
	c.clientSet = kubernetes.Clientset{}
	c.configFlags = configFlags
	config, err := configFlags.ToRESTConfig()

	if err != nil {
		return fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}
	c.clientSet = *clientset
	return nil
}

// load config for the metrics endpoint
func (c *Connector) LoadMetricConfig(configFlags *genericclioptions.ConfigFlags) error {
	c.metricSet = metricsclientset.Clientset{}
	c.metricFlags = configFlags
	config, err := configFlags.ToRESTConfig()

	if err != nil {
		return fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	metricset, err := metricsclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset for metrics: %w", err)
	}

	c.metricSet = *metricset
	return nil
}

// returns a list of pods or a list with one pod when given a pod name
func (c *Connector) GetPods(podNameList []string) ([]v1.Pod, error) {
	if len(c.podList) == 0 {
		err := c.LoadPods(podNameList)
		return c.podList, err
	}

	if len(podNameList) > 0 {
		err := c.LoadPods(podNameList)
		return c.podList, err
	}

	return c.podList, nil

}

// podMetaMap keys the extracted per-pod metadata by namespace+name so that
// identically-named pods across namespaces (e.g. StatefulSet thanos-receive-0
// listed with -A) do not overwrite each other.
func podMetaMap(podList []v1.Pod, extract func(v1.Pod) map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(podList))
	for _, pod := range podList {
		out[pod.Namespace+"/"+pod.Name] = extract(pod)
	}
	return out
}

func (c *Connector) GetPodAnnotations(podList []v1.Pod) (map[string]map[string]string, error) {
	return podMetaMap(podList, func(p v1.Pod) map[string]string { return p.Annotations }), nil
}

func (c *Connector) GetPodLabels(podList []v1.Pod) (map[string]map[string]string, error) {
	return podMetaMap(podList, func(p v1.Pod) map[string]string { return p.Labels }), nil
}

func (c *Connector) GetNodeLabels(podList []v1.Pod) (map[string]map[string]string, error) {
	//
	var nameList []string

	labelMap := make(map[string]map[string]string)
	nodeNames := make(map[string]int)

	for _, pod := range c.podList {
		nodeName := pod.Spec.NodeName
		if _, ok := nodeNames[nodeName]; !ok {
			nodeNames[nodeName] = 1
			nameList = append(nameList, nodeName)
		}
	}

	nodeList, err := c.GetNodes(nameList)
	if err != nil {
		return map[string]map[string]string{}, err
	}

	for _, node := range nodeList {
		name := node.Name
		labels := node.Labels
		labelMap[name] = labels
	}

	return labelMap, nil
}

// returns a list of nodes
func (c *Connector) GetNodes(nodeNameList []string) ([]v1.Node, error) {
	selector := metav1.ListOptions{}

	switch len(nodeNameList) {
	case 0:
		// list all nodes, optionally filtered by label selector
		if len(c.Flags.labels) > 0 {
			selector.LabelSelector = c.Flags.labels
		}
		nodes, err := c.clientSet.CoreV1().Nodes().List(context.TODO(), selector)
		if err != nil {
			return []v1.Node{}, fmt.Errorf("failed to retrieve node list from server: %w", err)
		}
		if len(nodes.Items) == 0 {
			return []v1.Node{}, errors.New("no nodes found in default namespace")
		}
		return nodes.Items, nil

	case 1:
		// single node: direct Get is cheapest
		node, err := c.clientSet.CoreV1().Nodes().Get(context.TODO(), nodeNameList[0], metav1.GetOptions{})
		if err != nil {
			return []v1.Node{}, fmt.Errorf("failed to retrieve node from server: %w", err)
		}
		return []v1.Node{*node}, nil

	default:
		// multiple names: one List + client-side filter beats N sequential GETs
		needed := make(map[string]struct{}, len(nodeNameList))
		for _, n := range nodeNameList {
			needed[n] = struct{}{}
		}
		all, err := c.clientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return []v1.Node{}, fmt.Errorf("failed to retrieve node list from server: %w", err)
		}
		nodeList := make([]v1.Node, 0, len(nodeNameList))
		for _, n := range all.Items {
			if _, ok := needed[n.Name]; ok {
				nodeList = append(nodeList, n)
			}
		}
		return nodeList, nil
	}
}

// SelectMatchingPodSpec select pods to inclue or exclude based on the field in v1.Pods.Spec an operator (!=, ==, =) and a string value to match with
func (c *Connector) SelectMatchinghPodSpec(pods []v1.Pod) ([]v1.Pod, error) {
	var newPodList []v1.Pod

	// grab and compare the field name to the user suppilied string as the user may have typed all in caps
	includeList := make(map[string]matchValue)

	fields := reflect.VisibleFields(reflect.TypeOf(v1.Pod{}.Spec))
	for _, field := range fields {
		isValid := false

		name := strings.ToUpper(field.Name)
		// restrict to basic types (string, int, bool)
		switch field.Type.String() {
		case "string", "*string":
			fallthrough
		case "int", "*int":
			fallthrough
		case "int32", "*int32":
			fallthrough
		case "int64", "*int64":
			fallthrough
		case "bool", "*bool":
			isValid = true
		}

		if !isValid {
			continue
		}

		if value, ok := c.Flags.matchSpecList[name]; ok {
			includeList[field.Name] = value
		}
	}

	// now we can loop through doing a name lookup with should be faster than searching each name to find a match
	for _, i := range pods {
		fields := reflect.ValueOf(i.Spec)
		for k, v := range includeList {
			field := fields.FieldByName(k)
			fieldString := convertToString(field, field.Interface())
			switch v.operator {
			case "=":
				fallthrough
			case "==":
				if fieldString == v.value {
					newPodList = append(newPodList, i)
				}
			case "!=":
				if fieldString != v.value {
					newPodList = append(newPodList, i)
				}
			default:
				return []v1.Pod{}, errors.New("invalid operator found")
			}
		}

	}

	return newPodList, nil
}

// GetMetricPods get an array of pod metrics
func (c *Connector) GetMetricPods(podNameList []string) ([]v1beta1.PodMetrics, error) {
	podList := []v1beta1.PodMetrics{}
	selector := metav1.ListOptions{}

	namespace := c.GetNamespace(c.Flags.allNamespaces)

	if len(podNameList) > 0 {
		for _, podname := range podNameList {
			if len(c.Flags.labels) > 0 {
				return []v1beta1.PodMetrics{}, fmt.Errorf("error: you cannot specify a pod name and a selector together")
			}

			// single pod
			pod, err := c.metricSet.MetricsV1beta1().PodMetricses(namespace).Get(context.TODO(), podname, metav1.GetOptions{})
			if err == nil {
				podList = append(podList, []v1beta1.PodMetrics{*pod}...)
			} else {
				return []v1beta1.PodMetrics{}, fmt.Errorf("failed to retrieve pod from metrics: %w", err)
			}
		}

		return podList, nil
	} else {
		if len(c.Flags.labels) > 0 {
			selector.LabelSelector = c.Flags.labels
		}

		podList, err := c.metricSet.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), selector)
		if err == nil {
			if len(podList.Items) == 0 {
				return []v1beta1.PodMetrics{}, errors.New("no metric info found for pods in namespace")
			} else {
				return podList.Items, nil
			}
		} else {
			return []v1beta1.PodMetrics{}, fmt.Errorf("failed to retrieve pod list from metrics: %w", err)
		}
	}
}

func (c *Connector) GetConfigMaps(configMapName string) (v1.ConfigMap, error) {

	namespace := c.GetNamespace(c.Flags.allNamespaces)

	if len(configMapName) == 0 {
		return v1.ConfigMap{}, nil
	}

	cm, err := c.clientSet.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
	if err == nil {
		return *cm, nil
	}

	return v1.ConfigMap{}, nil
}

func (c *Connector) GetConfigMapValue(configMap string, key string) string {
	var val map[string]map[string]string

	if len(configMap) <= 0 {
		return ""
	}

	if _, ok := c.configMapArray[configMap]; !ok {
		cm, err := c.GetConfigMaps(configMap)
		if err != nil {
			c.configMapArray[configMap] = make(map[string]string)
			return ""
		}

		if len(c.configMapArray) > 0 {
			val = c.configMapArray
		} else {
			val = make(map[string]map[string]string)
		}
		val[configMap] = cm.Data
		c.configMapArray = val

	}

	return c.configMapArray[configMap][key]
}

// GetNamespace retrieves the namespace that is currently set as default.
// The kubeconfig read is performed at most once per Connector instance and cached.
func (c *Connector) GetNamespace(allNamespaces bool) string {
	if allNamespaces {
		// get/list pods will search all namespaces in the current context
		return ""
	}

	if len(c.setNameSpace) >= 1 {
		return c.setNameSpace
	}

	// was a namespace specified on the cmd line
	if len(*c.configFlags.Namespace) > 0 {
		return *c.configFlags.Namespace
	}

	// return cached result if we already resolved it
	if c.namespaceResolved {
		return c.resolvedNamespace
	}

	// expensive: read kubeconfig from disk — done at most once per invocation
	namespace := ""
	ctx := ""
	clientCfg, _ := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if len(*c.configFlags.Context) > 0 {
		ctx = *c.configFlags.Context
	} else {
		ctx = clientCfg.CurrentContext
	}

	if clientCfg.Contexts[ctx] != nil {
		namespace = clientCfg.Contexts[ctx].Namespace
	}

	if len(namespace) == 0 {
		namespace = "default"
	}

	c.resolvedNamespace = namespace
	c.namespaceResolved = true
	return namespace
}

// SetNamespace sets the namespace to use when searching for pods
func (c *Connector) SetNamespace(namespace string) {
	if len(namespace) >= 1 {
		c.setNameSpace = namespace
	}
}

// convertToString expects a reflect value and the raw interface value and returns the value
// as a string, it also handles pointers correctly
func convertToString(field reflect.Value, value interface{}) string {

	switch value.(type) {
	case *bool:
		if !field.IsNil() {
			return fmt.Sprint(reflect.Indirect(field).Bool())
		}

	case *string:
		if !field.IsNil() {
			return fmt.Sprint(reflect.Indirect(field).String())
		}

	case *int, *int32, *int64:
		if !field.IsNil() {
			return fmt.Sprint(reflect.Indirect(field).Int())
		}
	}

	return fmt.Sprint(value)
}

func (c *Connector) LoadPods(podNameList []string) error {
	podList := []v1.Pod{}
	selector := metav1.ListOptions{}

	namespace := c.GetNamespace(c.Flags.allNamespaces)

	if len(podNameList) > 0 {
		if len(c.Flags.labels) > 0 {
			c.podList = []v1.Pod{}
			return fmt.Errorf("error: you cannot specify a pod name and a selector together")
		}

		// single pod
		for _, podname := range podNameList {
			pod, err := c.clientSet.CoreV1().Pods(namespace).Get(context.TODO(), podname, metav1.GetOptions{})
			if err == nil {
				podList = append(podList, []v1.Pod{*pod}...)
			} else {
				c.podList = []v1.Pod{}
				return fmt.Errorf("failed to retrieve pod from server: %w", err)
			}
		}

		c.podList = podList
		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	pods, err := c.clientSet.CoreV1().Pods(namespace).List(context.TODO(), selector)
	if err == nil {
		if len(pods.Items) == 0 {
			c.podList = []v1.Pod{}
			return errors.New("no pods found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				c.podList, err = c.SelectMatchinghPodSpec(pods.Items)
				return err
			} else {
				c.podList = pods.Items
				return nil
			}
		}
	} else {
		c.podList = []v1.Pod{}
		return fmt.Errorf("failed to retrieve pod list from server: %w", err)
	}
}

// GetOwnersList calls GetOwnerReference for each pod and returns a unique list of owner types as the key with an array of pods as the value
func (c *Connector) GetOwnersList() (map[string][]v1.Pod, map[string]string) {
	parentList := map[string][]v1.Pod{}
	typeList := map[string]string{}

	for _, pod := range c.podList {
		ownerRef := pod.GetOwnerReferences()
		if len(ownerRef) == 0 {
			continue
		}

		for _, a := range ownerRef {
			parentList[a.Name] = append(parentList[a.Name], pod)
			typeList[a.Name] = a.Kind
		}
	}

	return parentList, typeList
}

func (c *Connector) GetReplicaSet(replicaName string, namespace string) *a1.ReplicaSet {
	var rs []a1.ReplicaSet

	if _, ok := c.replicaList[namespace]; ok {
		rs = c.replicaList[namespace]
	} else {
		c.LoadReplicaSet([]string{}, namespace)
		if _, ok := c.replicaList[namespace]; ok {
			rs = c.replicaList[namespace]
		}
	}

	for _, r := range rs {
		if r.Name == replicaName {
			return &r
		}
	}
	return nil
}

func (c *Connector) LoadReplicaSet(replicaNameList []string, namespace string) error {

	log := logger{location: "k8sconnector:LoadReplicaSet"}
	log.Debug("Start")

	selector := metav1.ListOptions{}
	if c.replicaList == nil {
		c.replicaList = make(map[string][]a1.ReplicaSet)
	}

	if len(replicaNameList) > 0 {
		// single pod
		for _, replicaName := range replicaNameList {
			rs, err := c.clientSet.AppsV1().ReplicaSets(namespace).Get(context.TODO(), replicaName, metav1.GetOptions{})
			if err == nil {
				list := append(c.replicaList[namespace], *rs)
				c.replicaList[namespace] = list
			} else {
				return fmt.Errorf("failed to retrieve ReplicaSet from server: %w", err)
			}
		}

		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	rs, err := c.clientSet.AppsV1().ReplicaSets(namespace).List(context.TODO(), selector)
	if err == nil {
		if len(rs.Items) == 0 {
			return errors.New("no ReplicaSet found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				return err
			} else {
				c.replicaList[namespace] = append(c.replicaList[namespace], rs.Items...)
				return nil
			}
		}
	} else {
		return fmt.Errorf("failed to retrieve ReplicaSet list from server: %w", err)
	}
}

func (c *Connector) GetDeployment(deploymentName string, namespace string) *a1.Deployment {
	var de []a1.Deployment

	if _, ok := c.deploymentList[namespace]; ok {
		de = c.deploymentList[namespace]
	} else {
		c.LoadDeployment([]string{}, namespace)
		if _, ok := c.deploymentList[namespace]; ok {
			de = c.deploymentList[namespace]
		}
	}

	for _, d := range de {
		if d.Name == deploymentName {
			return &d
		}
	}
	return nil
}

func (c *Connector) LoadDeployment(deploymentNameList []string, namespace string) error {
	log := logger{location: "k8sconnector:LoadDeployment"}
	log.Debug("Start")

	selector := metav1.ListOptions{}
	if c.deploymentList == nil {
		c.deploymentList = make(map[string][]a1.Deployment)
	}

	if len(deploymentNameList) > 0 {
		// single pod
		for _, name := range deploymentNameList {
			d, err := c.clientSet.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err == nil {
				list := append(c.deploymentList[namespace], *d)
				c.deploymentList[namespace] = list
			} else {
				return fmt.Errorf("failed to retrieve Deployment from server: %w", err)
			}
		}

		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	d, err := c.clientSet.AppsV1().Deployments(namespace).List(context.TODO(), selector)

	if err == nil {
		if len(d.Items) == 0 {
			return errors.New("no Deployment found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				return err
			} else {
				c.deploymentList[namespace] = append(c.deploymentList[namespace], d.Items...)
				return nil
			}
		}
	} else {
		return fmt.Errorf("failed to retrieve Deployment list from server: %w", err)
	}
}

func (c *Connector) GetDaemonSet(daemonName string, namespace string) *a1.DaemonSet {
	var rs []a1.DaemonSet

	if _, ok := c.daemonList[namespace]; ok {
		rs = c.daemonList[namespace]
	} else {
		c.LoadDaemonSet([]string{}, namespace)
		if _, ok := c.daemonList[namespace]; ok {
			rs = c.daemonList[namespace]
		}
	}

	for _, r := range rs {
		if r.Name == daemonName {
			return &r
		}
	}
	return nil
}

func (c *Connector) LoadDaemonSet(daemonNameList []string, namespace string) error {
	log := logger{location: "k8sconnector:LoadDaemonSet"}
	log.Debug("Start")

	selector := metav1.ListOptions{}
	if c.daemonList == nil {
		c.daemonList = make(map[string][]a1.DaemonSet)
	}

	if len(daemonNameList) > 0 {
		// single pod
		for _, name := range daemonNameList {
			d, err := c.clientSet.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err == nil {
				list := append(c.daemonList[namespace], *d)
				c.daemonList[namespace] = list
			} else {
				return fmt.Errorf("failed to retrieve DaemonSet from server: %w", err)
			}
		}

		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	d, err := c.clientSet.AppsV1().DaemonSets(namespace).List(context.TODO(), selector)

	if err == nil {
		if len(d.Items) == 0 {
			return errors.New("no DaemonSet found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				return err
			} else {
				c.daemonList[namespace] = append(c.daemonList[namespace], d.Items...)
				return nil
			}
		}
	} else {
		return fmt.Errorf("failed to retrieve DaemonSet list from server: %w", err)
	}
}

func (c *Connector) GetStatefulSet(statefulsetName string, namespace string) *a1.StatefulSet {
	var ss []a1.StatefulSet

	if _, ok := c.statefulList[namespace]; ok {
		ss = c.statefulList[namespace]
	} else {
		c.LoadStatefulSet([]string{}, namespace)
		if _, ok := c.statefulList[namespace]; ok {
			ss = c.statefulList[namespace]
		}
	}

	for _, s := range ss {
		if s.Name == statefulsetName {
			return &s
		}
	}
	return nil
}

func (c *Connector) LoadStatefulSet(statefulNameList []string, namespace string) error {
	log := logger{location: "k8sconnector:LoadStatefulSet"}
	log.Debug("Start")

	selector := metav1.ListOptions{}
	if c.statefulList == nil {
		c.statefulList = make(map[string][]a1.StatefulSet)
	}

	if len(statefulNameList) > 0 {
		// single pod
		for _, replicaName := range statefulNameList {
			s, err := c.clientSet.AppsV1().StatefulSets(namespace).Get(context.TODO(), replicaName, metav1.GetOptions{})
			if err == nil {
				list := append(c.statefulList[namespace], *s)
				c.statefulList[namespace] = list
			} else {
				return fmt.Errorf("failed to retrieve StatefulSet from server: %w", err)
			}
		}

		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	s, err := c.clientSet.AppsV1().StatefulSets(namespace).List(context.TODO(), selector)

	if err == nil {
		if len(s.Items) == 0 {
			return errors.New("no StatefulSet found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				return err
			} else {
				c.statefulList[namespace] = append(c.statefulList[namespace], s.Items...)
				return nil
			}
		}
	} else {
		return fmt.Errorf("failed to retrieve StatefulSet list from server: %w", err)
	}
}

func (c *Connector) GetJob(jobName string, namespace string) *batchv1.Job {
	var cj []batchv1.Job

	if _, ok := c.jobList[namespace]; ok {
		cj = c.jobList[namespace]
	} else {
		c.LoadJob([]string{}, namespace)
		if _, ok := c.jobList[namespace]; ok {
			cj = c.jobList[namespace]
		}
	}

	for _, j := range cj {
		if j.Name == jobName {
			return &j
		}
	}
	return nil
}

func (c *Connector) LoadJob(jobNameList []string, namespace string) error {
	log := logger{location: "k8sconnector:LoadJob"}
	log.Debug("Start")

	selector := metav1.ListOptions{}
	if c.jobList == nil {
		c.jobList = make(map[string][]batchv1.Job)
	}

	if len(jobNameList) > 0 {
		// single pod
		for _, name := range jobNameList {
			j, err := c.clientSet.BatchV1().Jobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err == nil {
				list := append(c.jobList[namespace], *j)
				c.jobList[namespace] = list
			} else {
				return fmt.Errorf("failed to retrieve Job from server: %w", err)
			}
		}

		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	j, err := c.clientSet.BatchV1().Jobs(namespace).List(context.TODO(), selector)

	if err == nil {
		if len(j.Items) == 0 {
			return errors.New("no Jobs found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				return err
			} else {
				c.jobList[namespace] = append(c.jobList[namespace], j.Items...)
				return nil
			}
		}
	} else {
		return fmt.Errorf("failed to retrieve Job list from server: %w", err)
	}
}

func (c *Connector) GetCronJob(jobName string, namespace string) *batchv1.CronJob {
	var cj []batchv1.CronJob

	if _, ok := c.cronJobList[namespace]; ok {
		cj = c.cronJobList[namespace]
	} else {
		c.LoadCronJob([]string{}, namespace)
		if _, ok := c.cronJobList[namespace]; ok {
			cj = c.cronJobList[namespace]
		}
	}

	for _, j := range cj {
		if j.Name == jobName {
			return &j
		}
	}
	return nil
}

func (c *Connector) LoadCronJob(jobNameList []string, namespace string) error {
	log := logger{location: "k8sconnector:LoadCronJob"}
	log.Debug("Start")

	selector := metav1.ListOptions{}
	if c.cronJobList == nil {
		c.cronJobList = make(map[string][]batchv1.CronJob)
	}

	if len(jobNameList) > 0 {
		// single pod
		for _, name := range jobNameList {
			j, err := c.clientSet.BatchV1().CronJobs(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			if err == nil {
				list := append(c.cronJobList[namespace], *j)
				c.cronJobList[namespace] = list
			} else {
				return fmt.Errorf("failed to retrieve CronJob from server: %w", err)
			}
		}

		return nil
	}

	// multi pods
	if len(c.Flags.labels) > 0 {
		selector.LabelSelector = c.Flags.labels
	}

	j, err := c.clientSet.BatchV1().CronJobs(namespace).List(context.TODO(), selector)

	if err == nil {
		if len(j.Items) == 0 {
			return errors.New("no CronJobs found in default namespace")
		} else {
			if len(c.Flags.matchSpecList) > 0 {
				return err
			} else {
				c.cronJobList[namespace] = append(c.cronJobList[namespace], j.Items...)
				return nil
			}
		}
	} else {
		return fmt.Errorf("failed to retrieve CronJob list from server: %w", err)
	}
}

// ClearPodCache clears the cached pod list, forcing a re-fetch on the next GetPods call.
func (c *Connector) ClearPodCache() {
	c.podList = nil
}

// GetAllPodsAllNamespaces returns all pods across all namespaces regardless of
// the current namespace filter. Used to compute accurate node allocations.
func (c *Connector) GetAllPodsAllNamespaces() ([]v1.Pod, error) {
	pods, err := c.clientSet.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pods for node allocation: %w", err)
	}
	return pods.Items, nil
}

// GetMetricNodes returns node metrics from the metrics-server.
func (c *Connector) GetMetricNodes() ([]v1beta1.NodeMetrics, error) {
	list, err := c.metricSet.MetricsV1beta1().NodeMetricses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve node metrics: %w", err)
	}
	return list.Items, nil
}

// WatchPods starts a Kubernetes watch stream for pods in the configured namespace.
func (c *Connector) WatchPods(ctx context.Context) (watch.Interface, error) {
	namespace := c.GetNamespace(c.Flags.allNamespaces)
	opts := metav1.ListOptions{}
	if len(c.Flags.labels) > 0 {
		opts.LabelSelector = c.Flags.labels
	}
	return c.clientSet.CoreV1().Pods(namespace).Watch(ctx, opts)
}

func (c *Connector) BuildOwnersList() []*LeafNode {

	rootnode := LeafNode{child: []*LeafNode{}, childIndex: make(map[string]*LeafNode)}

	for _, pod := range c.podList {
		nodename := pod.Spec.NodeName
		// build leaf-to-root: start with the pod, appendParents appends ancestors
		parentList := []ParentData{{
			name:          pod.Name,
			namespace:     pod.Namespace,
			kind:          TypeNamePod,
			kindIndicator: TypeIDPod,
			pod:           pod,
		}}
		oref := pod.GetOwnerReferences()

		// appendParents appends ancestors in leaf-to-root order; reverse gives root-to-leaf
		parentList = c.appendParents(parentList, oref, nodename, pod.Namespace)
		slices.Reverse(parentList)

		// finally we can loop through the above list adding children to the tree where they are needed and using child nodes if they already exist
		current := &rootnode
		for i, v := range parentList {
			child := current.getChild(v.name)
			child.kind = v.kind
			child.kindIndicator = v.kindIndicator
			child.namespace = v.namespace
			child.indent = i
			child.data = v
			current = child
		}

	}

	return rootnode.child

}

// appendParents appends ancestor ParentData entries in leaf-to-root order (no prepend).
// The caller must reverse the result to get root-to-leaf ordering for tree display.
func (c *Connector) appendParents(current []ParentData, oref []metav1.OwnerReference, nodename string, namespace string) []ParentData {
	log := logger{location: "k8sconnector:appendParents"}
	log.Debug("Start")

	// no owner: attach directly to the node from the pod spec
	if len(oref) == 0 {
		return append(current, ParentData{
			name:          nodename,
			kind:          TypeNameNode,
			kindIndicator: TypeIDNode,
		})
	}

	for _, v := range oref {
		log.Debug("v.Kind", v.Kind)
		if v.Kind == TypeNameNode {
			return append(current, ParentData{
				name:          v.Name,
				kind:          v.Kind,
				kindIndicator: TypeIDNode,
			})
		}
		if v.Kind == TypeNameDeployment {
			deployment := c.GetDeployment(v.Name, namespace)
			if deployment != nil {
				return c.appendParents(append(current, ParentData{
					name:          v.Name,
					kind:          v.Kind,
					kindIndicator: TypeIDDeployment,
					namespace:     deployment.Namespace,
					deployment:    *deployment,
				}), deployment.GetOwnerReferences(), nodename, namespace)
			}
		}
		if v.Kind == TypeNameReplicaSet {
			replica := c.GetReplicaSet(v.Name, namespace)
			if replica != nil {
				return c.appendParents(append(current, ParentData{
					name:          v.Name,
					kind:          v.Kind,
					kindIndicator: TypeIDReplicaSet,
					namespace:     replica.Namespace,
					replica:       *replica,
				}), replica.GetOwnerReferences(), nodename, namespace)
			}
		}
		if v.Kind == TypeNameDaemonSet {
			daemon := c.GetDaemonSet(v.Name, namespace)
			if daemon != nil {
				return c.appendParents(append(current, ParentData{
					name:          v.Name,
					kind:          v.Kind,
					kindIndicator: TypeIDDaemonSet,
					namespace:     daemon.Namespace,
					daemon:        *daemon,
				}), daemon.GetOwnerReferences(), nodename, namespace)
			}
		}
		if v.Kind == TypeNameStatefulSet {
			stateful := c.GetStatefulSet(v.Name, namespace)
			if stateful != nil {
				return c.appendParents(append(current, ParentData{
					name:          v.Name,
					kind:          v.Kind,
					kindIndicator: TypeIDStatefulSet,
					namespace:     stateful.Namespace,
					stateful:      *stateful,
				}), stateful.GetOwnerReferences(), nodename, namespace)
			}
		}
		if v.Kind == TypeNameJob {
			job := c.GetJob(v.Name, namespace)
			if job != nil {
				return c.appendParents(append(current, ParentData{
					name:          v.Name,
					kind:          v.Kind,
					kindIndicator: TypeIDJob,
					namespace:     job.Namespace,
					job:           *job,
				}), job.GetOwnerReferences(), nodename, namespace)
			}
		}
		if v.Kind == TypeNameCronJob {
			job := c.GetCronJob(v.Name, namespace)
			if job != nil {
				return c.appendParents(append(current, ParentData{
					name:          v.Name,
					kind:          v.Kind,
					kindIndicator: TypeIDCronJob,
					namespace:     job.Namespace,
					cronjob:       *job,
				}), job.GetOwnerReferences(), nodename, namespace)
			}
		}
	}

	return current
}
