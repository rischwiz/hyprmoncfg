package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/lid"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func (m Model) renderLayoutView(height int) string {
	g := m.layoutGeometry(hitRect{w: m.footerContentWidth(), h: height})
	canvas := m.renderCanvasPane(g.stage.w, g.stage.h)
	left := canvas
	if g.hardware.h > 0 {
		left = lipgloss.JoinVertical(lipgloss.Left, canvas, m.renderHardwarePane(g.hardware.w, g.hardware.h))
	}
	inspector := m.renderInspectorPane(g.inspector.w, g.inspector.h, g.compact)
	if g.compact {
		return lipgloss.JoinVertical(lipgloss.Left, left, inspector)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGapWidth), inspector)
}

// layoutGeometry is the single source of truth for where the layout tab's
// panes sit, shared by rendering and pointer hit-testing. The stage and the
// hardware facts for the pictured display share one column, as in the panel;
// the Display and Color controls own the other. Stacked on small terminals,
// the stage takes only the rows the arrangement needs.
type layoutPanes struct {
	stage, hardware, inspector hitRect
	compact                    bool
}

func (m Model) layoutGeometry(body hitRect) layoutPanes {
	if m.useCompactLayout(body.h) {
		hardware := m.hardwarePaneHeight(body.w)
		frame := m.styles.inactivePane.GetVerticalFrameSize()
		const inspectorMin = 6
		stage := clampInt(m.stageFitRows(body.w)+frame, 5, max(5, body.h-hardware-inspectorMin))
		if body.h-stage-hardware < 3 {
			// Too short for everything: keep a small stage and the controls,
			// and let the hardware box give up rows first.
			stage = clampInt(body.h/3, 3, stage)
			hardware = clampInt(body.h-stage-3, 0, hardware)
		}
		inspector := max(1, body.h-stage-hardware)
		return layoutPanes{
			stage:     hitRect{x: body.x, y: body.y, w: body.w, h: stage},
			hardware:  hitRect{x: body.x, y: body.y + stage, w: body.w, h: hardware},
			inspector: hitRect{x: body.x, y: body.y + stage + hardware, w: body.w, h: inspector},
			compact:   true,
		}
	}

	canvasWidth, inspectorWidth := m.layoutPaneWidths()
	hardware := min(m.hardwarePaneHeight(canvasWidth), max(0, body.h-8))
	return layoutPanes{
		stage:     hitRect{x: body.x, y: body.y, w: canvasWidth, h: body.h - hardware},
		hardware:  hitRect{x: body.x, y: body.y + body.h - hardware, w: canvasWidth, h: hardware},
		inspector: hitRect{x: body.x + canvasWidth + paneGapWidth, y: body.y, w: inspectorWidth, h: body.h},
	}
}

// stageFitRows is how many canvas rows the arrangement needs at this pane
// width: the width-limited scale, the stage margin, and the rows for displays
// drawn outside the geometry.
func (m Model) stageFitRows(paneWidth int) int {
	inner := max(1, paneWidth-m.styles.inactivePane.GetHorizontalFrameSize())
	rows := len(m.hiddenDisplayRows(max(1, inner-4), 99))
	return stageRowsFor(m.editOutputs, inner) + rows
}

// stageRowsFor mirrors canvasLayoutFor: the height at which the arrangement's
// width, not its height, limits the scale.
func stageRowsFor(outputs []editableOutput, width int) int {
	const cellW = 2.2
	canvasW := max(20, width-2)
	minX, minY, maxX, maxY, ok := 0, 0, 0, 0, false
	for _, output := range outputs {
		if !output.spatial() {
			continue
		}
		w, h := output.logicalSize()
		if !ok {
			minX, minY, maxX, maxY, ok = output.X, output.Y, output.X+w, output.Y+h, true
			continue
		}
		minX, minY = min(minX, output.X), min(minY, output.Y)
		maxX, maxY = max(maxX, output.X+w), max(maxY, output.Y+h)
	}
	if !ok {
		return 3
	}
	scale := float64(canvasW-4) / (float64(max(1, maxX-minX)) * cellW)
	return int(math.Ceil(float64(max(1, maxY-minY))*scale)) + 4
}

func (m Model) renderCanvasPane(width int, height int) string {
	tone := paneToneIdle
	if m.layoutFocus == layoutFocusCanvas && m.tab == tabLayout {
		tone = paneToneFocused
	}
	panel := m.paneStyle(tone)
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())
	body := padBlock(m.renderCanvas(innerWidth, innerHeight), innerWidth, innerHeight)
	return m.renderTitledPaneWithMeta(tone, "Monitor Layout", m.canvasPaneMeta(), body, width)
}

func (m Model) canvasPaneMeta() string {
	switch m.lidState {
	case lid.Open:
		return "Lid: open"
	case lid.Closed:
		return "Lid: closed"
	default:
		return ""
	}
}

func (m Model) renderCanvas(width, height int) string {
	if len(m.editOutputs) == 0 {
		return "(no monitors)"
	}
	if height <= 2 {
		selected := m.editOutputs[m.selectedOutput]
		lines := []string{fitString(selected.Name, width)}
		if height == 2 {
			lines = append(lines, fitString(selected.DisplayMode(), width))
		}
		return strings.Join(lines, "\n")
	}

	layout := m.canvasLayout(width, height)
	if !layout.ok && len(m.hiddenDisplayRows(width, height)) == 0 {
		if m.hasMirroredOutputs() {
			return "(mirrors shown below)"
		}
		return "(all monitors disabled)"
	}

	canvasW := layout.width
	canvasH := layout.height

	grid := m.newCanvasCells(canvasW, canvasH)
	workspaces := workspacePlanByConnector(profile.ResolveWorkspaceRules(m.currentProfile("draft"), nil))

	rects := append([]canvasRect(nil), layout.rects...)
	sort.SliceStable(rects, func(i, j int) bool {
		if rects[i].index == m.selectedOutput {
			return false
		}
		if rects[j].index == m.selectedOutput {
			return true
		}
		return rects[i].index < rects[j].index
	})

	for _, rect := range rects {
		output := m.editOutputs[rect.index]
		selected := rect.index == m.selectedOutput
		issue, _ := m.canvasOutputIssue(output)
		colors := m.canvasCardStyle(output, selected)
		paintCard(grid, rect, selected, colors, func(maxLines, maxWidth int) []cardLine {
			return m.monitorCardLines(output, workspaces[output.Name], monitorCardLayout,
				maxLines, maxWidth, colors, issue, m.styles.palette.warning)
		})
	}
	if m.snap != nil {
		for _, mark := range m.snap.Marks {
			for _, rect := range layout.rects {
				if rect.index == mark.OutputIndex {
					paintSnapMark(grid, rect, mark.Edge, m.styles.palette.snapHighlight)
				}
			}
		}
	}
	m.paintHiddenDisplays(grid)
	return renderCanvasCells(grid)
}

// inspectorLayout is the single source of truth shared by renderInspectorPane
// and inspectorFieldAt so their row math cannot drift.
type inspectorLayout struct {
	lines     []string
	fieldRows map[int]int // field index → index into lines
	// fieldLines is how many lines a field takes; only a wrapped Scale row
	// takes more than one.
	fieldLines map[int]int
	// choices holds each choice row's option spans, in columns from the
	// start of its line, for pointer selection.
	choices map[int][]choiceSpan
}

func (m Model) buildInspectorLayout(output editableOutput, innerWidth int, compact bool) inspectorLayout {
	lines := make([]string, 0, len(layoutFields)+2)

	labelWidth := 0
	shortLabels := compact || innerWidth < 34 || (m.inspectorTab == inspectorTabColor && innerWidth < 50)
	for _, field := range inspectorFieldsForTab(m.inspectorTab) {
		label := layoutFields[field]
		if shortLabels {
			label = layoutFieldShortLabel(field)
		}
		labelWidth = max(labelWidth, lipgloss.Width(label))
	}

	fieldRows := make(map[int]int, len(layoutFields))
	fieldLines := make(map[int]int, len(layoutFields))
	choices := make(map[int][]choiceSpan)
	for _, idx := range inspectorFieldsForTab(m.inspectorTab) {
		if idx == advancedFieldStart {
			lines = append(lines, "")
		}
		labelText := layoutFields[idx]
		if shortLabels {
			labelText = layoutFieldShortLabel(idx)
		}
		label := m.styles.label.Render(fmt.Sprintf("%-*s", labelWidth, labelText))
		focused := m.layoutFocus == layoutFocusInspector && idx == m.inspectorField && m.tab == tabLayout
		raw := m.layoutFieldValue(output, idx)
		issue, hasIssue := m.layoutFieldIssue(output, idx)
		fieldRows[idx] = len(lines)
		fieldLines[idx] = 1

		if idx == 2 {
			rows, spans := m.renderScaleRow(output, innerWidth-labelWidth-1, focused, !compact)
			if len(rows) > 0 {
				for i := range spans {
					spans[i].start += labelWidth + 1
					spans[i].end += labelWidth + 1
				}
				choices[idx] = spans
				indent := strings.Repeat(" ", labelWidth+1)
				for i, row := range rows {
					if i == 0 {
						lines = append(lines, label+" "+row)
					} else {
						lines = append(lines, indent+row)
					}
				}
				fieldLines[idx] = len(rows)
				continue
			}
		}

		if !hasIssue {
			if row, spans, ok := m.renderChoiceRow(idx, raw, innerWidth-labelWidth-1, focused); ok {
				for i := range spans {
					spans[i].start += labelWidth + 1
					spans[i].end += labelWidth + 1
				}
				choices[idx] = spans
				lines = append(lines, label+" "+row)
				continue
			}
		}

		if m.inlineEntryActive(idx) {
			lines = append(lines, label+" "+m.renderInlineEntry(innerWidth-labelWidth-1))
			continue
		}

		valueText := fieldOptionLabel(idx, raw)
		if idx == 2 {
			valueText += "x"
		}
		valueStyle := m.styles.value
		if hasIssue {
			valueStyle = m.styles.warning
		}
		if focused {
			valueStyle = m.styles.focused
			if hasIssue {
				valueStyle = withFG(valueStyle, m.styles.palette.warning)
			}
		}
		value := valueStyle.Render(valueText)
		if hasIssue {
			value = lipgloss.JoinHorizontal(lipgloss.Left, value, " ", m.styles.warning.Render("⚠ "+issue))
		}
		lines = append(lines, fmt.Sprintf("%s %s", label, value))
	}

	return inspectorLayout{lines: lines, fieldRows: fieldRows, fieldLines: fieldLines, choices: choices}
}

func inspectorFieldsForTab(tab inspectorTab) []int {
	if tab == inspectorTabColor {
		// The EDID action sits directly under the capability fields it fills.
		return []int{3, 4, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, edidDetectField, 20}
	}
	return []int{0, 1, 2, 5, 6, 7, 8, 9}
}

func (m *Model) normalizeInspectorField() {
	fields := inspectorFieldsForTab(m.inspectorTab)
	for _, field := range fields {
		if m.inspectorField == field {
			return
		}
	}
	if len(fields) > 0 {
		m.inspectorField = fields[0]
	}
}

func (m *Model) moveInspectorField(delta int) {
	fields := inspectorFieldsForTab(m.inspectorTab)
	if len(fields) == 0 {
		return
	}
	position := 0
	for idx, field := range fields {
		if field == m.inspectorField {
			position = idx
			break
		}
	}
	m.inspectorField = fields[clampIndex(position+delta, len(fields))]
}

// cycleLayoutPane walks the layout tab's three panes in the order they appear:
// the canvas, then Display, then Color. Tab moving between panes and the
// bracket keys always cycling monitors keeps each key to one meaning.
func (m *Model) cycleLayoutPane(delta int) {
	position := 0
	if m.layoutFocus == layoutFocusInspector {
		position = 1 + int(m.inspectorTab)
	}

	position = wrapIndex(position+delta, 3)
	if position == 0 {
		m.layoutFocus = layoutFocusCanvas
		return
	}
	m.layoutFocus = layoutFocusInspector
	m.inspectorTab = inspectorTab(position - 1)
	m.normalizeInspectorField()
}

func inspectorScrollOffset(totalLines, selectedLine, height int) int {
	if height <= 0 || selectedLine < height {
		return 0
	}
	offset := selectedLine - height + 1
	if offset < 0 {
		offset = 0
	}
	if offset >= totalLines {
		offset = totalLines - 1
	}
	return offset
}

func (m Model) renderInspectorPane(width int, height int, compact bool) string {
	tone := paneToneIdle
	if m.layoutFocus == layoutFocusInspector && m.tab == tabLayout {
		tone = paneToneFocused
	}
	panel := m.paneStyle(tone)
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())

	if len(m.editOutputs) == 0 {
		body := fitBlock("(none)", innerWidth, innerHeight)
		return m.renderInspectorTabbedPane(tone, body, width)
	}

	layout := m.buildInspectorLayout(m.editOutputs[m.selectedOutput], innerWidth, compact)
	lines := layout.lines

	if m.layoutFocus == layoutFocusInspector && m.tab == tabLayout {
		if row, ok := layout.fieldRows[m.inspectorField]; ok {
			// Keep every line of a wrapped field in view.
			row += max(1, layout.fieldLines[m.inspectorField]) - 1
			offset := inspectorScrollOffset(len(lines), row, innerHeight)
			lines = lines[offset:]
		}
	}

	body := fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight)
	return m.renderInspectorTabbedPane(tone, body, width)
}

// hardwarePaneHeight fits the six hardware facts: two columns of three rows
// when the pane is wide enough, one column of six otherwise.
func (m Model) hardwarePaneHeight(width int) int {
	inner := max(1, width-m.styles.staticPane.GetHorizontalFrameSize())
	return len(m.hardwareGridLines(inner)) + m.styles.staticPane.GetVerticalFrameSize()
}

func (m Model) renderHardwarePane(width, height int) string {
	panel := m.paneStyle(paneToneStatic)
	innerWidth := max(1, width-panel.GetHorizontalFrameSize())
	innerHeight := max(1, height-panel.GetVerticalFrameSize())
	body := fitBlock(strings.Join(m.hardwareGridLines(innerWidth), "\n"), innerWidth, innerHeight)
	return m.renderTitledPane(paneToneStatic, "Hardware", body, width)
}

// hardwareGridLines lays the selected display's hardware facts out in two
// columns when they fit, so the stage keeps the rows.
func (m Model) hardwareGridLines(width int) []string {
	if len(m.editOutputs) == 0 {
		return []string{m.styles.subtle.Render("(none)")}
	}
	rows := m.hardwareDetailRows(m.editOutputs[m.selectedOutput])
	labelWidth := 0
	valueWidth := 0
	for _, row := range rows {
		labelWidth = max(labelWidth, lipgloss.Width(row.label))
		valueWidth = max(valueWidth, lipgloss.Width(row.value))
	}
	columnWidth := labelWidth + 1 + valueWidth
	if width < columnWidth*2+4 {
		return m.renderDetailRows(rows)
	}
	half := (len(rows) + 1) / 2
	lines := make([]string, 0, half)
	column := max(columnWidth+4, width/2)
	for idx := 0; idx < half; idx++ {
		cell := func(row detailRow) string {
			return m.styles.label.Render(fmt.Sprintf("%-*s", labelWidth, row.label)) + " " + m.styles.value.Render(row.value)
		}
		line := cell(rows[idx])
		if idx+half < len(rows) {
			line += strings.Repeat(" ", max(1, column-lipgloss.Width(line))) + cell(rows[idx+half])
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) renderInspectorTabbedPane(tone paneTone, body string, width int) string {
	title, titleWidth := m.renderInspectorTabs()
	return m.renderPaneWithTitle(tone, title, titleWidth, "", body, width)
}

// paneTone is how loud a pane's chrome should be. Only the pane your keys act
// on takes the accent; a pane that never takes focus still reads clearly; a
// pane you could switch to but have not stays muted.
type paneTone int

const (
	paneToneIdle paneTone = iota
	paneToneFocused
	paneToneStatic
)

func (m Model) paneStyle(tone paneTone) lipgloss.Style {
	switch tone {
	case paneToneFocused:
		return m.styles.activePane
	case paneToneStatic:
		return m.styles.staticPane
	default:
		return m.styles.inactivePane
	}
}

func (m Model) paneBorderColor(tone paneTone) string {
	switch tone {
	case paneToneFocused:
		return m.styles.palette.paneActiveBorder
	case paneToneStatic:
		return m.styles.palette.paneStaticBorder
	default:
		return m.styles.palette.paneBorder
	}
}

// renderTitledPane places the pane label inside its top border, leaving every
// interior row available to the editor. This mirrors the compact pane chrome
// used by terminal applications such as Lazygit.
func (m Model) renderTitledPane(tone paneTone, title, body string, width int) string {
	return m.renderTitledPaneWithMeta(tone, title, "", body, width)
}

func (m Model) renderTitledPaneWithMeta(tone paneTone, title, meta, body string, width int) string {
	styledTitle := withFG(lipgloss.NewStyle().Bold(true), m.paneBorderColor(tone)).Render(title)
	return m.renderPaneWithTitle(tone, styledTitle, lipgloss.Width(title), meta, body, width)
}

func (m Model) renderPaneWithTitle(tone paneTone, styledTitle string, titleWidth int, meta, body string, width int) string {
	panel := m.paneStyle(tone)
	rendered := panel.Width(styleRenderWidth(width, panel)).Render(body)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}

	labelWidth := titleWidth + 2
	topWidth := lipgloss.Width(lines[0])
	start := 2
	if topWidth <= start+labelWidth+1 {
		return rendered
	}

	styledLabel := " " + styledTitle + " "
	lines[0] = ansi.Cut(lines[0], 0, start) + styledLabel + ansi.Cut(lines[0], start+labelWidth, topWidth)

	if meta != "" && len(lines) > 1 {
		bottom := len(lines) - 1
		bottomWidth := lipgloss.Width(lines[bottom])
		metaLabel := " " + meta + " "
		metaWidth := lipgloss.Width(metaLabel)
		metaStart := bottomWidth - metaWidth - 1
		if metaStart > 1 {
			styledMeta := withFG(lipgloss.NewStyle(), m.paneBorderColor(tone)).Render(metaLabel)
			lines[bottom] = ansi.Cut(lines[bottom], 0, metaStart) + styledMeta + ansi.Cut(lines[bottom], metaStart+metaWidth, bottomWidth)
		}
	}
	return strings.Join(lines, "\n")
}

func scrollLinesToFit(lines []string, selectedLine, height int) []string {
	offset := inspectorScrollOffset(len(lines), selectedLine, height)
	return lines[offset:]
}
