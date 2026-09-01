# kubectl-ice - Quickstart

`kubectl-ice` is a single-binary **kubectl plugin** (`kubectl ice`) that shows
Kubernetes pod information **at the container level**. Where `kubectl get pods`
collapses a multi-container pod into one line, ice gives you one row per
container: its image, ports, probes, security context, volumes, lifecycle hooks,
env, restart count and live cpu/memory usage. Init and ephemeral containers are
first-class rows, not a blind spot.

- **Language / runtime:** Go 1.27, compiled to a static `kubectl-ice` binary.
- **Key deps:** `k8s.io/client-go` + `k8s.io/cli-runtime` (cluster access and the
  standard kubectl flags), `k8s.io/metrics` (cpu/memory usage),
  `spf13/cobra` (commands), `charm.land/bubbletea/v2` (watch-mode redraw).
- **Read-only:** every command lists or gets; nothing mutates the cluster.
- **Provenance:** PixiBixi fork of
  [NimbleArchitect/kubectl-ice](https://github.com/NimbleArchitect/kubectl-ice),
  Apache 2.0. See [NOTICE](../NOTICE) for what this fork changed (watch mode, the
  `node`, `conditions` and `completion` commands, performance work).

Entry point: [`cmd/plugin/main.go`](../cmd/plugin/main.go) calls
`cli.InitAndExecute()`, and [`cmd/plugin/cli/root.go`](../cmd/plugin/cli/root.go)
builds the cobra root, then `plugin.InitSubCommands(cmd)` registers everything.

## Install

Published to the [PixiBixi krew custom index](https://github.com/PixiBixi/krew-index),
not the official krew-index. The binary name collides with upstream, so remove
upstream first:

```bash
kubectl krew uninstall ice        # only if the upstream plugin is installed
kubectl krew index add pixibixi https://github.com/PixiBixi/krew-index.git
kubectl krew install pixibixi/ice
kubectl krew upgrade ice          # later, to update
```

Or drop the `kubectl-ice` binary from a release archive onto your `PATH`. From
source: `git clone && make bin`, then copy `./bin/kubectl-ice`.

## Command catalog

The authoritative list is `InitSubCommands` in
[`pkg/plugin/plugin.go`](../pkg/plugin/plugin.go). Sixteen data commands, backed
by thirteen `Looper` implementations (`cpu`/`memory` share `resources.go`,
`ip` shares `ports.go`), plus `completion` and `version`.

**Container spec (one row per container, read from `pod.Spec`)**

| Command | Aliases | Columns | Own flags |
|---|---|---|---|
| `image` | `im` | PULL, IMAGEID, CONTAINERID, IMAGE, TAG | `--id` |
| `command` | `cmd`, `exec`, `args` | COMMAND, ARGUMENTS | |
| `environment` | `env`, `vars` | NAME, VALUE | `--translate` (resolve configmap refs) |
| `ports` | `port`, `po` | PORTNAME, PORT, PROTO, HOSTPORT, IP | `--show-ip` |
| `probes` | `probe` | PROBE, DELAY, PERIOD, TIMEOUT, SUCCESS, FAILURE, CHECK, ACTION | |
| `volumes` | `volume`, `vol` | VOLUME, TYPE, BACKING, SIZE, RO, MOUNT-POINT | `-d/--device` (switches to PVC_NAME, DEVICE_PATH) |
| `lifecycle` | | LIFECYCLE, HANDLER, ACTION | |
| `security` | `sec` | ALLOW_PRIVILEGE_ESCALATION, PRIVILEGED, RO_ROOT_FS, RUN_AS_NON_ROOT, RUN_AS_USER, RUN_AS_GROUP | `--selinux` (switches to USER, ROLE, TYPE, LEVEL) |
| `capabilities` | `cap` | ADD, DROP | |

**Container status (one row per container, read from `pod.Status`)**

| Command | Aliases | Columns | Own flags |
|---|---|---|---|
| `status` | `st` | READY, STARTED, RESTARTS, STATE, REASON, EXIT-CODE, SIGNAL, ID, TIMESTAMP, AGE, MESSAGE | `-d/--details`, `-p/--previous`, `--id`, `--oddities` |
| `restarts` | `restart` | RESTARTS | `--oddities` |

**Metrics (spec plus metrics-server, requires metrics-server)**

| Command | Aliases | Columns | Own flags |
|---|---|---|---|
| `cpu` | | USED, REQUEST, LIMIT, %REQ, %LIMIT | `-i/--include-init`, `-r/--raw`, `--oddities` |
| `memory` | `mem` | same | same, plus `--size` (Ki/Mi/Gi/...) |

`cpu` and `memory` are the only commands where init containers are **hidden by
default**: `-i` opts them in. Everywhere else the builder sets
`showInitContainers: true` unconditionally, because an init container's image,
security context and probes count as much as an app container's.

If metrics-server is missing or unreachable the error is logged and the table
still prints the configured requests and limits: see the `log.Tell(err)` in
[`resources.go`](../pkg/plugin/resources.go).

**Pod level (one or more rows per pod, not per container)**

| Command | Aliases | Columns | Notes |
|---|---|---|---|
| `conditions` | `condition`, `cond` | CONDITION, STATUS, REASON, AGE, MESSAGE | one row per pod condition; `-m "STATUS!=True"` isolates what is blocking readiness |
| `ip` | | IP | shares the `ports` Looper with `DontListContainers` on |

**Node level (no Looper, no pods loop)**

| Command | Aliases | Notes |
|---|---|---|
| `node` | `nodes`, `no` | per-node requests/limits as a percentage of allocatable, pod count, bin-packing. `-u/--usage` adds live metrics, `-C/--compute-class` and `--class` read the GKE `cloud.google.com/compute-class` label, `--overallocated` keeps only nodes where limits exceed allocatable. |

**Utility**

- `completion bash|zsh|fish` writes the script to stdout, or installs it with
  `--install`. For `kubectl ice <TAB>`, symlink `kubectl_complete-ice` to the
  binary somewhere on `PATH`.
- `version` prints the goreleaser-injected version; `kubectl-ice -v` prints just
  the string.

The `README.md` command and flag lists are maintained by hand, so treat
`kubectl-ice --help` and `kubectl-ice <command> -h` as authoritative: they are
generated from the registry and cannot drift.

## Cross-cutting behaviour

Every command except `node` inherits these through `addCommonFlags` and
`processCommonFlags` ([`plugin.go`](../pkg/plugin/plugin.go)). Learn them once.

### Pod selection
The standard kubectl flags are wired in via
`genericclioptions.ConfigFlags`: `-n`, `--context`, `--kubeconfig`,
`--request-timeout` and friends. On top of them:

- positional args are pod names (a `Get` per name, no `List`);
- `-A/--all-namespaces` widens the search and turns the NAMESPACE column on;
- `-l/--selector` is a label selector, and is **mutually exclusive with a pod
  name** (`you cannot specify a pod name and a selector together`);
- `-c/--container` keeps only containers whose name matches;
- `--select 'FIELD OP VALUE'` filters on scalar `pod.Spec` fields
  (`priorityClassName`, `priority`, `nodeName`, ...) with `==`, `=`, `!=`. It is
  resolved by reflection over `v1.PodSpec`, restricted to string/int/bool
  fields, so a field name that is not scalar silently matches nothing.

With no `-n` and no `-A`, the namespace comes from the current kubeconfig
context, resolved once per invocation and cached.

### Row filtering: `--match`, `--match-only`, `--oddities`
`-m/--match` takes a comma-separated list of `COLUMN OP VALUE`, where OP is one
of `==`, `=`, `!=`, `<`, `<=`, `>`, `>=`. The column name must be an actual
header (uppercase), otherwise the run fails with `invalid column name specified`.
String comparisons support `*` and `?` wildcards, and `*` crosses `/`, so
`--match 'IMAGE=*nginx*'` works on a registry path.

```bash
kubectl ice mem -l app=web --match 'USED>=4096'   # drop rows under 4096 kB used
kubectl ice status --match 'READY=false'
```

> **A numeric match compares against the cell's stored value, not the displayed
> text**, and for `memory` those units differ per column: `USED` is stored in kB
> (`metrics.Memory().Value() / 1000` in `resources.go`) while `REQUEST` and
> `LIMIT` are stored in bytes. So `USED>=4096` means 4096 kB, and
> `REQUEST>=4096` means 4096 bytes. `--size` changes only the rendering, never
> the comparison.

`-M/--match-only` filters the same way but excludes the hidden rows from the
tree totals, so a parent row sums only what you can see.

`--oddities` (on `cpu`, `memory`, `restarts`, `status`) keeps only the outliers:
it computes quartile fences at 1.5x the interquartile range on one numeric column
and hides everything inside them. It needs **at least 5 visible rows** to mean
anything and errors out below that. On `status` it is skipped in tree view and
under `--previous`, where an outlier is the point rather than the anomaly.

### Tree view
`-t/--tree` walks each pod up its `ownerReferences` chain (Pod, ReplicaSet,
Deployment, DaemonSet, StatefulSet, Job, CronJob) and prints a tree with values
aggregated into the parent rows. `--node-tree` roots the same tree at the node
instead. Fourteen commands offer it; `ip` and `node` do not.

`--tree` and `--sort` are **mutually exclusive** and the run is rejected: the
tree order is the hierarchy, and sorting rows would break it.

`-T/--show-type` reveals the `T` column that names each row's kind:
`I`=init container, `C`=container, `E`=ephemeral, `P`=pod, `D`=Deployment,
`R`=ReplicaSet, `A`=DaemonSet, `S`=StatefulSet, `J`=Job, `O`=CronJob, `N`=node.

### Columns
Default columns are `T`, `NAMESPACE`, `NODE`, `PODNAME`, `CONTAINER`, most of
them hidden until asked for: `--show-namespace`, `--show-node`, `-T`. The pod
name column hides itself when you named exactly one pod.

Three flags add columns from cluster metadata: `--node-label`, `--pod-label`,
`--annotation`. `--columns A,B,C` inverts the logic and shows only the named
columns.

`--sort COLUMN` sorts by header name, `!COLUMN` descending, and repeats
left to right (`--sort 'PODNAME,!RESTARTS'`) because each pass is stable.

### Output formats
`-o csv|list|json|yaml`, default being the aligned text table. Any other value
is rejected.

> **The structured formats are raw dumps, not the rendered table.** Only the text
> renderer honours hidden columns, hidden rows, the sort order and the tree
> placeholder rows. `-o json|yaml|csv|list` walks the raw row slice with every
> column, so it also returns what `--columns`, `--oddities` and the column-hiding
> flags removed, in insertion order. See
> [architecture.md](architecture.md#cell-and-table-tablego).

### Colour
`--color columns|mix|errors|custom|none`, or the **`ICE_COLOR`** environment
variable. `ICE_COLOUR` is accepted as an alias, because upstream kubectl-ice only
ever documented that spelling: the flag beats both, and `ICE_COLOR` wins when
both are set.

- `columns` gives each column a colour off a 14-entry wheel, for scanning wide
  tables sideways.
- `errors` colours only semantic cells: green 0-50%, orange 51-75%, red 76%+ for
  utilisation, green/red for ready-style booleans.
- `mix` keeps the wheel on headers and semantic colours on cells.
- `custom;<mod>.<code>;...` sets the wheel by hand, and a `g`/`w`/`b` prefix
  overrides the good/warning/bad colours instead.

`node` forces `errors` whenever colour is on at all: a colour wheel across its
14 columns hides the signal it exists to show.

### Watch mode
`-w/--watch` renders once, then re-renders in place on Kubernetes pod events,
event-driven through the client-go Watch API (no polling). `q` or `ctrl-c` exits.
It refuses to run with `--filename` or piped stdin, and `node` does not offer it.

Pod events do not fire when only the **usage numbers** move, so `cpu`, `memory`
and `resources` add a 25 second ticker on top of the event stream. Details in
[architecture.md](architecture.md#watch-mode).

### Reading yaml instead of a cluster
`-f/--filename file.yaml`, or a pipe on stdin, replaces the API entirely:

```bash
kubectl get pods -o yaml | kubectl-ice status
kubectl-ice security -f ./deploy.yaml
```

Supported kinds are `List` (what `kubectl get pods -o yaml` produces), `Pod`,
`Deployment`, `ReplicaSet`, `StatefulSet`, `DaemonSet`, `Job` and `CronJob`;
workloads contribute their pod template. An unrecognised kind contributes
nothing and the run ends with an explicit error rather than an empty table.

> A pipe with no data on it blocks forever. After 2 seconds ice prints
> `waiting for yaml on stdin, press ctrl-c to cancel` to stderr, so a hang reads
> as a hang and not as a slow cluster. When scripting, pass `< /dev/null` if the
> command should never read stdin.

### Interrupts and debugging
`ctrl-c` cancels the request in flight (the context comes from cobra through
`signal.NotifyContext`) and exits `130`. A second `ctrl-c` kills the process.
client-go's own klog output is discarded: it is transport noise a plugin user
cannot act on.

`ICE_LOG=debug` turns on the internal `logger.Debug` trace on stdout.

## Develop and test

```bash
make bin       # go fmt + go vet, then build to bin/kubectl-ice
make test      # go test -race ./pkg/... ./cmd/... -coverprofile cover.out
make lint      # golangci-lint run (config: .golangci.yml)

go test ./pkg/plugin/ -run TestName -v          # single test
make bin && cp bin/kubectl-ice ~/.krew/bin/     # install locally
```

CI gates every push and PR to `main`: [`ci.yml`](../.github/workflows/ci.yml)
runs `go mod verify`, build and `go test -race`;
[`lint.yml`](../.github/workflows/lint.yml) runs golangci-lint at **zero
findings**, so any new one blocks. Separate workflows add `govulncheck`,
goimports and markdownlint suggestions through reviewdog, and `zizmor` on the
workflows themselves. Run `make lint` before pushing.

Three more gates run on pull requests:
[`codeql.yml`](../.github/workflows/codeql.yml) (also on push to `main`, plus a
weekly cron so a query published after the last push still gets to run),
[`dependency-review.yml`](../.github/workflows/dependency-review.yml) on the
dependency diff, and
[`validate-pr-title.yml`](../.github/workflows/validate-pr-title.yml), which
holds the PR title to Conventional Commits, the same vocabulary `svu` reads to
decide the next version.

Tests live beside the code in `package plugin` (white-box), and use client-go's
fake clientset. See the [Testing section](architecture.md#testing) for the
helpers and the gotchas.

## Release

Releases are **automatic on push to `main`**.
[`release.yml`](../.github/workflows/release.yml) runs
[`svu`](https://github.com/caarlos0/svu) to compute the next `vX.Y.Z` from the
conventional commits since the last tag (`feat` minor, `fix` patch); when
`svu next` equals `svu current` nothing is releasable and the job stops there.
Otherwise it creates the tag with one `gh api` call, so the checkout keeps
`persist-credentials: false`, then runs goreleaser in the same job. A
`GITHUB_TOKEN`-created tag does not re-trigger a workflow, which is why tagging
and releasing live together. So you release by writing the right commit type,
not by tagging. Pushing a `v*` tag by hand still works as an escape hatch.

`--v0` keeps a breaking change from jumping straight to `v1.0.0` while the
project is pre-1.0. And `perf:` **does not cut a release**: svu implements the
Conventional Commits spec, where only `fix` and `feat` are normative. Use `fix:`
for a change that has to ship on its own.

goreleaser ([`.goreleaser.yml`](../.goreleaser.yml)) builds linux/darwin on
amd64/arm64, injects the version through
`-X .../cmd/plugin/cli.version`, and pushes the regenerated `ice.yaml` to
[PixiBixi/krew-index](https://github.com/PixiBixi/krew-index) with the
`KREW_INDEX_TOKEN` PAT. It also emits an **SBOM per archive** (goreleaser
shells out to `syft`, which the workflow installs first: a missing binary fails
the release only after the archives are built) and a **keyless cosign signature
over `checksums.txt`**, which covers every archive at once since it holds their
SHA256. cosign v3 writes a single `.sigstore.json` bundle rather than the old
`.sig` plus `.pem` pair.

On top of that, every release gets a **build-provenance attestation** over the
archives, `checksums.txt` and the SBOMs, signed keylessly with the job's
`id-token`. A signature says who published; provenance says how the artifact was
built. Verify a download with:

```bash
gh attestation verify kubectl-ice_<version>_Darwin_arm64.tar.gz \
  --repo PixiBixi/kubectl-ice
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/PixiBixi/kubectl-ice/'
```

Keyless means there is no private key to store or rotate: the identity comes from
the GitHub OIDC token and lands in the Sigstore transparency log.

Renovate ([`renovate.json`](../renovate.json)) drives the version bumps, and
`forkProcessing` is on because Renovate skips forks by default. Minor Go-module
updates map to `feat(deps)` (minor release), patch/digest to `fix(deps)` (patch
release), GitHub-Actions updates stay `chore(deps)` (no release: they do not ship
in the binary). All `k8s.io/**` and `sigs.k8s.io/**` modules are batched into one
PR because they share a release train and must move in lockstep. Minor/patch/
digest updates automerge once CI passes, but only after a **5-day
`minimumReleaseAge` cooldown**: a hijacked package is usually spotted and yanked
within days, so waiting costs nothing and catches the window that matters.
`vulnerabilityAlerts` overrides the cooldown, so CVE fixes still land at once.

## Playground

[`k8s-templates/`](../k8s-templates) holds the demo manifests used to generate
documentation output: pods with random cpu, deliberately broken pods, probes,
volumes, a job and a daemonset. `./up.sh` applies them, `./down.sh` removes them.

## Where to go next

- **[architecture.md](architecture.md)** - the `Looper`/`RowBuilder`/`Table`/
  `Connector` layering, the shared run sequence, the owner-tree builder, the
  API caches, the watch loop, the measured performance decisions you must not
  undo, and a step-by-step guide to adding a subcommand.
