package tui

import (
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/prefs"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Without a daemon the editor owns the preview deadline, so it has to read
// the same saved preference the daemon would.
func TestDirectModePreviewUsesTheSavedPreviewTime(t *testing.T) {
	dir := t.TempDir()
	m := Model{styles: newStyles(), store: profile.NewStore(dir)}
	if got := m.previewTimeout(); got != 30*time.Second {
		t.Fatalf("default preview time = %v, want 30s", got)
	}

	if _, err := prefs.Save(dir, withPreviewTime(60)); err != nil {
		t.Fatal(err)
	}
	if got := m.previewTimeout(); got != 60*time.Second {
		t.Fatalf("preview time after saving 60 = %v", got)
	}

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	updated, _ := m.Update(applyMsg{profile: profile.Profile{Name: "desk"}})
	pending := mustModel(t, updated).pending
	if pending == nil || !pending.deadline.Equal(now.Add(60*time.Second)) || pending.total != 60*time.Second {
		t.Fatalf("pending = %+v, want a 60s deadline", pending)
	}

	if got := (Model{}).previewTimeout(); got != 30*time.Second {
		t.Fatalf("preview time without a store = %v", got)
	}
}

// withPreviewTime is the defaults with another preview time.
func withPreviewTime(seconds int) prefs.Preferences {
	p := prefs.Default()
	p.PreviewTimeoutSeconds = seconds
	return p
}
