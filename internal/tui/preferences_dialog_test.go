package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/prefs"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func preferencesTestModel(t *testing.T) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.store = profile.NewStore(dir)
	return m, dir
}

func openPreferences(t *testing.T, m Model) Model {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Fatal("p did not start loading preferences")
	}
	opened := mustModel(t, runModelUpdate(t, m, cmd()))
	if opened.mode != modePreferences || opened.prefsDialog == nil {
		t.Fatalf("preferences dialog did not open: mode %v", opened.mode)
	}
	return opened
}

func TestPreferencesDialogShowsEverySettingAtSmallSizes(t *testing.T) {
	m, _ := preferencesTestModel(t)
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 32}} {
		m.width, m.height = size.width, size.height
		view := ansi.Strip(openPreferences(t, m).View())
		requireContains(t, view,
			"Preferences", "Preview time", "15s", "30s", "60s", "120s",
			"New display side", "Right", "Left", "Above", "Below",
			"New display alignment", "Center", "Edge",
			"New display VRR", "Fullscreen", "New setup notification",
			"Enter saves. Esc discards.")
		for _, line := range strings.Split(view, "\n") {
			if len([]rune(line)) > size.width {
				t.Fatalf("%dx%d: a line is wider than the terminal: %q", size.width, size.height, line)
			}
		}
	}
}

func TestPreferencesDialogSavesWhatWasChosen(t *testing.T) {
	m, dir := preferencesTestModel(t)
	dialog := openPreferences(t, m)

	// Preview time: 30s -> 60s. Then down to the side row: Right -> Left.
	dialog = mustModel(t, runModelUpdate(t, dialog, tea.KeyMsg{Type: tea.KeyRight}))
	dialog = mustModel(t, runModelUpdate(t, dialog, tea.KeyMsg{Type: tea.KeyDown}))
	dialog = mustModel(t, runModelUpdate(t, dialog, tea.KeyMsg{Type: tea.KeyRight}))
	if saved, _ := prefs.Load(dir); saved != prefs.Default() {
		t.Fatalf("choosing an option wrote preferences before Enter: %+v", saved)
	}

	_, cmd := dialog.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter did not save")
	}
	done := mustModel(t, runModelUpdate(t, dialog, cmd()))
	if done.mode != modeMain || done.prefsDialog != nil || done.statusErr {
		t.Fatalf("dialog did not close cleanly: mode %v, status %q", done.mode, done.status)
	}
	saved, err := prefs.Load(dir)
	if err != nil || saved.PreviewTimeoutSeconds != 60 || saved.NewDisplaySide != profile.NewDisplayLeft {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if !saved.NotifyNewSetup || saved.NewDisplayAlignment != profile.NewDisplayCenter {
		t.Fatalf("untouched settings changed: %+v", saved)
	}
}

func TestPreferencesDialogEscDiscardsAndArrowsStopAtTheEnds(t *testing.T) {
	m, dir := preferencesTestModel(t)
	dialog := openPreferences(t, m)

	for i := 0; i < 6; i++ {
		dialog = mustModel(t, runModelUpdate(t, dialog, tea.KeyMsg{Type: tea.KeyRight}))
	}
	if dialog.prefsDialog.prefs.PreviewTimeoutSeconds != 120 {
		t.Fatalf("arrows past the end gave %d, want 120", dialog.prefsDialog.prefs.PreviewTimeoutSeconds)
	}

	closed := mustModel(t, runModelUpdate(t, dialog, tea.KeyMsg{Type: tea.KeyEsc}))
	if closed.mode != modeMain || closed.prefsDialog != nil {
		t.Fatal("Esc did not close the dialog")
	}
	if saved, _ := prefs.Load(dir); saved != prefs.Default() {
		t.Fatalf("Esc saved the changes: %+v", saved)
	}
}

func TestPreferencesDialogOptionsCanBeClicked(t *testing.T) {
	m, _ := preferencesTestModel(t)
	dialog := openPreferences(t, m)

	click := func(d Model, rowLabel, option string) Model {
		t.Helper()
		for y, line := range strings.Split(ansi.Strip(d.View()), "\n") {
			labelAt := strings.Index(line, rowLabel)
			if labelAt < 0 {
				continue
			}
			at := strings.Index(line[labelAt:], " "+option+" ")
			if at < 0 {
				t.Fatalf("option %q not on the %q row: %q", option, rowLabel, line)
			}
			x := len([]rune(line[:labelAt+at])) + 1
			return mustModel(t, runModelUpdate(t, d, tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}))
		}
		t.Fatalf("row %q not found", rowLabel)
		return d
	}

	dialog = click(dialog, "New display side", "Below")
	dialog = click(dialog, "New setup notification", "Off")
	// "Off" also appears on the VRR row; the notification click must not touch it.
	got := dialog.prefsDialog.prefs
	if got.NewDisplaySide != profile.NewDisplayBelow || got.NotifyNewSetup || got.NewDisplayVRR != 0 {
		t.Fatalf("after clicks: %+v", got)
	}
	dialog = click(dialog, "New display VRR", "Fullscreen")
	if dialog.prefsDialog.prefs.NewDisplayVRR != 2 {
		t.Fatalf("VRR click gave %d", dialog.prefsDialog.prefs.NewDisplayVRR)
	}
}

func TestPreferencesOpenFromTheHeaderWhenThereIsRoom(t *testing.T) {
	m, _ := preferencesTestModel(t)
	m.width = 140
	header := strings.Split(ansi.Strip(m.View()), "\n")[m.appContentY()]
	at := strings.Index(header, preferencesLabel)
	if at < 0 {
		t.Fatalf("no Preferences control in a wide header: %q", header)
	}
	x := len([]rune(header[:at])) + 2
	_, cmd := m.Update(tea.MouseMsg{X: x, Y: m.appContentY(), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if cmd == nil {
		t.Fatal("clicking Preferences did nothing")
	}
	if _, ok := cmd().(preferencesLoadedMsg); !ok {
		t.Fatal("clicking Preferences did not load preferences")
	}
}
