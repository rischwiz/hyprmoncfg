package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/ipc"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func (m Model) updateMainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		if m.applying {
			m.quitAfterRevert = true
			m.statusErr = false
			m.status = "Waiting for apply to finish, then restoring the previous configuration..."
			return m, nil
		}
		if m.dirty {
			return m.openQuitSaveDialog()
		}
		return m, tea.Quit
	case "1":
		m.tab = tabLayout
		return m, nil
	case "2":
		m.tab = tabWorkspaces
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual && !m.workspaceEdit.ManualRulesInitialized {
			m.workspaceEdit.Rules = m.materializeManualWorkspaceRules()
			m.workspaceEdit.ManualRulesInitialized = len(m.workspaceEdit.Rules) > 0
			if m.workspaceEdit.ManualRulesInitialized {
				m.markDirty()
			}
		}
		return m, nil
	case "3":
		m.tab = tabProfiles
		return m, nil
	case "?":
		m.mode = modeKeybindings
		return m, nil
	case "R":
		return m, m.restartDaemonCmd()
	case "U":
		m.disableUnknownOutputs = !m.disableUnknownOutputs
		m.markDirty()
		if m.disableUnknownOutputs {
			m.setStatusOK("Displays outside this profile will be disabled (save to keep)")
		} else {
			m.setStatusOK("New displays will extend the layout to the right (save to keep)")
		}
		return m, nil
	case "r":
		m.resetRequested = true
		m.draftProfileName = ""
		m.matchedProfileName = ""
		m.draftExec = ""
		m.markClean()
		return m, m.refreshCmd(false)
	case "s":
		if m.tab == tabProfiles {
			if len(m.profiles) == 0 {
				m.setStatusErr("No profiles to save")
				return m, nil
			}
			return m, m.saveProfileCmd(m.profiles[m.selectedProfile])
		}
		return m.openSaveDialog()
	case "a":
		if m.applying {
			m.setStatusErr("A configuration is already being applied")
			return m, nil
		}
		if m.tab == tabProfiles {
			if len(m.profiles) == 0 {
				m.setStatusErr("No profiles available")
				return m, nil
			}
			m.applying = true
			target := m.profiles[m.selectedProfile]
			return m, m.applyCmd(target)
		}
		m.applying = true
		return m, m.applyCmd(m.currentProfile("draft"))
	}

	switch m.tab {
	case tabLayout:
		return m.updateLayoutKeys(msg)
	case tabProfiles:
		return m.updateProfileKeys(msg)
	case tabWorkspaces:
		return m.updateWorkspaceKeys(msg)
	default:
		return m, nil
	}
}

func (m *Model) updateLayoutKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.editOutputs) == 0 {
		return m, nil
	}

	switch msg.String() {
	case "tab":
		m.cycleLayoutPane(1)
		return m, nil
	case "shift+tab":
		m.cycleLayoutPane(-1)
		return m, nil
	case "0":
		m.moveSelectedOutputToOrigin()
		return m, nil
	case "[":
		m.selectedOutput = clampIndex(m.selectedOutput-1, len(m.editOutputs))
		return m, nil
	case "]":
		m.selectedOutput = clampIndex(m.selectedOutput+1, len(m.editOutputs))
		return m, nil
	}

	if m.layoutFocus == layoutFocusCanvas {
		if direction, ok := layoutSnapDirection(msg.String()); ok {
			return m, m.snapSelectedOutput(direction)
		}
		if dx, dy, ok := layoutMoveDelta(msg.String()); ok {
			return m, m.nudgeSelectedOutput(dx, dy, 24)
		}
		switch msg.String() {
		case " ":
			m.toggleSelectedOutput()
			return m, nil
		case "enter":
			m.layoutFocus = layoutFocusInspector
			m.normalizeInspectorField()
			return m, nil
		default:
			return m, nil
		}
	}

	switch msg.String() {
	case "up", "k":
		m.moveInspectorField(-1)
	case "down", "j":
		m.moveInspectorField(1)
	case "left", "h", "-", "_":
		m.adjustInspectorField(-1)
	case "right", "l", "+", "=":
		m.adjustInspectorField(1)
	case "shift+left", "shift+right":
		// Position fields take single-pixel steps; everything else steps as usual.
		delta := 1
		if msg.String() == "shift+left" {
			delta = -1
		}
		if m.inspectorField == 7 || m.inspectorField == 8 {
			m.nudgeInspectorPosition(delta)
		} else {
			m.adjustInspectorField(delta)
		}
	case " ", "enter":
		return m, m.activateInspectorField()
	default:
		return m, nil
	}

	return m, nil
}

func (m Model) updateProfileKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case " ":
		return m.toggleProfileAutomatic()
	case "up", "k":
		m.selectedProfile = clampIndex(m.selectedProfile-1, len(m.profiles))
	case "down", "j":
		m.selectedProfile = clampIndex(m.selectedProfile+1, len(m.profiles))
	case "e":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to edit")
			return m, nil
		}
		return m, m.openProfileExecInput()
	case "n":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to rename")
			return m, nil
		}
		return m, m.openProfileNameInput(false)
	case "c":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to duplicate")
			return m, nil
		}
		return m, m.openProfileNameInput(true)
	case "d":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to delete")
			return m, nil
		}
		m.mode, m.deleteProfileName = modeDeleteConfirm, m.profiles[m.selectedProfile].Name
		return m, nil
	case "enter":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles available")
			return m, nil
		}
		if m.applying {
			m.setStatusErr("A configuration is already being applied")
			return m, nil
		}
		m.applying = true
		return m, m.applyCmd(m.profiles[m.selectedProfile])
	case "l":
		if len(m.profiles) == 0 {
			m.setStatusErr("No profiles to load")
			return m, nil
		}
		m.loadProfile(m.profiles[m.selectedProfile])
		m.tab = tabLayout
	default:
		return m, nil
	}

	return m, nil
}

func (m Model) profileAutomatic() bool {
	return m.daemonOK && strings.TrimSpace(m.profileOverride) == ""
}

func (m Model) toggleProfileAutomatic() (tea.Model, tea.Cmd) {
	if m.ipc == nil || !m.daemonOK {
		m.setStatusErr("Automatic profile selection requires the daemon")
		return m, nil
	}
	if m.profileModePending {
		m.setStatusErr("Profile selection mode is already being updated")
		return m, nil
	}
	if m.applying || m.pending != nil {
		m.setStatusErr("Finish the active profile change first")
		return m, nil
	}

	enabled := !m.profileAutomatic()
	m.profileModePending = true
	if enabled {
		m.setStatusOK("Enabling automatic profile selection...")
	} else {
		m.setStatusOK("Turning off automatic profile selection...")
	}
	return m, m.setProfileAutomaticCmd(enabled)
}

func (m Model) updateWorkspaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	inItems := m.workspaceEdit.SelectedField >= len(workspaceFields)
	if inItems {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	}

	switch msg.String() {
	case "up":
		m.moveWorkspaceSelection(-1, true)
		return m, nil
	case "down":
		m.moveWorkspaceSelection(1, true)
		return m, nil
	case "pgup":
		m.moveWorkspaceSelection(-m.workspacePageStep(), false)
		return m, nil
	case "pgdown":
		m.moveWorkspaceSelection(m.workspacePageStep(), false)
		return m, nil
	case "home":
		m.setWorkspaceSelection(0)
		return m, nil
	case "end":
		m.setWorkspaceSelection(m.workspaceItemCount() - 1)
		return m, nil
	case "left", "h", "-", "_":
		if inItems {
			m.adjustWorkspaceItem(-1)
		} else {
			m.adjustWorkspaceField(-1)
		}
	case "right", "l", "+", "=":
		if inItems {
			m.adjustWorkspaceItem(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	case "enter":
		if !inItems && m.workspaceEdit.Enabled {
			switch m.workspaceEdit.SelectedField {
			case 1:
				return m, m.openNumericInput(
					numericInputWorkspaceCount,
					-1,
					"Set Workspace Count",
					"Type any positive number. Enter applies. Esc cancels.",
					strconv.Itoa(m.workspaceEdit.MaxWorkspaces),
				)
			case 2:
				if m.workspaceEdit.Strategy == profile.WorkspaceStrategySequential {
					return m, m.openNumericInput(
						numericInputWorkspaceGroupSize,
						-1,
						"Set Workspace Group Size",
						"Type any positive number. Enter applies. Esc cancels.",
						strconv.Itoa(m.workspaceEdit.GroupSize),
					)
				}
			}
		}
		if inItems {
			m.adjustWorkspaceItem(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	case " ":
		if inItems {
			m.adjustWorkspaceItem(1)
		} else {
			m.adjustWorkspaceField(1)
		}
	default:
		return m, nil
	}

	m.markDirty()
	return m, nil
}

func (m Model) updateConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending == nil {
		m.mode = modeMain
		return m, nil
	}

	switch answerKey(msg) {
	case "ctrl+c", "q":
		m.quitAfterRevert = true
		return m, m.revertCmd(*m.pending, "quit")
	case "y", "enter":
		var toastCmd tea.Cmd

		p := m.pending.profile
		confirmErr := m.confirmPending(*m.pending)
		if confirmErr != nil && m.pending.remote {
			if errors.Is(confirmErr, ipc.ErrTransactionUnavailable) {
				m.mode = modeMain
				m.pending = nil
				m.disarmPendingRemote()
				m.markClean()
				m.draftProfileName = ""
				m.matchedProfileName = ""
				m.draftExec = ""
				m.setStatusOK("Configuration reverted: confirmation timeout")
				return m, m.refreshCmd(false)
			}
			m.setStatusErr(fmt.Sprintf("Could not confirm configuration: %v", confirmErr))
			return m, nil
		}
		if target := strings.TrimSpace(p.Name); target != "" && target != "draft" {
			m.draftProfileName = target
			m.matchedProfileName = target
		}

		if confirmErr != nil {
			toastCmd = m.notifyUser(fmt.Sprintf("Post-apply failed for %q: %v", p.Name, confirmErr), true)
		}

		m.mode = modeMain
		m.pending = nil
		m.disarmPendingRevert()
		m.disarmPendingRemote()
		m.markClean()
		m.setStatusOK("Configuration kept")
		if m.quitAfterApply {
			m.quitAfterApply = false
			return m, tea.Quit
		}
		return m, tea.Batch(m.refreshCmd(false), toastCmd)
	case "n", "esc":
		return m, m.revertCmd(*m.pending, "user request")
	default:
		return m, nil
	}
}
