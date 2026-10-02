package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// profileMenuState is the action menu for the highlighted profile. It offers
// the same operations as the buttons under the list and runs them the same
// way, so the two can never drift apart.
type profileMenuState struct {
	profile string
	index   int
}

// profileMenuAction pairs a label with the profile key that performs it.
// Labels carry no key hints; shortcuts live in the footer and help.
type profileMenuAction struct {
	label string
	key   tea.KeyMsg
}

func runeKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

var profileMenuActions = []profileMenuAction{
	{"Preview", tea.KeyMsg{Type: tea.KeyEnter}},
	{"Edit layout", runeKey("l")},
	{"Edit post-apply command", runeKey("e")},
	{"Rename", runeKey("n")},
	{"Duplicate", runeKey("c")},
	{"Delete", runeKey("d")},
}

func (m *Model) openProfileMenu() {
	if len(m.profiles) == 0 || m.selectedProfile < 0 || m.selectedProfile >= len(m.profiles) {
		m.setStatusErr("No profiles available")
		return
	}
	m.profileMenu = &profileMenuState{profile: m.profiles[m.selectedProfile].Name}
	m.mode = modeProfileMenu
}

func (m *Model) updateProfileMenuKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.profileMenu == nil {
		m.mode = modeMain
		return m, nil
	}
	switch msg.String() {
	case "esc", "q", "m":
		m.profileMenu = nil
		m.mode = modeMain
	case "up", "k":
		m.profileMenu.index = clampIndex(m.profileMenu.index-1, len(profileMenuActions))
	case "down", "j":
		m.profileMenu.index = clampIndex(m.profileMenu.index+1, len(profileMenuActions))
	case "enter":
		return m.runProfileMenuAction(m.profileMenu.index)
	}
	return m, nil
}

// runProfileMenuAction closes the menu and performs the action exactly as its
// key would on the Profiles tab.
func (m *Model) runProfileMenuAction(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(profileMenuActions) {
		return m, nil
	}
	m.profileMenu = nil
	m.mode = modeMain
	return m.updateProfileKeys(profileMenuActions[index].key)
}

func (m *Model) updateProfileMenuMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.profileMenu == nil || msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if msg.Y < 0 || msg.Y >= len(lines) {
		return m, nil
	}
	// Each row is its marker or indent, the label, then a space, so "Edit
	// layout" cannot match the row that says "Edit post-apply command".
	for index, action := range profileMenuActions {
		at := strings.Index(lines[msg.Y], profileMenuMarker+action.label+" ")
		if at < 0 {
			at = strings.Index(lines[msg.Y], profileMenuIndent+action.label+" ")
		}
		if at >= 0 {
			return m.runProfileMenuAction(index)
		}
	}
	return m, nil
}

const (
	profileMenuMarker = "▸ "
	profileMenuIndent = "  "
)

func (m Model) renderProfileMenu() string {
	if m.profileMenu == nil {
		return ""
	}
	width := 0
	for _, action := range profileMenuActions {
		width = max(width, len(action.label))
	}
	body := make([]string, 0, len(profileMenuActions)+2)
	for index, action := range profileMenuActions {
		// The trailing pad gives every row the same width and a space after
		// the label, which the pointer hit test relies on.
		label := fmt.Sprintf("%-*s ", width, action.label)
		if index == m.profileMenu.index {
			body = append(body, profileMenuMarker+m.styles.focused.UnsetPadding().Render(label))
			continue
		}
		body = append(body, profileMenuIndent+m.styles.value.Render(label))
	}
	body = append(body, "", m.styles.help.Render("Enter runs the action. Esc closes."))
	return m.renderModalFrame(fmt.Sprintf("Actions for %s", m.profileMenu.profile), body)
}
