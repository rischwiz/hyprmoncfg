package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func undoTestModel(t *testing.T) (Model, *time.Time) {
	t.Helper()
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk, paneTestSide}, nil)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	m.selectedOutput = outputIndexNamed(t, m, "DP-2")
	return m, &now
}

func pressKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+r":
		msg = tea.KeyMsg{Type: tea.KeyCtrlR}
	}
	return mustModel(t, runModelUpdate(t, m, msg))
}

func TestUndoRestoresTheDraftAndRedoBringsTheEditBack(t *testing.T) {
	m, _ := undoTestModel(t)
	selected := m.selectedOutput
	startX := m.editOutputs[selected].X

	moved := pressKey(t, m, "right")
	movedX := moved.editOutputs[selected].X
	if movedX == startX || !moved.dirty {
		t.Fatalf("arrow did not move the display: x %d, dirty %v", movedX, moved.dirty)
	}

	undone := pressKey(t, moved, "u")
	if undone.editOutputs[selected].X != startX {
		t.Fatalf("undo left x at %d, want %d", undone.editOutputs[selected].X, startX)
	}
	if undone.dirty {
		t.Fatal("undoing the only edit should leave the draft clean")
	}

	redone := pressKey(t, undone, "ctrl+r")
	if redone.editOutputs[selected].X != movedX || !redone.dirty {
		t.Fatalf("redo x = %d dirty %v, want %d and dirty", redone.editOutputs[selected].X, redone.dirty, movedX)
	}
	if again := pressKey(t, redone, "ctrl+r"); again.editOutputs[selected].X != movedX || !again.statusErr {
		t.Fatal("redo with nothing left should change nothing and say so")
	}
}

func TestUndoTreatsAQuickRunOfTheSameKeyAsOneStep(t *testing.T) {
	m, now := undoTestModel(t)
	selected := m.selectedOutput
	startX := m.editOutputs[selected].X

	for i := 0; i < 3; i++ {
		*now = now.Add(200 * time.Millisecond)
		m = pressKey(t, m, "right")
	}
	if len(m.history.undo) != 1 {
		t.Fatalf("a run of nudges made %d undo steps, want 1", len(m.history.undo))
	}

	// A pause, or a different key, starts a new step.
	*now = now.Add(5 * time.Second)
	m = pressKey(t, m, "right")
	afterPauseX := m.editOutputs[selected].X
	m = pressKey(t, m, "down")
	if len(m.history.undo) != 3 {
		t.Fatalf("undo steps = %d, want 3", len(m.history.undo))
	}

	m = pressKey(t, m, "u")
	if m.editOutputs[selected].X != afterPauseX {
		t.Fatalf("first undo x = %d, want %d", m.editOutputs[selected].X, afterPauseX)
	}
	m = pressKey(t, pressKey(t, m, "u"), "u")
	if m.editOutputs[selected].X != startX {
		t.Fatalf("undoing every step left x at %d, want %d", m.editOutputs[selected].X, startX)
	}
	if after := pressKey(t, m, "u"); !after.statusErr {
		t.Fatal("undo with nothing left should say so")
	}
}

func TestUndoCoversThePolicyToggleAndANewEditDropsRedo(t *testing.T) {
	m, _ := undoTestModel(t)

	toggled := pressKey(t, m, "U")
	if !toggled.disableUnknownOutputs {
		t.Fatal("U did not toggle the policy")
	}
	undone := pressKey(t, toggled, "u")
	if undone.disableUnknownOutputs {
		t.Fatal("undo did not restore the policy")
	}
	if len(undone.history.redo) != 1 {
		t.Fatalf("redo steps = %d, want 1", len(undone.history.redo))
	}
	if edited := pressKey(t, undone, "right"); len(edited.history.redo) != 0 {
		t.Fatal("a new edit should drop the redo history")
	}
}

func TestUndoHistoryDoesNotSurviveLoadingAnotherDraft(t *testing.T) {
	m, _ := undoTestModel(t)
	m = pressKey(t, m, "right")
	if len(m.history.undo) != 1 {
		t.Fatalf("undo steps = %d, want 1", len(m.history.undo))
	}

	m.loadProfile(profile.FromState("Travel", []hypr.Monitor{paneTestLaptop}, nil))
	if len(m.history.undo) != 0 {
		t.Fatal("loading a profile kept the previous draft's history")
	}
	loaded := len(m.editOutputs)
	if after := pressKey(t, m, "u"); len(after.editOutputs) != loaded || !after.statusErr {
		t.Fatal("undo after loading a profile changed the draft")
	}
}

func TestMovingTheSelectionIsNotAnUndoStep(t *testing.T) {
	m, _ := undoTestModel(t)
	if m = pressKey(t, m, "]"); len(m.history.undo) != 0 {
		t.Fatal("selecting another display was recorded as an edit")
	}
	if m = pressKey(t, m, "2"); len(m.history.undo) != 0 {
		t.Fatal("switching tabs was recorded as an edit")
	}
}
