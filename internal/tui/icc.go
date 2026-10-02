package tui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/icc"
)

const (
	iccField = 20
	// iccCustomValue is the picker entry that opens the path dialog, for a
	// profile kept somewhere the scan does not look.
	iccCustomValue = "\x00custom"
)

// openICCPicker lists the display profiles installed on the system by name,
// with None to clear the setting and Custom path… for anything else. The
// current profile is always in the list, even when the scan did not find it.
func (m *Model) openICCPicker() {
	output := m.editOutputs[m.selectedOutput]
	list := m.listICC
	if list == nil {
		list = func() []icc.Profile { return icc.List(icc.Dirs()) }
	}

	options, labels := []string{""}, []string{"None"}
	selected, currentListed := 0, output.ICC == ""
	for _, profile := range list() {
		options = append(options, profile.Path)
		labels = append(labels, fmt.Sprintf("%s  (%s)", profile.Name, filepath.Base(profile.Path)))
		if profile.Path == output.ICC {
			selected, currentListed = len(options)-1, true
		}
	}
	if !currentListed {
		options = append(options, output.ICC)
		labels = append(labels, output.ICC)
		selected = len(options) - 1
	}
	options = append(options, iccCustomValue)
	labels = append(labels, "Custom path…")
	m.openLabeledPicker(layoutFields[iccField], iccField, options, labels, selected)
}

// openICCEntry types an absolute path to a profile.
func (m *Model) openICCEntry() tea.Cmd {
	output := m.editOutputs[m.selectedOutput]
	m.inspectorField = iccField
	return m.openNumericInput(
		numericInputICC,
		m.selectedOutput,
		fmt.Sprintf("%s for %s", layoutFields[iccField], output.Name),
		"Absolute path to an ICC device profile. Leave empty to clear. Enter applies. Esc cancels.",
		output.ICC,
	)
}
