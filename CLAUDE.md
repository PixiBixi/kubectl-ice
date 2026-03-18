# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
make bin       # Build binary → bin/kubectl-ice
make test      # Run tests with coverage (go test ./pkg/... ./cmd/...)
make fmt       # Format code (go fmt)
make vet       # Lint with go vet
```

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
- `cmd/plugin/cli/root.go` — Cobra root command setup
- `pkg/plugin/plugin.go:InitSubCommands()` — registers all subcommands

### Core Abstraction: `Looper` Interface

Every subcommand implements the `Looper` interface (`pkg/plugin/builder.go:12`):

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

1. **Command handler** (e.g., `Status()` in `pkg/plugin/status.go`) — creates a `RowBuilder`, sets flags
2. **`RowBuilder`** (`pkg/plugin/builder.go`) — connects to Kubernetes API, iterates pods/containers, calls `Looper` methods
3. **`Table`** (`pkg/plugin/table.go`) — holds `Cell` rows, handles sorting/filtering/coloring, renders to stdout

### Watch Mode (`--watch`/`-w`)

All subcommands support `--watch`/`-w` to re-render the table live on Kubernetes pod events (event-driven via `client-go` Watch API, no polling).

**Key files:**
- `pkg/plugin/watch.go` — `WatchBuild()` method on `RowBuilder`, `resetTable()`, watch loop with reconnect logic
- `pkg/plugin/k8sconnector.go` — `WatchPods(ctx)` and `ClearPodCache()`
- `pkg/plugin/builder.go` — `PreBuildFn func() error` field (used by `resources.go` to re-fetch metrics before each render)

**Pattern in each command function:**
```go
renderFn := func() (string, error) {
    // optional post-Build processing (oddities, etc.) on builder.Table
    return sprintTableAs(*builder.Table, commonFlagList.outputAs), nil
}
if commonFlagList.watch {
    return builder.WatchBuild(&loopinfo, renderFn)
}
if err := builder.Build(&loopinfo); err != nil { return err }
outputTableAs(*builder.Table, commonFlagList.outputAs)
return nil
```

On each pod event: pod cache is cleared → table reset → `Build()` re-fetches and rebuilds → screen cleared → `renderFn()` outputs. Ctrl+C exits gracefully.

**Watch mode for metrics commands** (`cpu --usage`, `memory --usage`, `resources`): pod events don't fire when only metrics change. Set `builder.RefreshInterval = 25 * time.Second` + `builder.PreBuildFn` to re-fetch metrics before each rebuild.

### Standalone Commands (no RowBuilder/Looper)

Some commands operate on non-pod resources and build the `Table` directly:
- `pkg/plugin/node.go` — iterates nodes, computes pod allocations via `GetAllPodsAllNamespaces()`

### Pod-level Commands (DontListContainers)

Commands that emit multiple rows per pod (not per container) set `builder.DontListContainers = true` and implement `BuildPodRow()` returning `[][]Cell` (e.g. `conditions.go`, `ip.go`).

### Adding a New Command

1. Create `pkg/plugin/<command>.go` — define a struct implementing `Looper`
2. Register the command in `pkg/plugin/plugin.go:InitSubCommands()`
3. Follow the pattern from an existing simple command (e.g., `pkg/plugin/image.go`)

### Key Files

| File | Role |
|------|------|
| `pkg/plugin/builder.go` | `RowBuilder` engine + `Looper` interface |
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

`setColourValue` in `pkg/plugin/utils.go`: `0–50%` → green, `51–75%` → orange, `76%+` → red.

For visually noisy multi-column commands (e.g. `node`), force `COLOUR_ERRORS` mode regardless of the user's `--color` flag to avoid rainbow columns.

### Testing Gotchas

- Import alias conflict: `resource` is already used in `resources.go` — use `apiresource` alias in test files:
  ```go
  apiresource "k8s.io/apimachinery/pkg/api/resource"
  ```
- Test files: `node_test.go`, `builder_test.go`, `k8sconnector_test.go`, `table_test.go`, `utils_test.go`
- All tests are in `package plugin` (white-box) — internal fields accessible directly

### Common Flags

All subcommands inherit flags via `processCommonFlags()` (`pkg/plugin/plugin.go`):
- Kubernetes: `-A`, `-n`, `-l`, `--context`
- Filtering: `-m/--match`, `-M/--match-only`, `--select`
- Display: `-t/--tree`, `--node-tree`, `--show-namespace`, `--show-node`, `-o`
- Watch: `-w/--watch`
- Custom columns: `--columns`, `--pod-label`, `--node-label`, `--annotation`
