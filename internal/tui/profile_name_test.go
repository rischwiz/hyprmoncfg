package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/profileio"
)

func profileNameTestModel(t *testing.T) (Model, *profile.Store) {
	t.Helper()
	store := profile.NewStore(t.TempDir())
	desk := profile.FromState("Desk", []hypr.Monitor{paneTestDesk}, nil)
	desk.Exec = "notify-send hi"
	travel := profile.FromState("Travel", []hypr.Monitor{paneTestLaptop}, nil)
	for _, p := range []profile.Profile{desk, travel} {
		if err := profileio.SaveWithSidecars(store, p); err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	m := paneTestModel(t, tabProfiles, []hypr.Monitor{paneTestDesk}, profiles)
	m.store = store
	m.selectedProfile = m.profileIndexByName("Desk")
	return m, store
}

func typeProfileName(t *testing.T, model tea.Model, name string) Model {
	t.Helper()
	m := mustModel(t, model)
	if m.nameInput == nil {
		t.Fatal("no name dialog is open")
	}
	m.nameInput.Input.SetValue(name)
	return m
}

func TestProfilesTabOffersRenameAndDuplicate(t *testing.T) {
	m, _ := profileNameTestModel(t)
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 32}} {
		m.width, m.height = size.width, size.height
		view := ansi.Strip(m.View())
		for _, label := range []string{"[Preview]", "[Edit]", "[Delete]", "[Rename]", "[Duplicate]"} {
			if !strings.Contains(view, label) {
				t.Fatalf("%dx%d: missing %s in:\n%s", size.width, size.height, label, view)
			}
		}
		// Shortcuts belong in the footer and help, never inside action labels.
		for _, hinted := range []string{"[n Rename]", "[Rename n]", "[c Duplicate]", "[Duplicate c]"} {
			if strings.Contains(view, hinted) {
				t.Fatalf("action label carries a key hint: %s", hinted)
			}
		}
	}
}

func TestRenameProfileFromTheProfilesTab(t *testing.T) {
	m, store := profileNameTestModel(t)

	opened := runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	dialog := mustModel(t, opened)
	if dialog.mode != modeProfileNameInput || dialog.nameInput.Input.Value() != "Desk" {
		t.Fatalf("rename dialog not open with the current name: mode %v", dialog.mode)
	}
	requireContains(t, ansi.Strip(dialog.View()), "Rename Desk", "New name", "Nothing is applied")

	typed := typeProfileName(t, opened, "Office")
	_, cmd := typed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming the name did nothing")
	}
	done := mustModel(t, runModelUpdate(t, typed, cmd()))

	if done.statusErr || !strings.Contains(done.status, `Renamed profile "Desk" to "Office"`) {
		t.Fatalf("status = %q (error %v)", done.status, done.statusErr)
	}
	renamed, err := store.Load("Office")
	if err != nil {
		t.Fatalf("renamed profile missing: %v", err)
	}
	if renamed.Exec != "notify-send hi" {
		t.Fatalf("rename lost the post-apply command: %q", renamed.Exec)
	}
	if _, err := store.Load("Desk"); err == nil {
		t.Fatal("old profile still exists")
	}
}

func TestDuplicateProfileDropsThePostApplyCommand(t *testing.T) {
	m, store := profileNameTestModel(t)

	opened := runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	dialog := mustModel(t, opened)
	if dialog.mode != modeProfileNameInput || dialog.nameInput.Input.Value() != "" {
		t.Fatalf("duplicate dialog should open empty: %q", dialog.nameInput.Input.Value())
	}
	requireContains(t, ansi.Strip(dialog.View()), "Duplicate Desk", "without its post-apply command")

	typed := typeProfileName(t, opened, "Desk gaming")
	_, cmd := typed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirming the name did nothing")
	}
	done := mustModel(t, runModelUpdate(t, typed, cmd()))
	if done.statusErr {
		t.Fatalf("duplicate failed: %s", done.status)
	}

	copied, err := store.Load("Desk gaming")
	if err != nil {
		t.Fatalf("copy missing: %v", err)
	}
	if copied.Exec != "" {
		t.Fatalf("copy kept the post-apply command %q", copied.Exec)
	}
	if source, err := store.Load("Desk"); err != nil || source.Exec != "notify-send hi" {
		t.Fatalf("source changed: %v", err)
	}
}

func TestProfileNameDialogRefusesATakenNameAndStaysOpen(t *testing.T) {
	m, store := profileNameTestModel(t)

	opened := runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	typed := typeProfileName(t, opened, "travel")
	updated, cmd := typed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	refused := mustModel(t, updated)

	if cmd != nil || refused.mode != modeProfileNameInput || refused.nameInput.Err == nil {
		t.Fatalf("taken name was accepted: mode %v", refused.mode)
	}
	requireContains(t, ansi.Strip(refused.View()), `A profile named "Travel" already exists.`)
	if _, err := store.Load("Desk"); err != nil {
		t.Fatalf("profile changed after a refused rename: %v", err)
	}

	closed := mustModel(t, runModelUpdate(t, refused, tea.KeyMsg{Type: tea.KeyEsc}))
	if closed.mode != modeMain || closed.nameInput != nil {
		t.Fatal("Esc did not close the dialog")
	}
}
