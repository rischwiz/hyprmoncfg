package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/icc"
)

func iccTestModel(t *testing.T, profiles ...icc.Profile) Model {
	t.Helper()
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.layoutFocus = layoutFocusInspector
	m.inspectorTab = inspectorTabColor
	m.inspectorField = iccField
	m.listICC = func() []icc.Profile { return profiles }
	return m
}

var (
	iccDesk  = icc.Profile{Path: "/usr/share/color/icc/desk.icc", Name: "Desk Calibrated"}
	iccPanel = icc.Profile{Path: "/home/example/.local/share/icc/panel.icc", Name: "Acme Panel P3"}
)

func pickerLabels(m Model) []string {
	labels := make([]string, 0, len(m.picker.List.Items()))
	for _, item := range m.picker.List.Items() {
		labels = append(labels, item.(fieldPickerItem).label)
	}
	return labels
}

func TestICCRowOpensAPickerOfInstalledProfiles(t *testing.T) {
	m := iccTestModel(t, iccDesk, iccPanel)
	opened := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	if opened.mode != modeModePicker || opened.picker == nil {
		t.Fatalf("the ICC row did not open a picker: mode %v", opened.mode)
	}

	got := strings.Join(pickerLabels(opened), "|")
	want := "None|Desk Calibrated  (desk.icc)|Acme Panel P3  (panel.icc)|Custom path…"
	if got != want {
		t.Fatalf("picker = %q, want %q", got, want)
	}
	requireContains(t, ansi.Strip(opened.View()), "Pick a display profile for DP-1", "Desk Calibrated")
}

func TestPickingAProfileSetsItsAbsolutePath(t *testing.T) {
	m := iccTestModel(t, iccDesk, iccPanel)
	opened := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	opened.picker.List.Select(2)

	picked := mustModel(t, runModelUpdate(t, opened, tea.KeyMsg{Type: tea.KeyEnter}))
	if picked.editOutputs[0].ICC != iccPanel.Path || !picked.dirty || picked.mode != modeMain {
		t.Fatalf("ICC = %q, dirty %v, mode %v", picked.editOutputs[0].ICC, picked.dirty, picked.mode)
	}

	// None clears it again, and the status names it instead of a blank.
	reopened := mustModel(t, runModelUpdate(t, picked, tea.KeyMsg{Type: tea.KeyEnter}))
	if reopened.picker.List.Index() != 2 {
		t.Fatalf("the current profile is not preselected: index %d", reopened.picker.List.Index())
	}
	reopened.picker.List.Select(0)
	cleared := mustModel(t, runModelUpdate(t, reopened, tea.KeyMsg{Type: tea.KeyEnter}))
	if cleared.editOutputs[0].ICC != "" || !strings.Contains(cleared.status, "to None for DP-1") {
		t.Fatalf("ICC = %q, status %q", cleared.editOutputs[0].ICC, cleared.status)
	}
}

func TestAProfileOutsideTheScanStaysInThePicker(t *testing.T) {
	m := iccTestModel(t, iccDesk)
	m.editOutputs[0].ICC = "/opt/calibration/custom.icc"

	opened := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	labels := pickerLabels(opened)
	if got := strings.Join(labels, "|"); got != "None|Desk Calibrated  (desk.icc)|/opt/calibration/custom.icc|Custom path…" {
		t.Fatalf("picker = %q", got)
	}
	if opened.picker.List.Index() != 2 {
		t.Fatalf("the saved profile is not preselected: index %d", opened.picker.List.Index())
	}

	// Choosing it again keeps the path exactly as saved.
	kept := mustModel(t, runModelUpdate(t, opened, tea.KeyMsg{Type: tea.KeyEnter}))
	if kept.editOutputs[0].ICC != "/opt/calibration/custom.icc" {
		t.Fatalf("the saved path changed to %q", kept.editOutputs[0].ICC)
	}
}

func TestCustomPathOpensThePathDialog(t *testing.T) {
	m := iccTestModel(t) // nothing installed: None and Custom path… only
	m.editOutputs[0].ICC = ""
	opened := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	if got := strings.Join(pickerLabels(opened), "|"); got != "None|Custom path…" {
		t.Fatalf("picker with nothing installed = %q", got)
	}

	opened.picker.List.Select(1)
	dialog := mustModel(t, runModelUpdate(t, opened, tea.KeyMsg{Type: tea.KeyEnter}))
	if dialog.mode != modeNumericInput || dialog.input == nil || dialog.input.Kind != numericInputICC {
		t.Fatalf("Custom path… did not open the path dialog: mode %v", dialog.mode)
	}
	if dialog.editOutputs[0].ICC != "" || dialog.dirty {
		t.Fatal("opening the path dialog changed the draft")
	}
}
