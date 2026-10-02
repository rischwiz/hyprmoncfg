package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func openProfileMenuWithKey(t *testing.T, m Model) Model {
	t.Helper()
	opened := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")}))
	if opened.mode != modeProfileMenu || opened.profileMenu == nil {
		t.Fatalf("action menu did not open: mode %v", opened.mode)
	}
	return opened
}

func TestProfileMenuListsEveryActionWithoutKeyHints(t *testing.T) {
	m, _ := profileNameTestModel(t)
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 32}} {
		m.width, m.height = size.width, size.height
		view := ansi.Strip(openProfileMenuWithKey(t, m).View())
		requireContains(t, view, "Actions for Desk", "Preview", "Edit layout", "Edit post-apply command",
			"Rename", "Duplicate", "Delete", "Enter runs the action. Esc closes.")
		for _, line := range strings.Split(view, "\n") {
			for _, action := range profileMenuActions {
				if strings.Contains(line, action.label+" (") || strings.Contains(line, "["+action.label) {
					t.Fatalf("an action label carries a key hint: %q", line)
				}
			}
		}
	}
}

func TestProfileMenuRunsTheSameActionAsTheKey(t *testing.T) {
	m, _ := profileNameTestModel(t)
	menu := openProfileMenuWithKey(t, m)

	// Down to Rename (Preview, Edit layout, Edit post-apply command, Rename).
	for i := 0; i < 3; i++ {
		menu = mustModel(t, runModelUpdate(t, menu, tea.KeyMsg{Type: tea.KeyDown}))
	}
	renaming := mustModel(t, runModelUpdate(t, menu, tea.KeyMsg{Type: tea.KeyEnter}))
	if renaming.mode != modeProfileNameInput || renaming.nameInput.Source != "Desk" || renaming.nameInput.Duplicate {
		t.Fatalf("Rename from the menu did not open the rename dialog: mode %v", renaming.mode)
	}
	if renaming.profileMenu != nil {
		t.Fatal("the menu stayed open behind the dialog")
	}

	// Delete is the last entry, one step up from the first since the list
	// wraps like the profile list does, and it still asks first.
	menu = openProfileMenuWithKey(t, m)
	menu = mustModel(t, runModelUpdate(t, menu, tea.KeyMsg{Type: tea.KeyUp}))
	deleting := mustModel(t, runModelUpdate(t, menu, tea.KeyMsg{Type: tea.KeyEnter}))
	if deleting.mode != modeDeleteConfirm || deleting.deleteProfileName != "Desk" {
		t.Fatalf("Delete from the menu skipped the confirmation: mode %v", deleting.mode)
	}
}

func TestProfileMenuEscClosesWithoutActing(t *testing.T) {
	m, store := profileNameTestModel(t)
	menu := openProfileMenuWithKey(t, m)

	updated, cmd := menu.Update(tea.KeyMsg{Type: tea.KeyEsc})
	closed := mustModel(t, updated)
	if cmd != nil || closed.mode != modeMain || closed.profileMenu != nil {
		t.Fatalf("Esc did not just close the menu: mode %v", closed.mode)
	}
	if profiles, _ := store.List(); len(profiles) != 2 {
		t.Fatalf("closing the menu changed the profiles: %d", len(profiles))
	}
}

func TestProfileMenuEntriesCanBeClicked(t *testing.T) {
	m, _ := profileNameTestModel(t)
	menu := openProfileMenuWithKey(t, m)

	clickLabel := func(label string) Model {
		t.Helper()
		for y, line := range strings.Split(ansi.Strip(menu.View()), "\n") {
			at := strings.Index(line, "  "+label+" ")
			if at < 0 {
				at = strings.Index(line, "▸ "+label+" ")
			}
			if at >= 0 {
				x := len([]rune(line[:at])) + 3
				return mustModel(t, runModelUpdate(t, menu, tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}))
			}
		}
		t.Fatalf("menu entry %q not found", label)
		return menu
	}

	if got := clickLabel("Edit post-apply command"); got.mode != modeProfileExecInput {
		t.Fatalf("clicking Edit post-apply command gave mode %v", got.mode)
	}
	// "Edit layout" shares a word with the entry above and must not be confused with it.
	if got := clickLabel("Edit layout"); got.mode != modeMain || got.tab != tabLayout {
		t.Fatalf("clicking Edit layout gave mode %v on tab %v", got.mode, got.tab)
	}
	if got := clickLabel("Duplicate"); got.mode != modeProfileNameInput || !got.nameInput.Duplicate {
		t.Fatalf("clicking Duplicate gave mode %v", got.mode)
	}
}

func TestRightClickOnAProfileOpensItsMenu(t *testing.T) {
	m, _ := profileNameTestModel(t)
	m.selectedProfile = m.profileIndexByName("Desk")

	for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		at := strings.Index(line, "Travel")
		if at < 0 {
			continue
		}
		x := len([]rune(line[:at])) + 1
		if _, ok := m.profileRowAt(x, y); !ok {
			continue
		}
		opened := mustModel(t, runModelUpdate(t, m, tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonRight}))
		if opened.mode != modeProfileMenu || opened.profileMenu == nil || opened.profileMenu.profile != "Travel" {
			t.Fatalf("right click did not open Travel's menu: mode %v", opened.mode)
		}
		return
	}
	t.Fatal("no Travel row found in the profile list")
}
