package tui

import (
	"reflect"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// undoLimit bounds how many steps the editor remembers.
const undoLimit = 100

// undoCoalesceWindow is how close two edits with the same key must be to
// count as one step, so a run of nudges undoes at once.
const undoCoalesceWindow = time.Second

// draftSnapshot is everything an edit can change in the draft. Selection and
// live state are left out: moving the highlight is not an edit.
type draftSnapshot struct {
	outputs        []editableOutput
	workspaces     workspaceEditor
	disableUnknown bool
	dirty          bool
	draftSaved     bool
}

type undoHistory struct {
	undo []draftSnapshot
	redo []draftSnapshot
	// generation changes whenever the draft is replaced or restored rather
	// than edited, which is how Update tells the two apart.
	generation int
	lastKey    string
	lastAt     time.Time
}

func (m Model) draftSnapshot() draftSnapshot {
	workspaces := m.workspaceEdit
	workspaces.SelectedField, workspaces.SelectedOrder = 0, 0
	workspaces.MonitorOrder = append([]string(nil), workspaces.MonitorOrder...)
	workspaces.Rules = append([]profile.WorkspaceRule(nil), workspaces.Rules...)
	return draftSnapshot{
		outputs:        append([]editableOutput(nil), m.editOutputs...),
		workspaces:     workspaces,
		disableUnknown: m.disableUnknownOutputs,
		dirty:          m.dirty,
		draftSaved:     m.draftSaved,
	}
}

func (s draftSnapshot) sameDraft(other draftSnapshot) bool {
	return s.disableUnknown == other.disableUnknown &&
		reflect.DeepEqual(s.outputs, other.outputs) &&
		reflect.DeepEqual(s.workspaces, other.workspaces)
}

func (m *Model) restoreDraft(snapshot draftSnapshot) {
	selectedField, selectedOrder := m.workspaceEdit.SelectedField, m.workspaceEdit.SelectedOrder
	m.editOutputs = append([]editableOutput(nil), snapshot.outputs...)
	m.workspaceEdit = snapshot.workspaces
	m.workspaceEdit.MonitorOrder = append([]string(nil), snapshot.workspaces.MonitorOrder...)
	m.workspaceEdit.Rules = append([]profile.WorkspaceRule(nil), snapshot.workspaces.Rules...)
	m.workspaceEdit.SelectedField, m.workspaceEdit.SelectedOrder = selectedField, selectedOrder
	m.disableUnknownOutputs = snapshot.disableUnknown
	m.dirty, m.draftSaved = snapshot.dirty, snapshot.draftSaved
	m.history.generation++
	m.drag = nil
	m.syncSelections()
	m.revalidate()
}

// clearUndo forgets the history. Loading live state or a profile replaces the
// draft, and undoing across that would bring back displays that may be gone.
func (m *Model) clearUndo() {
	m.history = undoHistory{generation: m.history.generation + 1}
}

func (m *Model) undoEdit() {
	if len(m.history.undo) == 0 {
		m.setStatusErr("Nothing to undo")
		return
	}
	last := len(m.history.undo) - 1
	snapshot := m.history.undo[last]
	m.history.undo = m.history.undo[:last]
	m.history.redo = append(m.history.redo, m.draftSnapshot())
	m.restoreDraft(snapshot)
	m.setStatusOK("Undid the last change")
}

func (m *Model) redoEdit() {
	if len(m.history.redo) == 0 {
		m.setStatusErr("Nothing to redo")
		return
	}
	last := len(m.history.redo) - 1
	snapshot := m.history.redo[last]
	m.history.redo = m.history.redo[:last]
	m.history.undo = append(m.history.undo, m.draftSnapshot())
	m.restoreDraft(snapshot)
	m.setStatusOK("Redid the change")
}

// recordUndo remembers the draft as it was before an input that changed it.
func (m *Model) recordUndo(before draftSnapshot, generation int, key string) {
	if m.history.generation != generation || before.sameDraft(m.draftSnapshot()) {
		return
	}
	now := m.clock()
	coalesce := key != "" && key == m.history.lastKey && len(m.history.undo) > 0 &&
		now.Sub(m.history.lastAt) <= undoCoalesceWindow
	m.history.lastKey, m.history.lastAt = key, now
	m.history.redo = nil
	if coalesce {
		return
	}
	m.history.undo = append(m.history.undo, before)
	if len(m.history.undo) > undoLimit {
		m.history.undo = m.history.undo[len(m.history.undo)-undoLimit:]
	}
}

// undoCoalesceKey names an input for coalescing: one drag is one step, and so
// is a quick run of the same key on the same display. Other inputs get no key
// and always stand alone.
func (m Model) undoCoalesceKey(msg tea.Msg) string {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if m.drag != nil {
			return "drag"
		}
	case tea.KeyMsg:
		if m.mode == modeMain {
			return "key:" + msg.String() + ":" + string(rune('0'+m.tab)) + ":" + m.selectedOutputKey()
		}
	}
	return ""
}

func (m Model) selectedOutputKey() string {
	if m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return ""
	}
	return m.editOutputs[m.selectedOutput].Key
}

// Update records an undo step around every key and pointer input that edits
// the draft, so individual edit paths do not each have to remember to.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
	default:
		return m.update(msg)
	}

	before, generation, key := m.draftSnapshot(), m.history.generation, m.undoCoalesceKey(msg)
	updated, cmd := m.update(msg)
	var next Model
	switch updated := updated.(type) {
	case Model:
		next = updated
	case *Model:
		next = *updated
	default:
		return updated, cmd
	}
	next.recordUndo(before, generation, key)
	return next, cmd
}
