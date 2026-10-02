package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// moveSelectedOutputToOrigin puts the selected monitor where Hyprland's own
// `position = auto` would put a first monitor, so the layout can be compared
// against what Hyprland does on its own.
func (m *Model) moveSelectedOutputToOrigin() {
	if len(m.editOutputs) == 0 || !m.canMoveSelectedOutput() {
		return
	}
	if m.editOutputs[m.selectedOutput].X == 0 && m.editOutputs[m.selectedOutput].Y == 0 {
		return
	}
	m.placeSelected(0, 0, 0)
}

// canMoveSelectedOutput reports whether moving the selection means anything. A
// mirroring display shows its source's image wherever Hyprland decides to put
// it, so accepting a move would only leave the draft permanently different from
// what any apply can produce.
func (m *Model) canMoveSelectedOutput() bool {
	output := m.editOutputs[m.selectedOutput]
	if output.MirrorOf == "" {
		return true
	}
	m.setStatusErr(fmt.Sprintf("%s mirrors %s and follows it; move %s instead",
		output.Name, m.outputNameForKey(output.MirrorOf), m.outputNameForKey(output.MirrorOf)))
	return false
}

// moveSelectedOutput nudges the selected display. Like the panel, a nudge
// that would cover another display is refused and says why.
func (m *Model) moveSelectedOutput(dx, dy int) {
	if len(m.editOutputs) == 0 || !m.canMoveSelectedOutput() {
		return
	}
	output := m.editOutputs[m.selectedOutput]
	m.placeSelected(output.X+dx, output.Y+dy, 0)
}

func (m *Model) toggleSelectedOutput() {
	if len(m.editOutputs) == 0 {
		return
	}
	m.guardLayoutEdit(func() {
		m.editOutputs[m.selectedOutput].Enabled = !m.editOutputs[m.selectedOutput].Enabled
	})
}

func (m Model) analyzeSelectedSnap(threshold int) snapAnalysis {
	shared := profile.AnalyzeSnap(m.currentProfileOutputs(), m.selectedOutput, threshold)
	convertMarks := func(marks []profile.SnapMark) []snapMark {
		converted := make([]snapMark, 0, len(marks))
		for _, mark := range marks {
			converted = append(converted, snapMark{OutputIndex: mark.OutputIndex, Edge: snapEdge(mark.Edge)})
		}
		return converted
	}
	return snapAnalysis{
		x: snapAxisCandidate{pos: shared.X.Pos, dist: shared.X.Dist, marks: convertMarks(shared.X.Marks)},
		y: snapAxisCandidate{pos: shared.Y.Pos, dist: shared.Y.Dist, marks: convertMarks(shared.Y.Marks)},
	}
}

func (m Model) previewSelectedSnap(threshold int) *snapHintState {
	analysis := m.analyzeSelectedSnap(threshold)
	var marks []snapMark
	if analysis.x.dist <= threshold {
		marks = append(marks, analysis.x.marks...)
	}
	if analysis.y.dist <= threshold {
		marks = append(marks, analysis.y.marks...)
	}
	if len(marks) == 0 {
		return nil
	}
	return &snapHintState{Marks: marks}
}

// snapSelectedOutput is Place beside: flush against the nearest display on
// one side, centred on the other axis, through the shared placement rules.
func (m *Model) snapSelectedOutput(direction snapDirection) tea.Cmd {
	if len(m.editOutputs) == 0 || m.selectedOutput < 0 || m.selectedOutput >= len(m.editOutputs) {
		return nil
	}

	selected := m.editOutputs[m.selectedOutput]
	if !selected.Enabled || selected.MirrorOf != "" {
		m.setStatusErr("Selected monitor must be enabled and not mirrored to snap")
		return nil
	}

	x, y, anchorIndex, ok := profile.BesidePosition(m.currentProfileOutputs(), m.selectedOutput, direction.place())
	if !ok {
		m.setStatusErr("No other enabled monitor available for snapping")
		return nil
	}
	if _, placed := m.placeSelected(x, y, 0); !placed {
		return nil
	}

	var marks []snapMark
	switch direction {
	case snapDirectionLeft:
		marks = []snapMark{{m.selectedOutput, snapEdgeRight}, {anchorIndex, snapEdgeLeft}}
	case snapDirectionRight:
		marks = []snapMark{{m.selectedOutput, snapEdgeLeft}, {anchorIndex, snapEdgeRight}}
	case snapDirectionUp:
		marks = []snapMark{{m.selectedOutput, snapEdgeBottom}, {anchorIndex, snapEdgeTop}}
	case snapDirectionDown:
		marks = []snapMark{{m.selectedOutput, snapEdgeTop}, {anchorIndex, snapEdgeBottom}}
	}
	m.setStatusOK(fmt.Sprintf("Snapped %s %s %s", selected.Name, direction.relation(), m.editOutputs[anchorIndex].Name))
	return m.showSnapHint(&snapHintState{Marks: marks})
}

func (d snapDirection) place() profile.PlaceDirection {
	switch d {
	case snapDirectionLeft:
		return profile.PlaceLeft
	case snapDirectionRight:
		return profile.PlaceRight
	case snapDirectionUp:
		return profile.PlaceAbove
	default:
		return profile.PlaceBelow
	}
}

func (d snapDirection) relation() string {
	switch d {
	case snapDirectionLeft:
		return "left of"
	case snapDirectionRight:
		return "right of"
	case snapDirectionUp:
		return "above"
	case snapDirectionDown:
		return "below"
	default:
		return ""
	}
}

// reflowAfterResize keeps a layout packed when an output's logical size
// changes. Scale, mode, and transform all move an output's right and bottom
// edges while its top-left corner stays put, so without this the displays
// beside it gap or overlap. Everything past the old right or bottom edge
// moves with that edge: flush neighbors stay flush, deliberate gaps keep
// their width, and a row of displays shifts together.
func (m *Model) reflowAfterResize(index, oldWidth, oldHeight int) {
	outputs := m.currentProfileOutputs()
	profile.ReflowAfterResize(outputs, index, oldWidth, oldHeight)
	for idx := range outputs {
		m.editOutputs[idx].X = outputs[idx].X
		m.editOutputs[idx].Y = outputs[idx].Y
	}
}

func (m *Model) adjustInspectorField(delta int) {
	if len(m.editOutputs) == 0 {
		return
	}
	switch m.inspectorField {
	case 7, 8:
		if !m.canMoveSelectedOutput() {
			return
		}
		output := m.editOutputs[m.selectedOutput]
		if m.inspectorField == 7 {
			m.placeSelected(output.X+delta*10, output.Y, 0)
		} else {
			m.placeSelected(output.X, output.Y+delta*10, 0)
		}
		return
	}
	// Choice rows step like Omarchy's pill rows: arrows and h/l stop at the
	// first and last option; Enter advances and wraps.
	if values := inspectorChoiceValues(m.inspectorField); len(values) > 0 {
		current := m.layoutFieldValue(m.editOutputs[m.selectedOutput], m.inspectorField)
		pos := 0
		for idx, value := range values {
			if value == current {
				pos = idx
				break
			}
		}
		m.setInspectorChoice(m.inspectorField, values[clampInt(pos+delta, 0, len(values)-1)])
		return
	}
	m.guardLayoutEdit(func() { m.adjustInspectorFieldUnguarded(delta) })
}

func (m *Model) adjustInspectorFieldUnguarded(delta int) {
	output := &m.editOutputs[m.selectedOutput]
	oldWidth, oldHeight := output.logicalSize()
	switch m.inspectorField {
	case 0:
		output.Enabled = !output.Enabled
	case 1:
		if len(output.Modes) == 0 {
			return
		}
		output.ModeIndex = wrapIndex(output.ModeIndex+delta, len(output.Modes))
		if output.ModeUnsupported && output.ModeIndex > 0 {
			output.ModeUnsupported = false
		}
		output.applyMode(output.Modes[output.ModeIndex])
	case 2:
		output.Scale = nextSharpScale(output.Width, output.Height, output.Scale, delta)
	case 3:
		// Hyprland's bitdepth is a boolean in disguise: its parser only asks
		// whether the value is "10", so anything else means 10-bit off. There is
		// no 16-bit path to offer, and offering one would silently hand back 8.
		depths := []int{8, 10}
		current := 0
		for i, d := range depths {
			if d == output.Bitdepth {
				current = i
				break
			}
		}
		output.Bitdepth = depths[wrapIndex(current+delta, len(depths))]
	case 4:
		presets := []string{"srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid"}
		current := 0
		for i, p := range presets {
			if p == output.CM {
				current = i
				break
			}
		}
		output.CM = presets[wrapIndex(current+delta, len(presets))]
	case 5:
		output.VRR = wrapValue(output.VRR+delta, 0, 2)
	case 6:
		output.Transform = wrapValue(output.Transform+delta, 0, 7)
	case 9:
		targets := []string{""}
		for i, other := range m.editOutputs {
			if i != m.selectedOutput {
				targets = append(targets, other.Key)
			}
		}
		current := 0
		for i, t := range targets {
			if t == output.MirrorOf {
				current = i
				break
			}
		}
		output.MirrorOf = targets[wrapIndex(current+delta, len(targets))]
	case 10:
		output.SDRBrightness = clampFloat(sdrMultiplier(output.SDRBrightness)+float64(delta)*0.05, 0, 3.0)
	case 11:
		output.SDRSaturation = clampFloat(sdrMultiplier(output.SDRSaturation)+float64(delta)*0.05, 0, 3.0)
	case 12:
		output.SDRMinLuminance = clampFloat(output.SDRMinLuminance+float64(delta)*0.005, 0, 1.0)
	case 13:
		output.SDRMaxLuminance = clampInt(output.SDRMaxLuminance+delta*10, 0, 1000)
	case 14:
		eotfs := []string{"default", "gamma22", "srgb"}
		cur := 0
		for i, e := range eotfs {
			if e == output.SDREOTF {
				cur = i
				break
			}
		}
		output.SDREOTF = eotfs[wrapIndex(cur+delta, len(eotfs))]
	case 15:
		output.MinLuminance = clampFloat(output.MinLuminance+float64(delta)*0.001, 0, 1000.0)
	case 16:
		output.MaxLuminance = clampInt(output.MaxLuminance+delta*10, 0, 2000)
	case 17:
		output.MaxAvgLuminance = clampInt(output.MaxAvgLuminance+delta*10, 0, 2000)
	case 18:
		vals := []int{-1, 0, 1}
		cur := 1
		for i, v := range vals {
			if v == output.SupportsWideColor {
				cur = i
				break
			}
		}
		output.SupportsWideColor = vals[wrapIndex(cur+delta, len(vals))]
	case 19:
		vals := []int{-1, 0, 1}
		cur := 1
		for i, v := range vals {
			if v == output.SupportsHDR {
				cur = i
				break
			}
		}
		output.SupportsHDR = vals[wrapIndex(cur+delta, len(vals))]
	case 20:
		// ICC uses text input via activateInspectorField
	}
	m.reflowAfterResize(m.selectedOutput, oldWidth, oldHeight)
	m.layoutChanged()
}
