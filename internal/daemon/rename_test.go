package daemon

import (
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/profileio"
)

func renameTestService(t *testing.T) (*Service, *profile.Store) {
	t.Helper()
	store := profile.NewStore(t.TempDir())
	p := profile.New("desk", []profile.OutputConfig{{
		Key: "acme|panel|1", Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, Refresh: 60, Scale: 1,
	}})
	p.Exec = "notify-send hi"
	if err := profileio.SaveWithSidecars(store, p); err != nil {
		t.Fatal(err)
	}
	return New(nil, store, Config{}), store
}

func TestRenameKeepsRememberedChoicesPointingAtTheProfile(t *testing.T) {
	svc, store := renameTestService(t)
	svc.manualSet = "set"
	svc.manualProfile = profile.Profile{Name: "desk"}
	svc.lastProfile = profile.Profile{Name: "desk"}
	notified := 0
	svc.notify = func() { notified++ }

	if err := svc.Rename(ipc.RenameParams{Name: "desk", NewName: "office"}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if svc.manualProfile.Name != "office" || svc.lastProfile.Name != "office" {
		t.Fatalf("manual = %q, last = %q, want office", svc.manualProfile.Name, svc.lastProfile.Name)
	}
	if _, err := store.Load("office"); err != nil {
		t.Fatalf("renamed profile missing: %v", err)
	}
	if notified != 1 {
		t.Fatalf("status notifications = %d, want 1", notified)
	}
}

func TestRenameWaitsForAPreviewOfThatProfile(t *testing.T) {
	svc, store := renameTestService(t)
	svc.pending = &pendingTransaction{profile: profile.Profile{Name: "desk"}}

	err := svc.Rename(ipc.RenameParams{Name: "desk", NewName: "office"})
	if err == nil || !strings.Contains(err.Error(), "being previewed") {
		t.Fatalf("rename during preview: %v", err)
	}
	if _, err := store.Load("desk"); err != nil {
		t.Fatalf("profile changed during preview: %v", err)
	}
}

func TestDuplicateLeavesTheSourceAndDropsTheCommand(t *testing.T) {
	svc, store := renameTestService(t)

	if err := svc.Duplicate(ipc.DuplicateParams{Name: "desk", NewName: "desk copy"}); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	copied, err := store.Load("desk copy")
	if err != nil {
		t.Fatalf("copy missing: %v", err)
	}
	if copied.Exec != "" {
		t.Fatalf("copy kept the post-apply command %q", copied.Exec)
	}
	if source, err := store.Load("desk"); err != nil || source.Exec != "notify-send hi" {
		t.Fatalf("source changed: %+v, %v", source, err)
	}
}
