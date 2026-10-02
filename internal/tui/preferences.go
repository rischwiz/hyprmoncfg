package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/prefs"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// preferencesLabel is the header control that opens the dialog.
const preferencesLabel = "Preferences"

// preferencesDialogState is the small dialog for settings that belong to the
// person, not to a profile. It edits a copy; Enter saves it, Esc discards it.
type preferencesDialogState struct {
	prefs prefs.Preferences
	row   int
	err   error
}

type preferencesLoadedMsg struct {
	prefs prefs.Preferences
	err   error
}

type preferencesSavedMsg struct {
	err error
}

type preferenceOption struct {
	label    string
	selected func(prefs.Preferences) bool
	choose   func(*prefs.Preferences)
}

type preferenceRow struct {
	label   string
	options []preferenceOption
}

func preferenceRows() []preferenceRow {
	previewTimes := make([]preferenceOption, 0, len(prefs.PreviewTimeoutChoices))
	for _, seconds := range prefs.PreviewTimeoutChoices {
		previewTimes = append(previewTimes, preferenceOption{
			label:    fmt.Sprintf("%ds", seconds),
			selected: func(p prefs.Preferences) bool { return p.PreviewTimeoutSeconds == seconds },
			choose:   func(p *prefs.Preferences) { p.PreviewTimeoutSeconds = seconds },
		})
	}
	side := func(label string, value profile.NewDisplaySide) preferenceOption {
		return preferenceOption{
			label:    label,
			selected: func(p prefs.Preferences) bool { return p.NewDisplaySide == value },
			choose:   func(p *prefs.Preferences) { p.NewDisplaySide = value },
		}
	}
	alignment := func(label string, value profile.NewDisplayAlignment) preferenceOption {
		return preferenceOption{
			label:    label,
			selected: func(p prefs.Preferences) bool { return p.NewDisplayAlignment == value },
			choose:   func(p *prefs.Preferences) { p.NewDisplayAlignment = value },
		}
	}
	vrr := func(label string, value int) preferenceOption {
		return preferenceOption{
			label:    label,
			selected: func(p prefs.Preferences) bool { return p.NewDisplayVRR == value },
			choose:   func(p *prefs.Preferences) { p.NewDisplayVRR = value },
		}
	}
	notify := func(label string, value bool) preferenceOption {
		return preferenceOption{
			label:    label,
			selected: func(p prefs.Preferences) bool { return p.NotifyNewSetup == value },
			choose:   func(p *prefs.Preferences) { p.NotifyNewSetup = value },
		}
	}
	return []preferenceRow{
		{"Preview time", previewTimes},
		{"New display side", []preferenceOption{
			side("Right", profile.NewDisplayRight), side("Left", profile.NewDisplayLeft),
			side("Above", profile.NewDisplayAbove), side("Below", profile.NewDisplayBelow),
		}},
		{"New display alignment", []preferenceOption{
			alignment("Center", profile.NewDisplayCenter), alignment("Edge", profile.NewDisplayEdge),
		}},
		{"New display VRR", []preferenceOption{vrr("Off", 0), vrr("On", 1), vrr("Fullscreen", 2)}},
		{"New setup notification", []preferenceOption{notify("On", true), notify("Off", false)}},
	}
}

func (r preferenceRow) selectedIndex(p prefs.Preferences) int {
	for idx, option := range r.options {
		if option.selected(p) {
			return idx
		}
	}
	return 0
}

// openPreferencesCmd reads the preferences the backend holds, through the
// daemon when it is running, before the dialog opens.
func (m Model) openPreferencesCmd() tea.Cmd {
	client := m.ipc
	direct := m.loadPreferences
	return func() tea.Msg {
		if client == nil {
			return preferencesLoadedMsg{prefs: direct()}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		loaded, err := client.Preferences(ctx)
		return preferencesLoadedMsg{prefs: loaded, err: err}
	}
}

func (m Model) savePreferencesCmd(p prefs.Preferences) tea.Cmd {
	client, store := m.ipc, m.store
	return func() tea.Msg {
		if client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := client.SetPreferences(ctx, p)
			return preferencesSavedMsg{err: err}
		}
		if store == nil {
			return preferencesSavedMsg{err: fmt.Errorf("no config directory to save preferences in")}
		}
		_, err := prefs.Save(store.BaseDir(), p)
		return preferencesSavedMsg{err: err}
	}
}

func (m *Model) updatePreferencesKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.prefsDialog == nil {
		m.mode = modeMain
		return m, nil
	}
	rows := preferenceRows()
	row := rows[m.prefsDialog.row]
	switch msg.String() {
	case "esc", "q":
		m.prefsDialog = nil
		m.mode = modeMain
	case "up", "k":
		m.prefsDialog.row = clampIndex(m.prefsDialog.row-1, len(rows))
	case "down", "j", "tab":
		m.prefsDialog.row = clampIndex(m.prefsDialog.row+1, len(rows))
	case "left", "h":
		m.choosePreference(m.prefsDialog.row, row.selectedIndex(m.prefsDialog.prefs)-1)
	case "right", "l":
		m.choosePreference(m.prefsDialog.row, row.selectedIndex(m.prefsDialog.prefs)+1)
	case "enter":
		return m, m.savePreferencesCmd(m.prefsDialog.prefs)
	}
	return m, nil
}

// choosePreference selects one option of a row. Like the inspector's choice
// rows, arrows stop at the ends instead of wrapping.
func (m *Model) choosePreference(rowIndex, optionIndex int) {
	rows := preferenceRows()
	if m.prefsDialog == nil || rowIndex < 0 || rowIndex >= len(rows) {
		return
	}
	options := rows[rowIndex].options
	if optionIndex < 0 || optionIndex >= len(options) {
		return
	}
	options[optionIndex].choose(&m.prefsDialog.prefs)
	m.prefsDialog.row = rowIndex
	m.prefsDialog.err = nil
}

func (m *Model) updatePreferencesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.prefsDialog == nil || msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if msg.Y < 0 || msg.Y >= len(lines) {
		return m, nil
	}
	line := lines[msg.Y]
	for rowIndex, row := range preferenceRows() {
		labelAt := strings.Index(line, row.label)
		if labelAt < 0 {
			continue
		}
		// Options follow the label; search after it so a label and an option
		// that share a word cannot be confused.
		from := labelAt + len(row.label)
		for optionIndex, option := range row.options {
			at := strings.Index(line[from:], " "+option.label+" ")
			if at < 0 {
				continue
			}
			start := lipgloss.Width(line[:from+at])
			if msg.X >= start && msg.X < start+lipgloss.Width(option.label)+2 {
				m.choosePreference(rowIndex, optionIndex)
				return m, nil
			}
		}
	}
	return m, nil
}

func (m Model) renderPreferences() string {
	if m.prefsDialog == nil {
		return ""
	}
	rows := preferenceRows()
	labelWidth := 0
	for _, row := range rows {
		labelWidth = max(labelWidth, lipgloss.Width(row.label))
	}

	body := make([]string, 0, len(rows)+4)
	for rowIndex, row := range rows {
		focused := rowIndex == m.prefsDialog.row
		marker, labelStyle := "  ", m.styles.subtle
		if focused {
			marker, labelStyle = "▸ ", m.styles.label
		}
		selected := row.selectedIndex(m.prefsDialog.prefs)
		options := make([]string, 0, len(row.options))
		for optionIndex, option := range row.options {
			style := m.styles.subtle
			if optionIndex == selected {
				style = withBG(withFG(lipgloss.NewStyle().Bold(true), m.styles.palette.chipFg), m.styles.palette.chipBg)
				if focused {
					style = m.styles.focused.UnsetPadding()
				}
			}
			options = append(options, style.Render(" "+option.label+" "))
		}
		body = append(body, marker+labelStyle.Render(fmt.Sprintf("%-*s", labelWidth, row.label))+"  "+strings.Join(options, ""))
	}
	helpWidth := max(20, m.modalMaxWidth()-6)
	if m.prefsDialog.err != nil {
		body = append(body, "", m.styles.statusError.MaxWidth(helpWidth).Render(m.prefsDialog.err.Error()))
	}
	body = append(body, "",
		m.styles.help.MaxWidth(helpWidth).Render("New display settings apply to displays no layout knows yet."),
		m.styles.help.MaxWidth(helpWidth).Render("↑↓ select, ←→ change. Enter saves. Esc discards."))
	return m.renderModalFrame("Preferences", body)
}
