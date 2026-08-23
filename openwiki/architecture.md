# Architecture

Everything lives in two packages. `cmd/plugin/cli` owns the process (cobra root,
signals, klog silencing); `pkg/plugin` owns everything else. Inside
`pkg/plugin` the flow is layered:

```
cmd/plugin/main.go          auth provider imports, calls cli.InitAndExecute
  └─ cmd/plugin/cli/root.go cobra root, ExecuteContext with signal.NotifyContext, exit codes
       └─ pkg/plugin/plugin.go    the command registry + common flag parsing
            └─ run.go             the sequence every subcommand shares
                 ├─ <command>.go  a Looper: one file per subcommand, builds cells
                 ├─ builder.go    RowBuilder: walks pods and containers, calls the Looper
                 ├─ table.go      Cell/Table: hide, sort, colour, render
                 └─ k8sconnector.go  Connector: client-go, caches, owner tree
```

`pkg/plugin` is importable but is not a published API: the plugin is consumed as
a binary, which is why revive's `exported` and `package-comments` rules are off
in [`.golangci.yml`](../.golangci.yml).

## The shared run sequence (`run.go`)

Every subcommand is a call to `runSubCommand(cmd, kubeFlags, args, subCommand{...})`
([`run.go`](../pkg/plugin/run.go)). It loads the config, parses the common flags,
builds the `Table` and the `RowBuilder`, then builds and renders.

```go
type subCommand struct {
    loop               Looper  // builds the rows
    loopSpec           bool    // iterate pod.Spec.Containers
    loopStatus         bool    // iterate pod.Status.ContainerStatuses
    showInitContainers bool
    dontListContainers bool    // one or more rows per pod instead

    configure  func(runContext) error // read the subcommand's own flags
    filterRows func(runContext) error // --oddities, applied before rendering
}
```

**This exists because that sequence used to be copied into all thirteen
subcommands, and the copies had drifted**: one dropped the error from `Build`,
one passed its `Looper` by value, and two kept the `--oddities` filter in two
places that could disagree. Adding a step here reaches every command at once.

Two ordering rules the struct encodes:

- `configure` runs **after** `SetFlagsFrom` has applied the common flags, and
  `SetFlagsFrom` only ever turns fields on, so a choice made in `configure`
  cannot be lost. `status --details`, which forces the type column on, relies on
  that.
- `PodName` is set on the builder **before** `SetFlagsFrom`, which reads it to
  decide whether the PODNAME column is worth showing.

`finish()` (sort, then `filterRows`) sits between a built table and a printed one
and is called from **both** the one-shot and the watch path, so `--sort` and
`--oddities` behave identically whether the table was built once or is being
rebuilt on a pod event. `BuildContainerTable` deliberately does **not** sort:
sorting belongs to the caller, because the tree path and the watch rebuild do not
go through it.

`runWithConnector` is the same function once the connection exists. It is split
out so a test can hand it a `Connector` backed by client-go's fake and exercise
the whole sequence without an API server (see `run_test.go`).

## The `Looper` interface

Defined in [`builder.go`](../pkg/plugin/builder.go). One file per subcommand
implements it, and most implementations return an empty slice from the hooks
they do not use.

```go
type Looper interface {
    BuildBranch(info BuilderInformation, rows [][]Cell) ([]Cell, error)
    BuildContainerSpec(container v1.Container, info BuilderInformation) ([][]Cell, error)
    BuildEphemeralContainerSpec(container v1.EphemeralContainer, info BuilderInformation) ([][]Cell, error)
    BuildContainerStatus(container v1.ContainerStatus, info BuilderInformation) ([][]Cell, error)
    BuildPodRow(pod v1.Pod, info BuilderInformation) ([][]Cell, error)
    Headers() []string
    HideColumns(info BuilderInformation) []int
}
```

- `Headers()` names the command's own columns. They are appended after the
  default columns, and `HideColumns` returns indexes **relative to the
  command's own set** (`builder.go` offsets them by `DefaultHeaderLen`).
- `BuildContainerSpec` / `BuildContainerStatus` return `[][]Cell`: one row per
  call is the common case, but `probes` and `volumes` emit several.
- `BuildBranch` computes a parent row in tree view from its children's rows.
  A command with nothing to aggregate returns a slice of empty cells of the right
  length; the length still has to match, or `Table.AddRow` panics.
- `BuildPodRow` is only reached with `dontListContainers` (see `conditions.go`,
  and `ports.go` under the `ip` alias).

`Headers()` may depend on a flag: `security --selinux` and `volumes --device`
return a different column set entirely. That is why `configure` runs before
`LoadHeaders`.

## `RowBuilder` (`builder.go`)

`Build(loop)` is the engine:

1. Detect whether stdin was redirected (`HasStdinChanged`).
2. `LoadHeaders`: default columns + `Headers()`, then apply `HideColumns`, the
   `--match` filter setup (which needs the header names to resolve column names)
   and `setVisibleColumns`.
3. Fetch pods, from the API (`Connector.GetPods`) or from yaml
   (`loadYaml`, see below).
4. Either `BuildContainerTable` (flat) or `walkTreeCreateRow` (tree).
5. Apply `--columns` last, since it overrides every other hiding decision.

`podLoop` is where the container enumeration order is fixed: **init, then
regular, then ephemeral**, and within each, status rows before spec rows. A
command sets `loopSpec`, `loopStatus` or both and gets only the halves it asked
for.

### Default columns

`getDefaultHead` / `getDefaultCells` produce `T`, `NAMESPACE`, `NODE`, `PODNAME`,
`CONTAINER` in the flat case, and `T`, `NAMESPACE`, `NODE`, then a computed
`NAME` in tree view (`NAME` is built outside the helper so the builder controls
its indentation and the `Kind/name` prefix). Label, pod-label and annotation
columns are inserted between the two groups, in that fixed order, by both
`getDefaultHead` and `makeFullRow`. **Those two must stay in lockstep**: a column
added to one and not the other shifts every cell after it.

`setVisibleColumns` then hides by index (0 = `T`, 1 = NAMESPACE, 2 = NODE,
3 = PODNAME, 4 = CONTAINER), which is why those indexes are hard-coded there and
nowhere else.

### The `--match` filter

`setFilter` resolves each `COLUMN OP VALUE` to a column index once, at header
load, and pre-parses the value as both int64 and float64. `matchShouldExclude`
then runs per row over `[]matchFilter` indexed by column, dispatching on the
cell's own type. The int/float pre-parse and the index-aligned slice exist to
keep the per-row work allocation free: this runs once per row per filter, and it
was measured (see `match_bench_test.go`).

Only `--match-only` (`CalcFiltered`) changes the tree totals: with plain
`--match`, hidden rows still count toward their parent.

## `Cell` and `Table` (`table.go`)

A `Cell` carries the text plus a typed value (`typ` 0 string, 1 int64, 2 float64,
3 placeholder, -1 empty), an indent level and a colour pair. The typed value is
what makes numeric sorting, numeric `--match` and the oddities fences possible on
a table whose cells are otherwise strings. Build cells with the `NewCell*`
constructors, never by hand.

`Table.AddRow` **panics** when the row is shorter than the header count: rows are
built from the header definition, so a short row means a `Looper` returned the
wrong shape, and the panic names both counts rather than corrupting the table.

### Placeholder rows

Tree view needs a parent row printed *above* its children but computed *after*
them. `AddPlaceHolderRow` reserves the slot and returns an id;
`UpdatePlaceHolderRow(id, cells)` fills it in later, and `HidePlaceHolderRow`
drops it when the branch got filtered out. Only the text renderer resolves
placeholders back to their cells.

### Sorting

`SortByNames` maps header names to column indexes (`!NAME` for descending) and
calls `sort` once per column. **`sort` must stay a stable sort**
(`slices.SortStableFunc`): `SortByNames` relies on earlier columns keeping their
relative order for a multi-column sort to mean anything. Float comparison is
written out explicitly rather than through `cmp.Compare`, because `cmp` orders
NaN below every value and metrics can divide by zero, which would reshuffle rows
the previous implementation left untouched (`TestSortHandlesNaN`).

### The oddities fences

`ListOutOfRange(columnID)` sorts the column, takes the quartile rows
(`fenceQuartileRows`, averaging two rows when the middle falls between), and
excludes everything **inside** `q3 + 1.5*(q3-q1)` and `1.5*(q3-q1) - q1`. It
refuses to run on a string column or on 4 or fewer visible rows. The int path
truncates on integer division, as it always did, and
`TestGetFencesMatchesOldImplementation` pins that behaviour.

`hideOutOfRange` in `run.go` is the caller. Its `columnID` is **absolute**:
`restarts` passes a literal `4`, while `status` and `resources` offset from
`DefaultHeaderLen`, which only exists after `Build` has run.

### Rendering

`Fprint` (text) is the only renderer that honours `columnOrder`, hidden columns,
hidden rows, the sort order and placeholder resolution. It also owns the colour
logic: a 14-entry colour wheel per column, overridden by the cell's own semantic
colour under `errors` and `mix`.

`FprintJson`, `FprintYaml`, `FprintCsv` and `FprintList` walk `t.data` directly
with every column, so **they ignore hiding, ordering and placeholders**. Worth
knowing before you treat `-o json` as the table's contents. Both writers escape
properly: json goes through `jsontext.AppendQuote` and csv doubles embedded
quotes per RFC 4180, after a period where an unescaped quote produced invalid
output for every parser.

`strMatch` implements the `*`/`?` wildcards used by `--match`. `*` crosses `/`,
unlike `path.Match`, because `--match IMAGE=*app*` has to match a registry path.
It is a two-pointer scan with backtracking, not the previous dynamic-programming
table that allocated two slices per call, once per row per filter.

## `Connector` (`k8sconnector.go`)

The client-go wrapper, plus every cache. It holds `kubernetes.Interface` and
`metricsclientset.Interface` as **interfaces rather than concrete clientsets**, so
a test can inject client-go's fake and exercise the cache logic without an API
server.

`LoadConfig(ctx, configFlags)` stores the cobra context, and every API call goes
through `requestContext()`, which falls back to `context.Background()` so a
`Connector` built without `LoadConfig` (as the tests do) still works. That is
what makes `ctrl-c` abandon a request in flight instead of leaving it running
server side.

### Caches and the cluster-wide list

Pods, ReplicaSets, Deployments, DaemonSets, StatefulSets, Jobs, CronJobs and
ConfigMaps are each cached in the `Connector`, keyed by namespace for the
workload kinds. Two rules matter:

- **With `-A`, one cluster-wide list replaces one list per namespace.** On a
  cluster with 289 namespaces that was 289 sequential round trips *per kind*.
  Each `Load*` sets `listNamespace = ""` when `allNamespaces` is on.
- `recordLoaded` marks a kind as fetched so a cache **miss is not retried**
  against the API server. With `-A` the whole cluster was listed, so a later miss
  is real. Without it only one namespace was listed, and an empty result still
  has to be recorded, or every lookup in that namespace lists again.

**Anything keyed by pod must be keyed by `namespace + "/" + name`.** Two
StatefulSet pods can share a name across namespaces (`thanos-receive-0` listed
with `-A`), and keying on the name alone made the last one overwrite the others,
so every row showed identical values. That is what `podMetaMap` in
`k8sconnector.go` and `podMetrics2Hashtable` in `resources.go` both do, and what
`resources_test.go` pins.

`GetNamespace` resolves the kubeconfig namespace at most once per invocation
(reading kubeconfig from disk is the expensive part) and caches the result.

`ClearCache` drops **every** cached object, not just the pod list. Clearing only
the pods left the owner tree frozen at whatever it was on the first render, so a
rollout during a `--watch --tree` session kept showing the old ReplicaSet as the
parent.

`GetNodes` switches on the number of names asked for: list-all, a single `Get`
(cheapest), or one `List` plus a client-side filter, which beats N sequential
GETs.

### The owner tree

`BuildOwnersList` builds a `LeafNode` tree from the pod list.
For each pod it walks `ownerReferences` upward through `appendParents`
(ReplicaSet, Deployment, DaemonSet, StatefulSet, Job, CronJob, and the node from
`pod.Spec.NodeName` when a pod has no owner), then reverses the chain to
root-to-leaf order and grafts it in. `appendParents` **appends** and the caller
reverses, rather than prepending at each level. `LeafNode.getChild` keeps a
`childIndex` map alongside the slice for O(1) lookup, since the same parent is
hit once per pod.

Note that the recursion resolves each owner through `GetDeployment`,
`GetReplicaSet` and friends, which is exactly why the workload caches exist: a
1000-pod Deployment would otherwise re-fetch the same ReplicaSet 1000 times.

## Watch mode

[`watch.go`](../pkg/plugin/watch.go). `WatchBuild` renders once, then hands
control to a [Bubble Tea](https://github.com/charmbracelet/bubbletea) program
that redraws in place; a goroutine feeds it `contentMsg` on each rebuild and
`errMsg` for non-fatal problems, shown on a status line under the table.

The loop:

1. `Connection.WatchPods(ctx)` opens a pod watch, scoped to the same namespace
   and label selector the one-shot path uses.
2. `pipeEvents` reads the stream. On a pod event it calls `coalescePodEvents`,
   which **swallows further events for a 250 ms debounce window**: a rollout or a
   job burst fires many events and a rebuild refetches everything anyway, so one
   rebuild per event only adds lag. `coalescePodEvents` reports whether the stream
   is still usable, so the caller never keeps reading a closed channel.
3. `rebuild` runs `PreBuildFn`, `Connection.ClearCache()`, `resetTable()`,
   `Build()`, then `renderFn()`.
4. When the stream ends, the outer loop waits 5 seconds and reconnects,
   surfacing `watch stream ended, reconnecting…` in the status line.

`resetTable` builds a fresh `Table` but carries the colour settings over, since
those come from flags and do not change between renders.

**Metrics do not produce pod events.** `cpu`, `memory` and `resources` therefore
set `RefreshInterval = 25 * time.Second` and `PreBuildFn = fetchMetrics`, adding
a ticker that rebuilds independently of the event stream. Any future command
whose data can change without a pod event needs the same two fields.

`--watch` is rejected with `--filename` or piped stdin: there is nothing to
watch.

## Reading yaml (`builder_yamlreader.go`)

`loadYaml` splits the input on `---` and hands each document to
`convertFromYaml`, which dispatches on `kind`: `Pod` as is, `List` recursively
over its items, and each workload kind through its pod template (keeping the
workload's name so the output says where the row came from). An unrecognised kind
contributes nothing, and `hasPodData` is what distinguishes that from a real but
empty pod, which used to render as an empty table with no hint that the input was
not understood.

Two things not to undo:

- Documents accumulate in a `strings.Builder`. One `kubectl get pods -o yaml` is
  a single document tens of thousands of lines long, and growing a string by
  reassignment made that quadratic.
- Reading stdin starts a 2 second timer that prints `waiting for yaml on stdin`
  to stderr, cancelled by the first line through a `sync.OnceFunc`. A pipe with
  no data blocks forever, and a silent hang reads as a stuck API call.

## Standalone: the `node` command

[`node.go`](../pkg/plugin/node.go) has no `Looper` and no `RowBuilder`: it lists
nodes, calls `GetAllPodsAllNamespaces()` and sums requests and limits per node
itself, then fills a `Table` directly. Consequences worth knowing before you
extend it: no `--tree`, no `--watch`, no `--match`, and pod allocation is
**always** computed across all namespaces regardless of `-n`, because a per-node
total that only counts your namespace is wrong rather than filtered.

It also forces `COLOUR_ERRORS` whenever colour is enabled at all: a colour wheel
across its 14 columns is visually noisy and hides the semantic colours (STATUS,
and the utilisation percentages) that are the point of the command.

## Performance decisions you must not undo

Each of these is a measurement, not a preference. Re-measure before reverting.

- **`hugeParam` and `rangeValCopy` are enabled with raised thresholds** (1400 and
  1200 in [`.golangci.yml`](../.golangci.yml)), not disabled. The `Looper`
  interface passes `BuilderInformation` (1336 bytes) and `v1.Pod` (1168) **by
  value per container, on purpose**. Pointers were measured at +6% time and +152%
  allocations: taking the address for an interface method call moves them to the
  heap, while the by-value copy is a stack memcpy. The thresholds sit just above
  those two, so anything larger is still caught.
- **`Table.sort` must stay stable**, and float comparison must stay explicit (see
  Sorting above).
- **`strMatch` stays the two-pointer scan**, not a DP table.
- **The yaml reader keeps its `strings.Builder`.**
- **`--match` keeps its pre-parsed ints/floats and index-aligned filter slice.**
- **`-A` keeps listing cluster-wide once per kind.**

Benchmarks live next to the code: `builder_bench_test.go`,
`builder_yamlreader_bench_test.go`, `match_bench_test.go`. Run them with
`go test ./pkg/plugin/ -bench . -run '^$'` and compare with `benchstat`. No
before/after numbers means no performance claim.

## Adding a subcommand

1. Create `pkg/plugin/<command>.go`. Define the `Short`/`Description`/`Example`
   strings (the example string is formatted with `%[1]s` for the command path),
   a struct implementing `Looper`, and an exported entry function that calls
   `runSubCommand` with the right `subCommand` flags. Copy the shape from
   [`image.go`](../pkg/plugin/image.go) for a spec command,
   [`restarts.go`](../pkg/plugin/restarts.go) for a status command, or
   [`conditions.go`](../pkg/plugin/conditions.go) for a pod-level one.
2. Register it in `InitSubCommands` ([`plugin.go`](../pkg/plugin/plugin.go)):
   the cobra command, its aliases, `KubernetesConfigFlags.AddFlags`, its own
   flags, then `addCommonFlags(cmd)` and `rootCmd.AddCommand(cmd)`. Add the
   `tree`/`node-tree` flags only if the command supports tree view.
3. Build cells with the `NewCell*` constructors, and return a row whose length
   always equals `len(Headers())` on every code path, including the empty one.
   `BuildBranch` included.
4. If the command reads its own flags, do it in `configure`, not before: the
   common flags have not been applied yet.
5. For a `--oddities`-style filter, use `filterRows` with `hideOutOfRange`, and
   remember the column id is absolute.
6. Add a `_test.go` next to it. `looper_test.go` already carries a pod that
   exercises every column kind; extend it rather than building a new fixture.
7. Update the docs the change reaches: `README.md`'s command and flag lists,
   which are hand-maintained and drift silently, the
   [quickstart catalog](quickstart.md#command-catalog), and this page if you
   touched the builder, the table or the connector.

## Testing

All tests are in `package plugin` (white-box), so internal fields are reachable
directly. They use `k8s.io/client-go/kubernetes/fake` for the API, run the
command against a captured `os.Stdout`, and assert on substrings. The metrics
client has no fake wired up yet, so the metrics path is covered by unit-testing
`podMetrics2Hashtable` directly (`resources_test.go`).

Gotchas that will bite:

- **`os.Stdin` must point at `/dev/null`.** `Build` treats any stdin that is not
  a character device as yaml to read, so a test harness pipe makes it block
  forever. `captureStdout` in `run_test.go` does this; reuse it.
- **Import alias:** `resource` is taken by `resources.go`, so test files import
  `apiresource "k8s.io/apimachinery/pkg/api/resource"`.
- Command tests should go through the **real cobra command**
  (`runTestCommand` builds it via `InitSubCommands` and calls `ParseFlags`) rather
  than a hand-rolled flag set, so the test reads the same flags production does.
- `runWithConnector` is the seam for exercising the full sequence with a fake
  clientset.

Test files: `run_test.go`, `looper_test.go`, `builder_test.go`,
`builder_yamlreader_test.go`, `k8sconnector_test.go`,
`k8sconnector_cache_test.go`, `k8sconnector_pod_test.go`, `table_test.go`,
`table_render_test.go`, `table_sort_test.go`, `table_fences_test.go`,
`node_test.go`, `resources_test.go`, `security_test.go`, `status_test.go`,
`utils_test.go`, `utils_colour_test.go`, `watch_test.go`, `plugin_test.go`.

## Where to change what

| You want to... | Touch |
|---|---|
| Add or rename a command | `InitSubCommands` in `plugin.go` + a new `<command>.go` + its test |
| Add a flag shared by every command | `addCommonFlags` **and** `processCommonFlags` in `plugin.go` |
| Change a command's columns | its `Headers()` / `HideColumns()` |
| Change the default columns | `getDefaultHead` **and** `getDefaultCells` in `builder.go`, plus the indexes in `setVisibleColumns` |
| Change the container enumeration | `podLoop` in `builder.go` |
| Change the shared run sequence | `subCommand` / `runWithConnector` in `run.go` |
| Change `--match` semantics | `setFilter` + `matchShouldExclude` in `builder.go`, `strMatch` in `table.go` |
| Change sorting | `sort` / `SortByNames` in `table.go` |
| Change the `--oddities` range | `ListOutOfRange` + `getFences*` in `table.go`, callers via `hideOutOfRange` |
| Change text alignment or colour | `Fprint` in `table.go`, `setColourValue` in `utils.go` |
| Change an output format | the matching `Fprint*` in `table.go` |
| Add an API resource kind or change caching | `k8sconnector.go` (`Load*` + `recordLoaded` + `ClearCache`) |
| Change the tree hierarchy | `BuildOwnersList` / `appendParents` in `k8sconnector.go`, `walkTreeCreateRow` in `builder.go` |
| Change watch behaviour | `watch.go` (+ `RefreshInterval`/`PreBuildFn` on the command) |
| Support another yaml kind | `convertFromYaml` in `builder_yamlreader.go` |
| Change node allocation maths | `computeNodeAllocations` in `node.go` |
| Change shell completion | `completion.go` |
