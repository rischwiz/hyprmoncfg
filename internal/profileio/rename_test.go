package profileio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

func saveTestProfile(t *testing.T, store *profile.Store, name string, exec string) profile.Profile {
	t.Helper()
	p := profile.New(name, []profile.OutputConfig{{
		Key: "acme|panel|1", Name: "DP-1", Make: "Acme", Model: "Panel", Serial: "1",
		Enabled: true, Width: 1920, Height: 1080, Refresh: 60, Scale: 1,
	}})
	p.Exec = exec
	if err := SaveWithSidecars(store, p); err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
	saved, err := store.Load(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return saved
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestRenameMovesProfileAndSidecars(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	original := saveTestProfile(t, store, "desk", "notify-send hi")

	if err := Rename(store, "desk", "Home Office"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	renamed, err := store.Load("Home Office")
	if err != nil {
		t.Fatalf("load renamed: %v", err)
	}
	if renamed.Name != "Home Office" || renamed.Exec != "notify-send hi" {
		t.Fatalf("renamed = %q exec %q", renamed.Name, renamed.Exec)
	}
	if !renamed.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("created at changed: %v, want %v", renamed.CreatedAt, original.CreatedAt)
	}
	oldPaths, newPaths := store.PathsForName("desk"), store.PathsForName("Home Office")
	for _, path := range []string{oldPaths.JSON, oldPaths.Conf, oldPaths.Lua} {
		if exists(path) {
			t.Fatalf("old file remains: %s", path)
		}
	}
	for _, path := range []string{newPaths.JSON, newPaths.Conf, newPaths.Lua} {
		if !exists(path) {
			t.Fatalf("new file missing: %s", path)
		}
	}
}

func TestRenameRefusesATakenNameAndKeepsBoth(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	saveTestProfile(t, store, "desk", "")
	saveTestProfile(t, store, "Travel", "")

	// "travel!" is a different name stored in the same file as "Travel".
	for _, taken := range []string{"Travel", "travel!"} {
		err := Rename(store, "desk", taken)
		if !errors.Is(err, ErrProfileExists) {
			t.Fatalf("rename to %q: %v, want ErrProfileExists", taken, err)
		}
	}
	for _, name := range []string{"desk", "Travel"} {
		if p, err := store.Load(name); err != nil || p.Name != name {
			t.Fatalf("%s after refused rename: %q, %v", name, p.Name, err)
		}
	}
}

func TestRenameWithinTheSameFileChangesOnlyTheName(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	saveTestProfile(t, store, "desk", "")

	if err := Rename(store, "desk", "Desk"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	p, err := store.Load("desk")
	if err != nil || p.Name != "Desk" {
		t.Fatalf("profile = %q, %v", p.Name, err)
	}
}

func TestRenameMissingProfileFails(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	if err := Rename(store, "absent", "new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rename missing: %v, want not exist", err)
	}
}

func TestRenameLeavesASymlinkedProfileAlone(t *testing.T) {
	dir := t.TempDir()
	store := profile.NewStore(dir)
	saveTestProfile(t, store, "desk", "")
	paths := store.PathsForName("desk")
	source := filepath.Join(dir, "dotfiles-desk.json")
	if err := os.Rename(paths.JSON, source); err != nil {
		t.Fatalf("move profile: %v", err)
	}
	if err := os.Symlink(source, paths.JSON); err != nil {
		t.Fatalf("symlink profile: %v", err)
	}

	if err := Rename(store, "desk", "office"); err == nil {
		t.Fatal("rename replaced a symlinked profile")
	}
	if info, err := os.Lstat(paths.JSON); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink is gone: %v", err)
	}
	if exists(store.PathsForName("office").JSON) {
		t.Fatal("refused rename still wrote the new profile")
	}
}

func TestDuplicateCopiesLayoutButNotTheCommandUnlessAsked(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	saveTestProfile(t, store, "desk", "notify-send hi")

	if err := Duplicate(store, "desk", "desk copy", false); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if err := Duplicate(store, "desk", "desk with command", true); err != nil {
		t.Fatalf("duplicate with command: %v", err)
	}

	plain, err := store.Load("desk copy")
	if err != nil {
		t.Fatalf("load copy: %v", err)
	}
	if plain.Exec != "" || len(plain.Outputs) != 1 || plain.Outputs[0].Width != 1920 {
		t.Fatalf("copy = exec %q outputs %+v", plain.Exec, plain.Outputs)
	}
	withCommand, _ := store.Load("desk with command")
	if withCommand.Exec != "notify-send hi" {
		t.Fatalf("explicit copy lost the command: %q", withCommand.Exec)
	}
	if source, err := store.Load("desk"); err != nil || source.Exec != "notify-send hi" {
		t.Fatalf("source changed: %+v, %v", source, err)
	}
	if !exists(store.PathsForName("desk copy").Lua) {
		t.Fatal("copy has no sidecars")
	}
}

func TestDuplicateRefusesATakenName(t *testing.T) {
	store := profile.NewStore(t.TempDir())
	saveTestProfile(t, store, "desk", "")
	saveTestProfile(t, store, "travel", "")

	for _, taken := range []string{"travel", "desk", "Desk"} {
		if err := Duplicate(store, "desk", taken, false); !errors.Is(err, ErrProfileExists) {
			t.Fatalf("duplicate to %q: %v, want ErrProfileExists", taken, err)
		}
	}
}
