package plugin

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// contentMsg carries new table content to the Bubble Tea model.
type contentMsg string

// errMsg carries a non-fatal error to display in the status line.
type errMsg string

// watchModel is the Bubble Tea model for watch mode.
type watchModel struct {
	content string
	status  string
}

func (m watchModel) Init() tea.Cmd { return nil }

func (m watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case contentMsg:
		m.content = string(msg)
		m.status = ""
	case errMsg:
		m.status = string(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m watchModel) View() string {
	if m.status != "" {
		return m.content + "\n" + m.status
	}
	return m.content
}

// WatchBuild performs an initial Build+renderFn, then watches for Kubernetes pod
// events and re-renders in-place on each change using Bubble Tea.
// renderFn must return the table content as a string (use sprintTableAs).
func (b *RowBuilder) WatchBuild(loop Looper, renderFn func() (string, error)) error {
	if len(b.InputFilename) > 0 {
		return fmt.Errorf("--watch cannot be used with --filename")
	}

	stdinChanged, err := b.HasStdinChanged()
	if err != nil {
		return err
	}
	if stdinChanged {
		return fmt.Errorf("--watch cannot be used when reading from stdin")
	}

	// Initial build
	if err := b.Build(loop); err != nil {
		return err
	}
	content, err := renderFn()
	if err != nil {
		return err
	}

	m := watchModel{content: content}
	p := tea.NewProgram(m)

	ctx, cancel := context.WithCancel(context.Background())

	// Watch goroutine feeds updates to the Bubble Tea program.
	go func() {
		defer cancel()
		for {
			watcher, err := b.Connection.WatchPods(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				p.Send(errMsg(fmt.Sprintf("watch error: %v, reconnecting…", err)))
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}

			closed := b.pipeEvents(ctx, watcher, loop, renderFn, p)
			watcher.Stop()

			if ctx.Err() != nil {
				return
			}
			if closed {
				p.Send(errMsg("watch stream ended, reconnecting…"))
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}()

	_, runErr := p.Run()
	cancel() // stop watch goroutine when Bubble Tea exits
	return runErr
}

// pipeEvents reads from a watch stream and sends content updates to the Bubble Tea program.
// Returns true when the stream closed normally (reconnect needed), false on context cancellation.
func (b *RowBuilder) pipeEvents(ctx context.Context, watcher watch.Interface, loop Looper, renderFn func() (string, error), p *tea.Program) bool {
	var tickerC <-chan time.Time
	if b.RefreshInterval > 0 {
		ticker := time.NewTicker(b.RefreshInterval)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	rebuild := func() {
		if b.PreBuildFn != nil {
			if err := b.PreBuildFn(); err != nil {
				p.Send(errMsg(fmt.Sprintf("pre-build error: %v", err)))
			}
		}
		b.Connection.ClearPodCache()
		b.resetTable()
		if err := b.Build(loop); err != nil {
			p.Send(errMsg(fmt.Sprintf("refresh error: %v", err)))
			return
		}
		content, err := renderFn()
		if err != nil {
			p.Send(errMsg(fmt.Sprintf("render error: %v", err)))
			return
		}
		p.Send(contentMsg(content))
	}

	for {
		select {
		case <-ctx.Done():
			return false
		case <-tickerC:
			rebuild()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return true // stream closed, caller should reconnect
			}
			if _, ok := event.Object.(*v1.Pod); !ok {
				continue
			}
			rebuild()
		}
	}
}

// resetTable replaces the current table with a fresh one, preserving colour settings.
func (b *RowBuilder) resetTable() {
	b.Table = &Table{
		ColourOutput:  b.Table.ColourOutput,
		CustomColours: b.Table.CustomColours,
	}
}

// WatchBuildLegacy is kept for backward compatibility with tests; it uses the old stderr approach.
// It is not used in production code.
func (b *RowBuilder) watchLegacy(loop Looper, renderFn func() error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := b.Build(loop); err != nil {
		return err
	}
	if err := renderFn(); err != nil {
		return err
	}

	for {
		watcher, err := b.Connection.WatchPods(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "\nwatch error: %v\n", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
			continue
		}
		_ = watcher
		watcher.Stop()
		return nil
	}
}
