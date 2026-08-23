package plugin

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// TestCoalescePodEventsSwallowsBurst is the debounce: a rollout fires many pod
// events and a rebuild refetches everything anyway, so the burst has to collapse
// into one rebuild. synctest gives a virtual clock, so the window elapses
// instantly instead of the test sleeping through it.
func TestCoalescePodEventsSwallowsBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := watch.NewFakeWithChanSize(32, false)
		defer fake.Stop()

		for range 10 {
			fake.Add(&v1.Pod{})
		}

		start := time.Now()
		open := coalescePodEvents(context.Background(), fake, 250*time.Millisecond)

		if !open {
			t.Error("the stream is still open, want true")
		}
		if elapsed := time.Since(start); elapsed != 250*time.Millisecond {
			t.Errorf("waited %v, want the full 250ms window", elapsed)
		}
		if n := len(fake.ResultChan()); n != 0 {
			t.Errorf("%d events left unread, the burst was not swallowed", n)
		}
	})
}

// TestCoalescePodEventsStreamClosed covers the watcher dying inside the window:
// the caller has to learn about it rather than keep reading a dead channel.
func TestCoalescePodEventsStreamClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := watch.NewFakeWithChanSize(4, false)
		fake.Stop()

		if coalescePodEvents(context.Background(), fake, time.Minute) {
			t.Error("reported the stream open after it closed")
		}
	})
}

// TestCoalescePodEventsCancelled covers an interrupt landing inside the window.
func TestCoalescePodEventsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := watch.NewFakeWithChanSize(4, false)
		defer fake.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		if coalescePodEvents(ctx, fake, time.Hour) {
			t.Error("reported the stream open after the context was cancelled")
		}
		if elapsed := time.Since(start); elapsed != 10*time.Millisecond {
			t.Errorf("returned after %v, want as soon as the context was cancelled", elapsed)
		}
	})
}

func TestWatchModelUpdate(t *testing.T) {
	tests := []struct {
		name       string
		msg        tea.Msg
		wantView   string
		wantQuit   bool
		startState watchModel
	}{
		{
			name:       "content replaces the table and clears the status",
			msg:        contentMsg("NAME\nrow"),
			wantView:   "NAME\nrow",
			startState: watchModel{content: "old", status: "watch error"},
		},
		{
			name:       "an error shows under the table",
			msg:        errMsg("watch error: boom"),
			wantView:   "NAME\nrow\nwatch error: boom",
			startState: watchModel{content: "NAME\nrow"},
		},
		{
			name:       "q quits",
			msg:        tea.KeyPressMsg{Code: 'q', Text: "q"},
			wantView:   "table",
			wantQuit:   true,
			startState: watchModel{content: "table"},
		},
		{
			name:       "ctrl+c quits",
			msg:        tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl},
			wantView:   "table",
			wantQuit:   true,
			startState: watchModel{content: "table"},
		},
		{
			// v2 split key events into press and release, and both satisfy
			// tea.KeyMsg. Matching the interface would quit on the release too,
			// so releasing a key held from before the program started would end
			// the session.
			name:       "a key release does not quit",
			msg:        tea.KeyReleaseMsg{Code: 'q', Text: "q"},
			wantView:   "table",
			startState: watchModel{content: "table"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, cmd := test.startState.Update(test.msg)

			if got := model.View().Content; got != test.wantView {
				t.Errorf("View() = %q, want %q", got, test.wantView)
			}
			if gotQuit := cmd != nil; gotQuit != test.wantQuit {
				t.Errorf("quit = %v, want %v", gotQuit, test.wantQuit)
			}
		})
	}
}

func TestWatchModelInit(t *testing.T) {
	if cmd := (watchModel{}).Init(); cmd != nil {
		t.Error("Init returned a command, want nil")
	}
}

// TestResetTableKeepsColours checks the refresh does not silently drop the
// colour settings, which are read from flags once at startup.
func TestResetTableKeepsColours(t *testing.T) {
	builder := &RowBuilder{Table: &Table{
		ColourOutput:  COLOUR_MIX,
		CustomColours: [][2]int{{31, 0}, {32, 1}},
	}}
	builder.Table.SetHeader("NAME")
	builder.Table.AddRow(NewCellText("row"))

	builder.resetTable()

	if got := len(builder.Table.data); got != 0 {
		t.Errorf("the new table still holds %d rows", got)
	}
	if builder.Table.ColourOutput != COLOUR_MIX {
		t.Errorf("ColourOutput = %v, want it preserved", builder.Table.ColourOutput)
	}
	if len(builder.Table.CustomColours) != 2 {
		t.Errorf("CustomColours has %d entries, want 2", len(builder.Table.CustomColours))
	}
}

// TestWatchBuildRejectsFileAndStdin covers the two inputs watch mode cannot
// work with: there is nothing to watch behind a file or a pipe.
func TestWatchBuildRejectsFileAndStdin(t *testing.T) {
	builder := &RowBuilder{InputFilename: "pods.yaml", Table: &Table{}}

	err := builder.WatchBuild(&image{}, func() (string, error) { return "", nil })
	if err == nil {
		t.Fatal("want an error for --watch with --filename")
	}
	if got := err.Error(); got != "--watch cannot be used with --filename" {
		t.Errorf("error = %q", got)
	}
}
