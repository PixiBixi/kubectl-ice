# AGENTS.md

Guidance for coding agents working in this repository.

## OpenWiki

This repository has documentation located in the /openwiki directory.

Start here:
- [OpenWiki quickstart](openwiki/quickstart.md)

OpenWiki includes repository overview, architecture notes, workflows, domain concepts, operations, integrations, testing guidance, and source maps.

When working in this repository, read the OpenWiki quickstart first, then follow its links to the relevant architecture, workflow, domain, operation, and testing notes.

## Build & Development Commands

```bash
make bin       # Build binary -> bin/kubectl-ice
make test      # Run tests with the race detector and coverage
make lint      # Run golangci-lint (config: .golangci.yml)
make fmt       # Format code (go fmt)
make vet       # Lint with go vet
```

CI gates on every push and PR: build + `go test -race`, golangci-lint,
govulncheck, goimports and markdownlint suggestions via reviewdog, and zizmor on
the workflows themselves. golangci-lint is at zero findings, so any new one
blocks. Run `make lint` before pushing.

`hugeParam` and `rangeValCopy` run with raised thresholds, not disabled: the
`Looper` interface passes `BuilderInformation` (1408 bytes as of `k8s.io/api`
v0.37) and `v1.Pod` (1240) by value per container on purpose. Passing them by
pointer was measured at +6% time and +152% allocations, because taking the
address for an interface method call moves them to the heap while the by-value
copy is a stack memcpy. Do not "fix" that without re-measuring. Both structs
grow when `k8s.io/api` adds Pod fields, so a deps bump can need the thresholds
raised.

Run a single test:
```bash
go test ./pkg/plugin/ -run TestName -v
```

Install locally for testing:
```bash
make bin && cp bin/kubectl-ice ~/.krew/bin/  # or anywhere on PATH
```

## Architecture Overview

**kubectl-ice** is a kubectl plugin that displays container-level information from Kubernetes pods (CPU, memory, ports, security, volumes, etc.).

### Entry Point & Command Registration

- `cmd/plugin/main.go` → `cli.InitAndExecute()`
- `cmd/plugin/cli/root.go` - Cobra root command setup
- `pkg/plugin/plugin.go:InitSubCommands()` - registers all subcommands

### Core Abstraction: `Looper` Interface

Every subcommand implements the `Looper` interface (`pkg/plugin/builder.go:13`):

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

### Data Flow

1. **Command handler** (e.g., `Status()` in `pkg/plugin/status.go`) - builds a `Looper` and a `subCommand{loop, configure, filterRows, ...}` literal, then calls `runSubCommand()`
2. **`runSubCommand`/`runWithConnector`** (`pkg/plugin/run.go`) - shared plumbing every subcommand goes through: wires the `Connector`, `Table` and `RowBuilder` from the common and subcommand-specific flags, then dispatches to `builder.Build()` or `builder.WatchBuild()` and renders. This replaced per-command copies of that sequence, which had drifted (see the comment on `subCommand` in `run.go`)
3. **`RowBuilder`** (`pkg/plugin/builder.go`) - connects to Kubernetes API, iterates pods/containers, calls `Looper` methods
4. **`Table`** (`pkg/plugin/table.go`) - holds `Cell` rows, handles sorting/filtering/coloring, renders to stdout

### Watch Mode (`--watch`/`-w`)

All subcommands support `--watch`/`-w` to re-render the table live on Kubernetes pod events (event-driven via `client-go` Watch API, no polling).

**Key files:**
- `pkg/plugin/watch.go` - `WatchBuild()` method on `RowBuilder`, `resetTable()`, watch loop with reconnect logic
- `pkg/plugin/k8sconnector.go` - `WatchPods(ctx)` and `ClearCache()`
- `pkg/plugin/builder.go` - `PreBuildFn func() error` field (used by `resources.go` to re-fetch metrics before each render)
- `pkg/plugin/run.go` - `runSubCommand()`/`runWithConnector()`, the shared wiring every subcommand's watch and non-watch path goes through

`WatchBuild` renders once, then hands control to a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
program (module path `charm.land/bubbletea/v2`, not `github.com/charmbracelet/bubbletea/v2`)
that redraws in place. On a pod event, `coalescePodEvents` swallows further
events for a 250ms debounce window (a rollout or job burst fires many events,
and a rebuild refetches everything regardless, so one rebuild per event only
adds lag), then `rebuild` runs `PreBuildFn` → `Connection.ClearCache()` →
`resetTable()` → `Build()` → renders. `ClearCache()` drops every cached object,
not just pods: clearing only the pod list left the owner tree frozen on
`--watch --tree` during a rollout. When the watch stream ends, the loop waits 5
seconds and reconnects. Ctrl+C or `q` exits gracefully.

**Watch mode for metrics commands** (`cpu`, `memory`): pod events don't fire when only metrics change, so these always
set `builder.RefreshInterval = 25 * time.Second` + `builder.PreBuildFn` in watch mode, to re-fetch metrics before each rebuild regardless of pod events.

### Standalone Commands (no RowBuilder/Looper)

Some commands operate on non-pod resources and build the `Table` directly:
- `pkg/plugin/node.go` - iterates nodes, computes pod allocations via `GetAllPodsAllNamespaces()`

### Pod-level Commands (DontListContainers)

Commands that emit multiple rows per pod (not per container) set `builder.DontListContainers = true` and implement `BuildPodRow()` returning `[][]Cell` (e.g. `conditions.go`, `ip.go`).

### Adding a New Command

1. Create `pkg/plugin/<command>.go` - define a struct implementing `Looper`, and a handler function that builds a `subCommand{loop: &loopinfo, ...}` literal and calls `runSubCommand()`
2. Register the command in `pkg/plugin/plugin.go:InitSubCommands()`
3. Follow the pattern from an existing simple command (e.g., `pkg/plugin/image.go`)

### Key Files

| File | Role |
|------|------|
| `pkg/plugin/builder.go` | `RowBuilder` engine + `Looper` interface |
| `pkg/plugin/run.go` | `runSubCommand()`/`runWithConnector()`: shared wiring every subcommand goes through |
| `pkg/plugin/table.go` | Table rendering (JSON/YAML/CSV/list/text) |
| `pkg/plugin/plugin.go` | Subcommand registration + `processCommonFlags()` |
| `pkg/plugin/k8sconnector.go` | Kubernetes API client wrapper |
| `pkg/plugin/utils.go` | Filtering, matching, color utilities |
| `pkg/plugin/node.go` | Standalone node allocation command |
| `pkg/plugin/conditions.go` | Pod conditions (DontListContainers pattern) |
| `pkg/plugin/completion.go` | Shell completion (zsh/bash/fish + --install) |

### Cell Types & Tree View

Data is stored as `Cell` structs with type markers: `I`=init container, `C`=container, `E`=ephemeral, `P`=pod, `D`=deployment. These drive the `--tree` display mode.

### Color Thresholds

`setColourValue` in `pkg/plugin/utils.go`: `0-50%` → green, `51-75%` → orange, `76%+` → red.

For visually noisy multi-column commands (e.g. `node`), force `COLOUR_ERRORS` mode regardless of the user's `--color` flag to avoid rainbow columns.

### Testing Gotchas

- Import alias conflict: `resource` is already used in `resources.go` - use `apiresource` alias in test files:
  ```go
  apiresource "k8s.io/apimachinery/pkg/api/resource"
  ```
- Test files: `node_test.go`, `builder_test.go`, `k8sconnector_test.go`, `table_test.go`, `utils_test.go`
- All tests are in `package plugin` (white-box) - internal fields accessible directly

### Common Flags

All subcommands inherit flags via `processCommonFlags()` (`pkg/plugin/plugin.go`):
- Kubernetes: `-A`, `-n`, `-l`, `--context`
- Filtering: `-m/--match`, `-M/--match-only`, `--select`
- Display: `-t/--tree`, `--node-tree`, `--show-namespace`, `--show-node`, `-o`
- Watch: `-w/--watch`
- Custom columns: `--columns`, `--pod-label`, `--node-label`, `--annotation`
