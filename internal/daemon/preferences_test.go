package daemon

import (
	"os"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/prefs"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func preferencesTestService(t *testing.T) (*Service, string, *logRecorder) {
	t.Helper()
	dir := t.TempDir()
	logs := &logRecorder{}
	return New(nil, profile.NewStore(dir), Config{ConfigDir: dir, Logf: logs.logf}), dir, logs
}

func TestPreviewUsesTheSavedPreviewTimeUnlessTheClientSendsOne(t *testing.T) {
	svc, _, _ := preferencesTestService(t)

	if got := svc.previewTimeout(ipc.PreviewParams{}); got != 30*time.Second {
		t.Fatalf("default preview time = %v, want 30s", got)
	}
	if _, err := svc.SetPreferences(prefs.Preferences{PreviewTimeoutSeconds: 120}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}
	if got := svc.previewTimeout(ipc.PreviewParams{}); got != 120*time.Second {
		t.Fatalf("preview time after saving 120 = %v", got)
	}
	// An older client that still sends its own duration keeps it.
	if got := svc.previewTimeout(ipc.PreviewParams{TimeoutSeconds: 10}); got != 10*time.Second {
		t.Fatalf("explicit preview time = %v, want 10s", got)
	}
	if got := svc.previewTimeout(ipc.PreviewParams{TimeoutSeconds: 7 * 24 * 3600}); got != 24*time.Hour {
		t.Fatalf("an explicit week was not capped: %v", got)
	}
}

func TestSetPreferencesValidatesAndNotifies(t *testing.T) {
	svc, dir, _ := preferencesTestService(t)
	notified := 0
	svc.notify = func() { notified++ }

	if _, err := svc.SetPreferences(prefs.Preferences{PreviewTimeoutSeconds: 45}); err == nil {
		t.Fatal("a preview time that is not offered was saved")
	}
	if _, err := os.Stat(prefs.Path(dir)); !os.IsNotExist(err) {
		t.Fatal("a refused change wrote the preferences file")
	}
	saved, err := svc.SetPreferences(prefs.Preferences{PreviewTimeoutSeconds: 60})
	if err != nil || saved.PreviewTimeoutSeconds != 60 || saved.Version != prefs.Version {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if notified != 1 {
		t.Fatalf("status notifications = %d, want 1", notified)
	}
	if read, _ := svc.Preferences(); read != saved {
		t.Fatalf("read back %+v, want %+v", read, saved)
	}
}

func TestADamagedPreferencesFileStillLeavesPreviewsADeadline(t *testing.T) {
	svc, dir, logs := preferencesTestService(t)
	if err := os.WriteFile(prefs.Path(dir), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := svc.previewTimeout(ipc.PreviewParams{}); got != 30*time.Second {
		t.Fatalf("preview time with a damaged file = %v, want the 30s default", got)
	}
	if !logs.contains("using defaults") {
		t.Fatalf("the problem was not logged: %q", logs.all())
	}
}
