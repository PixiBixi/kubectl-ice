# kubectl-ice

A kubectl plugin that lists detailed information about the containers running
inside a pod, useful for trouble-shooting multi container issues. You can view
volume, image, port, probe, security and executable configuration, along with
current cpu and memory metrics, all at the container level (metrics require
metrics-server).

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/PixiBixi/kubectl-ice)
![GitHub](https://img.shields.io/github/license/PixiBixi/kubectl-ice)
[![CI](https://github.com/PixiBixi/kubectl-ice/actions/workflows/ci.yml/badge.svg)](https://github.com/PixiBixi/kubectl-ice/actions/workflows/ci.yml)

> **PixiBixi maintained fork** of
> [NimbleArchitect/kubectl-ice](https://github.com/NimbleArchitect/kubectl-ice).
> Adds watch mode, the node, conditions and completion commands, and performance
> work. Distributed through its own krew custom index (this repo), not the
> official krew-index. Licensing and provenance are in [LICENSE](LICENSE) and
> [NOTICE](NOTICE).

## Features

* Runs on Linux and macOS, amd64 and arm64
* Only uses read permissions, no writes are called
* Lists every container in a pod, init and ephemeral ones included
* Watch mode (`-w`) re-renders the table live on pod events
* Per-node resource allocation and bin-packing with the `node` command
* Tree view adds each container in a pod, then each pod in a replica or stateful
  set etc, all the way up to the node level
* Selectors work just like they do with the standard kubectl command
* Sortable output columns
* Include or exclude rows from output using the match flag, useful to exclude
  containers with low memory or cpu usage
* Show only the rows that dont fall within the computed range using the oddities
  flag (cpu, memory, restarts and status)
* Pods can be filtered using their priority and priorityClassName
* Most sub commands utilize aliases meaning less typing (eg command and cmd are
  the same)
* Easily view securityContext details and POSIX capabilities
* Can specify columns to output for a more custom view
* Ability to read yaml from file or stdin for processing, including the
  `kind: List` that `kubectl get pods -o yaml` produces, plus Pod, Deployment,
  ReplicaSet, StatefulSet, DaemonSet, Job and CronJob
* Limited colour output of some sub commands: green = ok, yellow = warning,
  red = bad

[![asciicast](https://asciinema.org/a/512927.svg)](https://asciinema.org/a/512927)

## Documentation

* [openwiki/quickstart.md](openwiki/quickstart.md) - full command catalog, the
  cross-cutting flags, and the release process
* [openwiki/architecture.md](openwiki/architecture.md) - internals, and how to
  add a subcommand
* `kubectl-ice <command> -h` for the authoritative flag list of any command

## Installation

### Install using krew (custom index)

This plugin is published to the [PixiBixi krew custom index](https://github.com/PixiBixi/krew-index).

```shell
# remove the upstream plugin first if installed (same kubectl-ice binary name)
kubectl krew uninstall ice

kubectl krew index add pixibixi https://github.com/PixiBixi/krew-index.git
kubectl krew install pixibixi/ice
```
update with
```shell
kubectl krew update
kubectl krew upgrade ice
```
dont have krew? check it out here [https://github.com/GoogleContainerTools/krew](https://github.com/GoogleContainerTools/krew)

### Install from binary
- download the required binary from the [releases](https://github.com/PixiBixi/kubectl-ice/releases) page
- unzip and copy the kubectl-ice file to your path
- run ```kubectl-ice help``` to check its working

### Install from Source

requires Go 1.27 or later, clone and build the source using the following commands
```shell
git clone https://github.com/PixiBixi/kubectl-ice.git
cd kubectl-ice
make bin
```
then copy ./bin/kubectl-ice to somewhere in your path and run ```kubectl-ice version``` to check its working

### Shell completion

```shell
kubectl-ice completion zsh --install     # or bash, fish

# for kubectl ice <TAB>, put a kubectl_complete-ice shim on your PATH
ln -s $(which kubectl-ice) $(dirname $(which kubectl-ice))/kubectl_complete-ice
```

## Usage
if kubectl-ice is in your path you can replace the command ```kubectl-ice``` with ```kubectl ice``` (remove the dash) to
 make it feel more like a native kubectl command, this also works if you have kubectl set as an alias, for example
 if k is aliased to kubectl you can type ```k ice status``` instead of ```kubectl-ice status```


The following commands are available for `kubectl-ice`
```
kubectl-ice capabilities  # Shows details of configured container POSIX capabilities
kubectl-ice command       # Retrieves the command line and any arguments specified at the container level
kubectl-ice conditions    # List pod conditions for each pod
kubectl-ice cpu           # Show configured cpu size, limit and % usage of each container
kubectl-ice environment   # List the env name and value for each container
kubectl-ice image         # List the image name and pull status for each container
kubectl-ice ip            # List ip addresses of all pods in the namespace listed
kubectl-ice lifecycle     # Show lifecycle actions for each container in a named pod
kubectl-ice memory        # Show configured memory size, limit and % usage of each container
kubectl-ice node          # Show node resource allocation and pod bin-packing
kubectl-ice ports         # Shows ports exposed by the containers in a pod
kubectl-ice probes        # Shows details of configured startup, readiness and liveness probes of each container
kubectl-ice restarts      # Show restart counts for each container in a named pod
kubectl-ice security      # Shows details of configured container security settings
kubectl-ice status        # List status of each container in a pod
kubectl-ice volumes       # Display container volumes and mount points
```

plus `completion` and `version`.

ice also supports all the standard kubectl flags in addition to:
```
Flags:
  -A, --all-namespaces                 List containers from pods in all namespaces
      --annotation string              Show the selected annotation as a column
      --color string                   Add some much needed colour to the table output. string can be one of: columns, custom, errors, mix and none (overrides env variable ICE_COLOR, or ICE_COLOUR)
      --columns string                 List of column names to show in the table output, all other columns are hidden
  -c, --container string               Container name. If set shows only the named containers
      --context string                 The name of the kubeconfig context to use
  -f, --filename string                Read pod information from this yaml file instead of the k8s api
  -m, --match string                   Filters out results, comma separated list of COLUMN OP VALUE, where OP can be one of ==,<,>,<=,>= and !=
  -M, --match-only string              Filters out results but only calculates up visible rows
  -n, --namespace string               If present, the namespace scope for this CLI request
      --node-label string              Show the selected node label as a column
      --node-tree                      Displays the tree with the nodes as the root
  -o, --output string                  Output format, currently csv, list, json and yaml are supported
      --pod-label string               Show the selected pod label as a column
      --select string                  Filters pods based on their spec field, comma separated list of FIELD OP VALUE, where OP can be one of ==, = and !=
  -l, --selector string                Selector (label query) to filter on
      --show-namespace                 Shows a column containing the pods namespace name for each container
      --show-node                      Show the node name column
      --sort string                    Sort by column, prefix with ! to sort descending (cannot be used with --tree)
  -t, --tree                           Display tree like view instead of the standard list
  -w, --watch                          Watch for pod changes and reprint the table on each event
  -T, --show-type                      Show the container type column where:
                                            I = init container
                                            C = container
                                            E = ephemeral container
                                            P = Pod
                                            D = Deployment
                                            R = ReplicaSet
                                            A = DaemonSet
                                            S = StatefulSet
                                            J = Job
                                            O = CronJob
                                            N = Node
```
`node` is the exception: it takes the kubectl flags and its own, but none of the
table flags above.

subcommands add their own flags, the command each one belongs to is in brackets
```
      --oddities         Show only the outlier rows that dont fall within the computed range, needs at least 5 visible rows (cpu, memory, restarts, status)
  -i, --include-init     Include init containers, which are hidden by default here only (cpu, memory)
  -r, --raw              Show raw uncooked values (cpu, memory)
      --size string      Convert to the selected size rather than the default Mi (memory)
  -d, --details          Display the timestamp instead of age along with the message column (status)
  -p, --previous         Show previous state (status)
      --id               Show the running container id (status, image)
      --translate        Read the configmap and show its values (environment)
      --selinux          Show the SELinux context applied to the containers (security)
  -d, --device           Show raw block device mappings within a container (volumes)
      --show-ip          Show the pods IP address column (ports)
  -u, --usage            Show actual resource usage from metrics-server (node)
  -C, --compute-class    Show the GKE compute class (node)
      --class string     Filter nodes by GKE compute class (node)
      --overallocated    Show only nodes where limits exceed allocatable capacity (node)
```

## Examples
Some example commands are listed below, run `kubectl-ice <command> -h` for the full usage of any of them.


### Single pod info
Shows the currently used memory along with the configured memory requests and limits of all containers (side cars) in the pod named web-pod
```
kubectl ice memory web-pod
```
### Named containers
the optional container flag (-c) searchs all selected pods and lists only containers that match the name web-frontend
```
kubectl ice command -c web-frontend
```

### Alternate status view
the tree flag shows the containers and pods in a tree view, with values calculated all the way up to the parent
```
kubectl ice status -l app=demoprobe --tree
```

### Live updates
the watch flag re-renders the table on every pod event, press q or ctrl-c to exit
```
kubectl ice status -l app=web --watch
```

### Excluding rows
use the --match flag to show only the output rows where the used memory column is at least 4096, this has the effect of excluding any row currently under 4 MB. The USED value of the memory command is compared in kilobytes, so 4096 can be replaced with any whole number of kilobytes
```
kubectl ice mem -l app=userandomcpu --match 'used>=4096'
```

### Extra selections
using the --select flag allows you to filter the pod selection to only pods that have a priorityClassName thats equal to system-cluster-critical, you can also match against priority
```
kubectl ice status --select 'priorityClassName=system-cluster-critical' -A
```

### Column labels
with the --node-label and --pod-label flags its possible to show the values of the labels as columns in the output table
```
kubectl ice status --node-label "beta.kubernetes.io/os" --pod-label "component" -n kube-system
```

### Reading yaml instead of a cluster
```
kubectl get pods -o yaml | kubectl-ice status
kubectl-ice security -f ./deploy.yaml
```

## Contributing

All feedback and contributions are welcome, if you want to raise an issue or help
with fixes or features please
[raise an issue to discuss](https://github.com/PixiBixi/kubectl-ice/issues).

## License
Licensed under Apache 2.0, see [LICENSE](https://github.com/PixiBixi/kubectl-ice/blob/main/LICENSE)
