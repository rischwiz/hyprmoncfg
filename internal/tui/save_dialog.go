package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) openSaveDialog() (tea.Model, tea.Cmd) {
	return m.openSaveDialogFor(saveDialogProfile)
}

func (m *Model) openQuitSaveDialog() (tea.Model, tea.Cmd) {
	return m.openSaveDialogFor(saveDialogQuit)
}

func (m *Model) openSaveDialogFor(purpose saveDialogPurpose) (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 64
	input.Width = m.saveDialogInputWidth()
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	name := m.saveDialogSuggestedName()
	input.SetValue(name)
	cmd := input.Focus()

	items := make([]profileListItem, 0, len(m.profiles))
	for _, prof := range m.profiles {
		items = append(items, profileListItem{name: prof.Name, updated: prof.UpdatedAt, outputs: len(prof.Outputs)})
	}
	prioritizeProfileListItem(items, name)

	inner := list.NewDefaultDelegate()
	inner.Styles.NormalTitle = m.styles.value
	inner.Styles.NormalDesc = m.styles.subtle
	inner.Styles.SelectedTitle = m.styles.focused.UnsetPadding()
	inner.Styles.SelectedDesc = m.styles.selectedDesc
	inner.Styles.DimmedTitle = m.styles.subtle
	inner.Styles.DimmedDesc = m.styles.subtle
	inner.Styles.FilterMatch = m.styles.badgeAccent
	// Highlighting marks the selected profile; an arrow beside it would repeat it.
	delegate := plainDelegate{inner}

	listHeight := clampInt(defaultHeight(m.height)-18, 3, 10)
	profileList := list.New(nil, delegate, m.saveDialogListWidth()-2, listHeight)
	profileList.Title = "Existing Profiles"
	profileList.SetShowHelp(false)
	profileList.SetShowPagination(false)
	profileList.SetShowStatusBar(false)
	profileList.SetFilteringEnabled(false)
	profileList.DisableQuitKeybindings()
	profileList.Styles.Title = m.styles.modalTitle
	profileList.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
	profileList.Styles.PaginationStyle = m.styles.subtle
	profileList.Styles.HelpStyle = m.styles.help
	profileList.Styles.NoItems = m.styles.subtle

	m.saveDialog = &saveDialogState{
		Input:   input,
		List:    profileList,
		All:     items,
		Filter:  "",
		Action:  defaultSaveAction(purpose),
		Purpose: purpose,
	}
	m.mode = modeSave
	m.saveOverwrite = ""
	m.rebuildSaveList(false)
	return m, cmd
}

func (m Model) saveDialogSuggestedName() string {
	if suggested := strings.TrimSpace(m.draftProfileName); suggested != "" {
		return suggested
	}
	if suggested := strings.TrimSpace(m.matchedProfileName); suggested != "" {
		return suggested
	}
	return defaultProfileName()
}

func prioritizeProfileListItem(items []profileListItem, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for idx, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.name), name) {
			continue
		}
		if idx == 0 {
			return
		}
		copy(items[1:idx+1], items[:idx])
		items[0] = item
		return
	}
}

func (m *Model) cycleSaveAction(delta int) {
	if m.saveDialog == nil {
		return
	}
	actions := saveActionsForPurpose(m.saveDialog.Purpose)
	current := 0
	for idx, action := range actions {
		if action == m.saveDialog.Action {
			current = idx
			break
		}
	}
	m.saveDialog.Action = actions[wrapIndex(current+delta, len(actions))]
}

func (m Model) selectedSaveAction() saveAction {
	if m.saveDialog == nil {
		return saveActionOnly
	}
	return m.saveDialog.Action
}

func (m Model) saveActionLabel(action saveAction) string {
	switch action {
	case saveActionApply:
		return "Save & Apply"
	case saveActionSaveQuit:
		return "Save, Apply & Quit"
	case saveActionDiscardQuit:
		return "Quit Without Saving"
	case saveActionCancel:
		return "Cancel"
	default:
		return "Save"
	}
}

func (m Model) renderSaveActionButtons() string {
	purpose := saveDialogProfile
	if m.saveDialog != nil {
		purpose = m.saveDialog.Purpose
	}
	actions := saveActionsForPurpose(purpose)
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		style := m.styles.value
		if action == m.selectedSaveAction() {
			style = m.styles.focused.UnsetPadding()
		}
		parts = append(parts, style.Render("["+m.saveActionLabel(action)+"]"))
	}
	return strings.Join(parts, "  ")
}

func defaultSaveAction(purpose saveDialogPurpose) saveAction {
	if purpose == saveDialogQuit {
		return saveActionSaveQuit
	}
	return saveActionApply
}

func saveActionsForPurpose(purpose saveDialogPurpose) []saveAction {
	if purpose == saveDialogQuit {
		return []saveAction{saveActionSaveQuit, saveActionDiscardQuit, saveActionCancel}
	}
	return []saveAction{saveActionOnly, saveActionApply, saveActionCancel}
}

func (m *Model) rebuildSaveList(resetSelection bool) {
	if m.saveDialog == nil {
		return
	}

	filter := strings.ToLower(strings.TrimSpace(m.saveDialog.Filter))
	current := ""
	if selected, ok := m.saveDialog.List.SelectedItem().(profileListItem); ok {
		current = selected.name
	}

	filtered := make([]list.Item, 0, len(m.saveDialog.All))
	for _, item := range m.saveDialog.All {
		if filter == "" || strings.Contains(strings.ToLower(item.name), filter) {
			filtered = append(filtered, item)
		}
	}
	m.saveDialog.List.SetItems(filtered)
	if len(filtered) == 0 {
		return
	}
	if resetSelection || current == "" {
		m.saveDialog.List.Select(0)
		return
	}
	for idx, item := range filtered {
		profileItem := item.(profileListItem)
		if profileItem.name == current {
			m.saveDialog.List.Select(idx)
			return
		}
	}
}

func (m *Model) syncSaveNameFromSelection() {
	if m.saveDialog == nil {
		return
	}
	if selected, ok := m.saveDialog.List.SelectedItem().(profileListItem); ok {
		m.saveDialog.Input.SetValue(selected.name)
		m.saveDialog.Input.CursorEnd()
	}
}

func (m Model) updateSaveKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.saveDialog == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.saveDialog.Purpose == saveDialogQuit {
			m.setStatusOK("Quit cancelled")
		}
		m.mode = modeMain
		m.saveDialog = nil
		m.saveOverwrite = ""
		return m, nil
	case "tab", "right":
		m.cycleSaveAction(1)
		return m, nil
	case "shift+tab", "left":
		m.cycleSaveAction(-1)
		return m, nil
	case "enter":
		if m.selectedSaveAction() == saveActionDiscardQuit {
			return m, tea.Quit
		}
		if m.selectedSaveAction() == saveActionCancel {
			if m.saveDialog.Purpose == saveDialogQuit {
				m.setStatusOK("Quit cancelled")
			}
			m.mode = modeMain
			m.saveDialog = nil
			m.saveOverwrite = ""
			return m, nil
		}
		name := strings.TrimSpace(m.saveDialog.Input.Value())
		if name == "" {
			m.setStatusErr("Profile name cannot be empty")
			return m, nil
		}
		if m.profileExists(name) {
			m.saveOverwrite = name
			m.mode = modeSaveConfirm
			return m, nil
		}
		return m, m.saveCmd(m.currentProfile(name))
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.saveDialog.List, cmd = m.saveDialog.List.Update(msg)
		m.syncSaveNameFromSelection()
		return m, cmd
	default:
		var cmd tea.Cmd
		before := m.saveDialog.Input.Value()
		m.saveDialog.Input, cmd = m.saveDialog.Input.Update(msg)
		if m.saveDialog.Input.Value() != before {
			m.saveDialog.Filter = m.saveDialog.Input.Value()
			m.rebuildSaveList(true)
		}
		return m, cmd
	}
}

func (m Model) updateSaveConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n":
		m.mode = modeSave
		return m, nil
	case "enter", "y":
		name := strings.TrimSpace(m.saveOverwrite)
		if name == "" {
			m.mode = modeSave
			return m, nil
		}
		return m, m.saveCmd(m.currentProfile(name))
	default:
		return m, nil
	}
}
