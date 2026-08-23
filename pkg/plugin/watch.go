package plugin

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// defaultDebounceWindow is how long we wait for more pod events before
// rebuilding. A rollout or a job burst fires many events and a rebuild refetches
// everything regardless, so one rebuild per event only adds lag.
const defaultDebounceWindow = 250 * time.Millisecond

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
	// KeyPressMsg, not the KeyMsg interface: v2 split key events into press and
	// release and both satisfy KeyMsg, so matching the interface quits on the
	// release too. Do NOT widen this back.
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m watchModel) View() tea.View {
	if m.status != "" {
		return tea.NewView(m.content + "\n" + m.status)
	}
	return tea.NewView(m.content)
}

// WatchBuild performs an initial Build+renderFn, then watches for Kubernetes pod
// events and re-renders in-place on each change using Bubble Tea.
// renderFn must return the table content as a string (use sprintTableAs).
func (b *RowBuilder) WatchBuild(loop Looper, renderFn func() (string, error)) error {
	if len(b.InputFilename) > 0 {
		return errors.New("--watch cannot be used with --filename")
	}

	stdinChanged, err := b.HasStdinChanged()
	if err != nil {
		return err
	}
	if stdinChanged {
		return errors.New("--watch cannot be used when reading from stdin")
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
		b.Connection.ClearCache()
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

			window := b.DebounceWindow
			if window <= 0 {
				window = defaultDebounceWindow
			}
			stillOpen := coalescePodEvents(ctx, watcher, window)

			rebuild()

			if !stillOpen {
				return ctx.Err() == nil // closed means reconnect, cancelled means stop
			}
		}
	}
}

// coalescePodEvents swallows further events for the debounce window so a burst
// of them produces a single rebuild. It reports whether the stream is still
// usable: false means it closed or the context was cancelled, and the caller
// must not keep reading it.
func coalescePodEvents(ctx context.Context, watcher watch.Interface, window time.Duration) bool {
	timer := time.NewTimer(window)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		case _, ok := <-watcher.ResultChan():
			if !ok {
				return false
			}
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
