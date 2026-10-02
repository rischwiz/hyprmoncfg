package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profileio"
)

// profileNameInputState is the dialog that asks for a name when renaming or
// duplicating the highlighted profile. Neither applies a layout.
type profileNameInputState struct {
	Source    string
	Duplicate bool
	Input     textinput.Model
	Err       error
}

type profileNameMsg struct {
	source    string
	name      string
	duplicate bool
	err       error
}

func (m *Model) openProfileNameInput(duplicate bool) tea.Cmd {
	if len(m.profiles) == 0 || m.selectedProfile < 0 || m.selectedProfile >= len(m.profiles) {
		return nil
	}

	selected := m.profiles[m.selectedProfile]
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 64
	input.Width = clampInt(m.modalMaxWidth()-16, 24, 48)
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	input.Placeholder = "profile name"
	if !duplicate {
		input.SetValue(selected.Name)
	}
	cmd := input.Focus()

	m.nameInput = &profileNameInputState{Source: selected.Name, Duplicate: duplicate, Input: input}
	m.mode = modeProfileNameInput
	return cmd
}

func (m Model) renderProfileNameInput() string {
	if m.nameInput == nil {
		return ""
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.nameInput.Input.View())
	title := fmt.Sprintf("Rename %s", m.nameInput.Source)
	help := "Enter renames the profile. Nothing is applied. Esc cancels."
	if m.nameInput.Duplicate {
		title = fmt.Sprintf("Duplicate %s", m.nameInput.Source)
		help = "Enter saves a copy without its post-apply command. Nothing is applied. Esc cancels."
	}
	body := []string{m.styles.label.Render("New name"), inputBox}
	if m.nameInput.Err != nil {
		body = append(body, "", m.styles.statusError.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(m.nameInput.Err.Error()))
	}
	body = append(body, "", m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(help))
	return m.renderModalFrame(title, body)
}

func (m *Model) updateProfileNameInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.nameInput == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.nameInput = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitProfileNameInput()
	}

	var cmd tea.Cmd
	m.nameInput.Input, cmd = m.nameInput.Input.Update(msg)
	m.nameInput.Err = nil
	return m, cmd
}

func (m *Model) commitProfileNameInput() tea.Cmd {
	if m.nameInput == nil {
		m.mode = modeMain
		return nil
	}
	state := *m.nameInput
	name := strings.TrimSpace(state.Input.Value())
	if name == "" {
		m.nameInput.Err = errors.New("Type a name for the profile.")
		return nil
	}
	if name == state.Source {
		if state.Duplicate {
			m.nameInput.Err = errors.New("The copy needs a different name.")
			return nil
		}
		m.nameInput = nil
		m.mode = modeMain
		return nil
	}
	// The backend is authoritative; this only spares a round trip for the
	// collision a person can see in the list.
	for _, existing := range m.profiles {
		if existing.Name == state.Source && !state.Duplicate {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(existing.Name), name) {
			m.nameInput.Err = fmt.Errorf("A profile named %q already exists.", existing.Name)
			return nil
		}
	}

	m.nameInput = nil
	m.mode = modeMain
	return m.profileNameCmd(state.Source, name, state.Duplicate)
}

func (m Model) profileNameCmd(source string, name string, duplicate bool) tea.Cmd {
	client, store := m.ipc, m.store
	return func() tea.Msg {
		result := profileNameMsg{source: source, name: name, duplicate: duplicate}
		switch {
		case client != nil:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if duplicate {
				result.err = client.Duplicate(ctx, ipc.DuplicateParams{Name: source, NewName: name})
			} else {
				result.err = client.Rename(ctx, source, name)
			}
		case duplicate:
			result.err = profileio.Duplicate(store, source, name, false)
		default:
			result.err = profileio.Rename(store, source, name)
		}
		return result
	}
}

// applyProfileNameResult records a finished rename or duplicate. A rename
// carries the editor's own references to the profile over to the new name.
func (m *Model) applyProfileNameResult(msg profileNameMsg) {
	if msg.err != nil {
		m.setStatusErr(msg.err.Error())
		return
	}
	if msg.duplicate {
		m.setStatusOK(fmt.Sprintf("Duplicated profile %q as %q", msg.source, msg.name))
	} else {
		for _, ref := range []*string{&m.draftProfileName, &m.matchedProfileName, &m.activeProfileName, &m.profileOverride} {
			if strings.EqualFold(strings.TrimSpace(*ref), strings.TrimSpace(msg.source)) {
				*ref = msg.name
			}
		}
		m.setStatusOK(fmt.Sprintf("Renamed profile %q to %q", msg.source, msg.name))
	}
	m.selectProfileAfterRefresh = msg.name
}
