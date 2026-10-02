package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func (m *Model) loadLiveState() {
	m.clearUndo()
	prevOutputs := m.editOutputs
	selectedKey := ""
	if m.selectedOutput >= 0 && m.selectedOutput < len(prevOutputs) {
		selectedKey = prevOutputs[m.selectedOutput].Key
	}
	draft, sourceName, suggestedName := profile.EditorProfileFromState(m.profiles, m.monitors, m.workspaceRules)
	if len(prevOutputs) > 0 && !m.resetRequested {
		previous := profile.Profile{Outputs: make([]profile.OutputConfig, 0, len(prevOutputs))}
		for _, output := range prevOutputs {
			previous.Outputs = append(previous.Outputs, output.profileOutput())
		}
		profile.PreserveUnreportedSettings(&draft, previous)
	}

	m.editOutputs = make([]editableOutput, 0, len(draft.Outputs))
	for _, saved := range draft.Outputs {
		live, ok := m.findLiveMonitor(saved)
		m.editOutputs = append(m.editOutputs, editableOutputFromProfile(saved, live, ok))
	}
	m.recoverMirroredIdentity()
	m.workspaceEdit = workspaceEditorFromSettings(draft.Workspaces, m.editOutputs)
	m.disableUnknownOutputs = draft.DisableUnknownOutputs
	m.matchedProfileName = ""
	m.activeProfileName = ""
	if sourceName != "" {
		m.draftProfileName = sourceName
		m.matchedProfileName = sourceName
		m.activeProfileName = sourceName
		m.draftExec = draft.Exec
	} else {
		m.draftProfileName = ""
		m.draftExec = ""
		m.matchedProfileName = suggestedName
	}
	m.resetRequested = false
	if idx := focusedOutputIndex(m.editOutputs); idx >= 0 {
		m.selectedOutput = idx
	} else if selectedKey != "" {
		m.selectedOutput = outputIndexByKey(m.editOutputs, selectedKey)
	}
	m.selectedOutput = clampIndex(m.selectedOutput, len(m.editOutputs))
	m.inspectorField = clampIndex(m.inspectorField, len(layoutFields))
	// matchedProfileName is already "the active profile, or the best scoring
	// one", which is the profile worth landing on in the profiles tab.
	if idx := m.profileIndexByName(m.matchedProfileName); idx >= 0 {
		m.selectedProfile = idx
	}
	m.picker = nil
	m.input = nil
	m.drag = nil
	m.markClean()

	m.revalidate()
}

func (m *Model) loadProfile(p profile.Profile) {
	m.clearUndo()
	outputs := make([]editableOutput, 0, len(p.Outputs))
	for _, saved := range p.Outputs {
		live, ok := m.findLiveMonitor(saved)
		outputs = append(outputs, editableOutputFromProfile(saved, live, ok))
	}
	m.editOutputs = outputs
	m.workspaceEdit = workspaceEditorFromSettings(p.Workspaces, m.editOutputs)
	m.disableUnknownOutputs = p.DisableUnknownOutputs
	m.selectedOutput = clampIndex(0, len(m.editOutputs))
	m.inspectorField = 0
	m.picker = nil
	m.input = nil
	m.drag = nil
	m.dirty = true
	m.draftSaved = true
	m.draftProfileName = p.Name
	m.matchedProfileName = p.Name
	m.draftExec = p.Exec
	m.setStatusOK(fmt.Sprintf("Loaded profile %q into editor", p.Name))

	m.revalidate()
}

// recoverMirroredIdentity restores Make/Model/Serial/Key for monitors whose
// identity was degraded by Hyprland while mirroring. It looks up the real
// identity from saved profiles by matching connector names.
func (m *Model) recoverMirroredIdentity() {
	for i, output := range m.editOutputs {
		if output.MirrorOf == "" || strings.TrimSpace(output.Make+" "+output.Model) != "" {
			continue
		}
		for _, prof := range m.profiles {
			for _, saved := range prof.Outputs {
				if saved.Name == output.Name && strings.TrimSpace(saved.Make+" "+saved.Model) != "" {
					m.editOutputs[i].Make = saved.Make
					m.editOutputs[i].Model = saved.Model
					m.editOutputs[i].Serial = saved.Serial
					m.editOutputs[i].Key = saved.Key
					break
				}
			}
			if strings.TrimSpace(m.editOutputs[i].Make+" "+m.editOutputs[i].Model) != "" {
				break
			}
		}
	}
}

func (m *Model) syncSelections() {
	m.selectedOutput = clampIndex(m.selectedOutput, len(m.editOutputs))
	m.selectedProfile = clampIndex(m.selectedProfile, len(m.profiles))
	m.inspectorField = clampIndex(m.inspectorField, len(layoutFields))
	workspaceItems := m.workspaceItemCount()
	m.workspaceEdit.SelectedField = clampIndex(m.workspaceEdit.SelectedField, workspaceItems)
	if m.workspaceEdit.SelectedField >= len(workspaceFields) {
		m.workspaceEdit.SelectedOrder = m.workspaceEdit.SelectedField - len(workspaceFields)
	} else {
		m.workspaceEdit.SelectedOrder = clampIndex(m.workspaceEdit.SelectedOrder, len(m.workspaceEdit.MonitorOrder))
	}
}

func (m Model) profileExists(name string) bool {
	_, ok := m.profileByName(name)
	return ok
}

func (m Model) profileIndexByName(name string) int {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return -1
	}
	for idx, prof := range m.profiles {
		if strings.TrimSpace(strings.ToLower(prof.Name)) == name {
			return idx
		}
	}
	return -1
}

func (m Model) profileByName(name string) (profile.Profile, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, prof := range m.profiles {
		if strings.TrimSpace(strings.ToLower(prof.Name)) == name {
			return prof, true
		}
	}
	return profile.Profile{}, false
}

func (m Model) hasMirroredOutputs() bool {
	for _, output := range m.editOutputs {
		if output.Enabled && output.MirrorOf != "" {
			return true
		}
	}
	return false
}

func (m Model) outputNameForKey(key string) string {
	return outputNameForKeyIn(m.editOutputs, key)
}

func outputNameForKeyIn(outputs []editableOutput, key string) string {
	for _, output := range outputs {
		if output.Key == key {
			return output.Name
		}
	}
	return key
}

func (m Model) currentProfile(name string) profile.Profile {
	p := profile.New(name, m.currentProfileOutputs())
	p.Workspaces = m.workspaceEdit.settings()
	p.DisableUnknownOutputs = m.disableUnknownOutputs
	p.Exec = m.currentProfileExec(name)
	p.Normalize()
	return p
}

func (m Model) currentProfileExec(name string) string {
	if exec := strings.TrimSpace(m.draftExec); exec != "" {
		return exec
	}
	if existing, ok := m.profileByName(name); ok {
		return existing.Exec
	}
	return ""
}

func (m Model) currentProfileOutputs() []profile.OutputConfig {
	outputs := make([]profile.OutputConfig, 0, len(m.editOutputs))
	for _, output := range m.editOutputs {
		outputs = append(outputs, output.profileOutput())
	}
	return outputs
}

func (m *Model) revalidate() {
	m.layoutErr = apply.ValidateLayout(m.currentProfileOutputs())
}

func (m *Model) layoutChanged() {
	m.markDirty()
	m.revalidate()
}

func (m *Model) nudgeSelectedOutput(dx, dy int, snapThreshold int) tea.Cmd {
	m.moveSelectedOutput(dx, dy)
	return m.showSnapHint(m.previewSelectedSnap(snapThreshold))
}

func liveConfigSignature(monitors []hypr.Monitor, lidState lid.State) string {
	return profile.MonitorStateHash(monitors) + "|lid=" + string(lidState)
}

func (m Model) liveConfigSignature() string {
	return liveConfigSignature(m.monitors, m.lidState)
}

func outputDisplayLabel(key string, outputs []profile.OutputConfig) string {
	for _, o := range outputs {
		if o.Key == key {
			if label := strings.TrimSpace(o.Make + " " + o.Model); label != "" {
				return label
			}
			return o.Name
		}
	}
	return key
}

func outputConnector(key string, outputs []profile.OutputConfig) string {
	for _, o := range outputs {
		if o.Key == key {
			return o.Name
		}
	}
	return key
}

func (m Model) outputLabelForKey(key string) string {
	return outputDisplayLabel(key, m.currentProfileOutputs())
}

func (m Model) findLiveMonitor(output profile.OutputConfig) (hypr.Monitor, bool) {
	return profile.NewMonitorResolver(m.monitors).ResolveOutput(output)
}

func (m *Model) setStatusErr(msg string) {
	m.status = msg
	m.statusErr = true
}

func (m *Model) setStatusOK(msg string) {
	m.status = msg
	m.statusErr = false
}

func (m *Model) notifyUser(msg string, isErr bool) tea.Cmd {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}
	m.toastSeq++
	token := m.toastSeq
	m.toast = &toastState{
		message: msg,
		err:     isErr,
		token:   token,
	}
	return clearToastCmd(token)
}

func (m *Model) markDirty() {
	m.dirty = true
	m.draftSaved = false
}

func (m *Model) markClean() {
	m.dirty = false
	m.draftSaved = false
}
