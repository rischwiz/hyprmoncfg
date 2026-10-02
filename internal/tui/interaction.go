package tui

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type pickerItem string

func (i pickerItem) FilterValue() string { return string(i) }

func (i pickerItem) Title() string { return string(i) }

func (i pickerItem) Description() string { return "" }

func (i pickerItem) Value() string { return string(i) }

type fieldPickerItem struct {
	pickerItem
	label string
}

func (i fieldPickerItem) Title() string { return i.label }

type modePickerState struct {
	OutputIndex int
	FieldIndex  int // -1 for mode picker, >= 0 for field option picker
	List        list.Model
}

type numericInputKind int

const (
	numericInputScale numericInputKind = iota
	numericInputPositionX
	numericInputPositionY
	numericInputICC
	numericInputFloat
	numericInputInt
	numericInputWorkspaceCount
	numericInputWorkspaceGroupSize
)

type numericInputState struct {
	Kind        numericInputKind
	OutputIndex int
	FieldIndex  int
	Title       string
	Hint        string
	Input       textinput.Model
}

type profileExecInputState struct {
	ProfileIndex int
	Title        string
	Input        textinput.Model
	Err          error
}

type profileListItem struct {
	name    string
	updated time.Time
	outputs int
}

func (i profileListItem) FilterValue() string { return i.name }

func (i profileListItem) Title() string { return i.name }

func (i profileListItem) Description() string {
	if i.updated.IsZero() {
		return fmt.Sprintf("%d outputs", i.outputs)
	}
	return fmt.Sprintf("updated %s  •  %d outputs", i.updated.Local().Format("2006-01-02 15:04"), i.outputs)
}

// arrowDelegate wraps list.DefaultDelegate and prepends a ▸ arrow on the selected item.
type arrowDelegate struct {
	list.DefaultDelegate
}

func (d arrowDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var buf strings.Builder
	d.DefaultDelegate.Render(&buf, m, index, item)
	isSelected := index == m.Index()
	for i, line := range strings.Split(buf.String(), "\n") {
		if i > 0 {
			fmt.Fprint(w, "\n")
		}
		if i == 0 && isSelected {
			fmt.Fprintf(w, "▸ %s", line)
		} else {
			fmt.Fprintf(w, "  %s", line)
		}
	}
}

// plainDelegate indents list rows like arrowDelegate without drawing an arrow,
// so both lists keep the same columns.
type plainDelegate struct {
	list.DefaultDelegate
}

func (d plainDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var buf strings.Builder
	d.DefaultDelegate.Render(&buf, m, index, item)
	for i, line := range strings.Split(buf.String(), "\n") {
		if i > 0 {
			fmt.Fprint(w, "\n")
		}
		fmt.Fprintf(w, "  %s", line)
	}
}

type saveDialogState struct {
	Input   textinput.Model
	List    list.Model
	All     []profileListItem
	Filter  string
	Action  saveAction
	Purpose saveDialogPurpose
}

type saveAction int

const (
	saveActionOnly saveAction = iota
	saveActionApply
	saveActionSaveQuit
	saveActionDiscardQuit
	saveActionCancel
)

type saveDialogPurpose int

const (
	saveDialogProfile saveDialogPurpose = iota
	saveDialogQuit
)

type canvasRect struct {
	index int
	x     int
	y     int
	w     int
	h     int
}

type canvasGeometry struct {
	// originX, originY are the logical coordinates drawn at offsetX, offsetY.
	originX int
	originY int
	ok      bool
	width   int
	height  int
	scale   float64
	cellW   float64
	offsetX int
	offsetY int
	rects   []canvasRect
}

type hitRect struct {
	x int
	y int
	w int
	h int
}

func (r hitRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (r hitRect) inner(style lipgloss.Style) hitRect {
	return hitRect{
		x: r.x + style.GetBorderLeftSize() + style.GetPaddingLeft(),
		y: r.y + style.GetBorderTopSize() + style.GetPaddingTop(),
		w: max(1, r.w-style.GetHorizontalFrameSize()),
		h: max(1, r.h-style.GetVerticalFrameSize()),
	}
}

func (m *Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSave:
		return m.updateSaveMouse(msg)
	case modeModePicker:
		return m.updateModePickerMouse(msg)
	case modeConfirm:
		return m.updateConfirmMouse(msg)
	case modeDeleteConfirm:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			switch {
			case m.visibleActionAt(msg.X, msg.Y, deleteConfirmLabel):
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
			case m.visibleActionAt(msg.X, msg.Y, deleteCancelLabel):
				return m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			}
		}
		return m, nil
	case modeProfileMenu:
		return m.updateProfileMenuMouse(msg)
	case modeNumericInput, modeProfileExecInput, modeProfileNameInput, modeSaveConfirm:
		return m, nil
	}

	if msg.Action == tea.MouseActionRelease {
		if m.drag != nil {
			return m, m.dragRelease(msg.X, msg.Y)
		}
		return m, nil
	}
	// Motion during a drag touches nothing but the dragged position: no hit
	// testing, layout, or validation per event.
	if msg.Action == tea.MouseActionMotion && m.drag != nil {
		m.dragMotion(msg.X, msg.Y)
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.Y == m.appContentY() {
		plainTabs := ansi.Strip(m.renderTabs())
		localX := msg.X - m.appContentX()
		if start, ok := visibleTextColumn(plainTabs, "Daemon not running"); ok {
			if localX >= start && localX < start+lipgloss.Width("Daemon not running") {
				return m, m.openURLCmd("Daemon not running", daemonURL)
			}
		}
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.restartHintAt(msg.X, msg.Y) {
		return m, m.restartDaemonCmd()
	}

	if tab, ok := m.tabAt(msg.X, msg.Y); ok && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		m.tab = tab
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if link, ok := m.footerLinkAt(msg.X, msg.Y); ok {
			return m, m.openURLCmd(link.label, link.url)
		}
	}

	switch m.tab {
	case tabLayout:
		return m.updateLayoutMouse(msg)
	case tabProfiles:
		return m.updateProfilesMouse(msg)
	case tabWorkspaces:
		return m.updateWorkspaceMouse(msg)
	default:
		return m, nil
	}
}

func (m Model) updateSaveMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.saveDialog == nil {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if index, ok := m.saveDialogItemIndexAt(msg.X, msg.Y); ok {
		m.saveDialog.List.Select(index)
		m.syncSaveNameFromSelection()
	}
	return m, nil
}

func visibleTextColumn(line, label string) (int, bool) {
	index := strings.Index(line, label)
	if index < 0 {
		return 0, false
	}
	return lipgloss.Width(line[:index]), true
}

func (m *Model) updateModePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if index, ok := m.modePickerItemIndexAt(msg.X, msg.Y); ok {
		m.picker.List.Select(index)
		return m, m.commitModePicker()
	}

	return m, nil
}

func (m *Model) updateLayoutMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	canvasRect, _ := m.layoutCanvasRect()
	inspectorRect, compact := m.layoutInspectorRect()
	layout := m.canvasLayout(canvasRect.w-m.styles.inactivePane.GetHorizontalFrameSize(), m.canvasMouseHeight())

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.paneTitleContains(msg.X, msg.Y, canvasRect, "Monitor Layout") {
		m.layoutFocus = layoutFocusCanvas
		return m, nil
	}

	if m.inCanvas(msg.X, msg.Y, canvasRect, layout) {
		m.layoutFocus = layoutFocusCanvas
		localX, localY := m.canvasLocalPoint(msg.X, msg.Y, canvasRect)
		rows := m.hiddenDisplayRows(layout.width-2, layout.height)
		if localY >= 0 && localY < len(rows) {
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				row := rows[localY]
				m.selectedOutput = row.index
				m.drag = nil
				if row.action != "" && localX >= row.actionX+1 && localX < row.actionX+1+len(row.action) {
					m.toggleSelectedOutput()
				} else {
					m.inspectorTab = inspectorTabDisplay
					m.inspectorField = 0
				}
			}
			return m, nil
		}
		if rect, ok := layout.rectAt(localX, localY); ok {
			m.selectedOutput = rect.index
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				m.snap = nil
				output := m.editOutputs[rect.index]
				if output.MirrorOf == "" {
					m.drag = &canvasDragState{OutputIndex: rect.index, StartX: msg.X, StartY: msg.Y,
						OrigX: output.X, OrigY: output.Y, Geometry: layout}
				}
			}
		}
		return m, nil
	}

	if inspectorRect.contains(msg.X, msg.Y) {
		wasFocused := m.layoutFocus == layoutFocusInspector && m.tab == tabLayout
		m.layoutFocus = layoutFocusInspector
		if tab, ok := m.inspectorTabAt(msg.X, msg.Y, inspectorRect); ok && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.inspectorTab = tab
			m.normalizeInspectorField()
			return m, nil
		}
		if field, line, ok := m.inspectorFieldLineAt(msg.Y, inspectorRect, compact, wasFocused); ok && msg.Action == tea.MouseActionPress {
			m.inspectorField = field
			switch msg.Button {
			case tea.MouseButtonLeft:
				if value, ok := m.inspectorChoiceAt(msg.X, field, line, inspectorRect, compact); ok {
					if field == 2 && value == scaleMoreValue {
						m.openScalePicker()
						return m, nil
					}
					m.setInspectorChoice(field, value)
					return m, nil
				}
				if len(inspectorChoiceValues(field)) > 0 {
					return m, nil
				}
				return m, m.activateInspectorField()
			case tea.MouseButtonWheelUp:
				m.adjustInspectorField(1)
			case tea.MouseButtonWheelDown:
				m.adjustInspectorField(-1)
			}
		}
	}

	return m, nil
}

func (m Model) inspectorTabAt(x, y int, inspectorRect hitRect) (inspectorTab, bool) {
	if y != inspectorRect.y {
		return inspectorTabDisplay, false
	}
	localX := x - inspectorRect.x
	cursor := 3 // rounded corner, border segment, and title padding
	for idx, label := range inspectorTabLabels {
		width := lipgloss.Width(label) + 2
		if localX >= cursor && localX < cursor+width {
			return inspectorTab(idx), true
		}
		cursor += width
	}
	return inspectorTabDisplay, false
}

// inspectorChoiceAt finds the option of a choice row under the pointer.
func (m Model) inspectorChoiceAt(x, field, line int, inspectorRect hitRect, compact bool) (string, bool) {
	if len(m.editOutputs) == 0 {
		return "", false
	}
	inner := inspectorRect.inner(m.styles.inactivePane)
	layout := m.buildInspectorLayout(m.editOutputs[m.selectedOutput], inner.w, compact)
	localX := x - inner.x
	for _, span := range layout.choices[field] {
		if span.line == line && localX >= span.start && localX < span.end {
			return span.value, true
		}
	}
	return "", false
}

func (m Model) paneTitleContains(x, y int, pane hitRect, title string) bool {
	if y != pane.y {
		return false
	}
	start := pane.x + 3
	return x >= start && x < start+lipgloss.Width(title)
}

func (m Model) updateProfilesMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// A right click on a profile opens its action menu, as in the panel.
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight {
		if row, ok := m.profileRowAt(msg.X, msg.Y); ok {
			m.selectedProfile = row
			m.openProfileMenu()
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	listRect := m.profilesListRect()
	automaticRect := m.profileAutomaticRect()
	if automaticRect.inner(m.styles.inactivePane).contains(msg.X, msg.Y) {
		return m.toggleProfileAutomatic()
	}
	for _, action := range []struct{ label, key string }{{"[Preview]", "enter"}, {"[Edit]", "l"}, {"[Delete]", "d"}, {"[Rename]", "n"}, {"[Duplicate]", "c"}} {
		if len(m.profiles) > 0 && m.visibleActionAt(msg.X, msg.Y, action.label) {
			if action.key == "enter" {
				return m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyEnter})
			}
			return m.updateProfileKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(action.key)})
		}
	}
	if !listRect.contains(msg.X, msg.Y) {
		if m.visibleActionAt(msg.X, msg.Y, "Post-apply command") || m.visibleActionAt(msg.X, msg.Y, "[Edit command]") {
			return m, m.openProfileExecInput()
		}
		return m, nil
	}

	if row, ok := m.profileRowAt(msg.X, msg.Y); ok {
		m.selectedProfile = row
	}
	return m, nil
}

// profileRowAt is the profile whose list row is under the pointer.
func (m Model) profileRowAt(x, y int) (int, bool) {
	listRect := m.profilesListRect()
	if !listRect.contains(x, y) {
		return 0, false
	}
	inner := listRect.inner(m.styles.activePane)
	row := y - inner.y - profileListHeaderRows + m.profileListScroll(inner.h)
	if row < 0 || row >= len(m.profiles) || y < inner.y+profileListHeaderRows || y >= inner.y+inner.h-m.profileListActionRows() {
		return 0, false
	}
	return row, true
}

func (m Model) updateWorkspaceMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	settingsRect := m.workspaceSettingsRect()
	if !settingsRect.contains(msg.X, msg.Y) {
		return m, nil
	}

	inner := settingsRect.inner(m.styles.activePane)
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.moveWorkspaceSelection(-3, false)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.moveWorkspaceSelection(3, false)
			return m, nil
		}
	}

	scrollOffset := m.workspaceSettingsScrollOffset(inner.h)
	fieldRow := msg.Y - inner.y + scrollOffset
	if fieldRow >= 0 && fieldRow < len(workspaceFields) && msg.Action == tea.MouseActionPress {
		m.workspaceEdit.SelectedField = fieldRow
		switch msg.Button {
		case tea.MouseButtonLeft:
			m.adjustWorkspaceField(1)
			m.markDirty()
		}
		return m, nil
	}

	// Strategy-specific items start after the fields, a blank, and their header.
	orderStart := inner.y + len(workspaceFields) + 2
	visualY := msg.Y + scrollOffset
	if msg.Action == tea.MouseActionPress && visualY >= orderStart {
		row := visualY - orderStart
		if row >= 0 && row < m.workspaceListItemCount() {
			m.workspaceEdit.SelectedField = len(workspaceFields) + row
			m.workspaceEdit.SelectedOrder = row
		}
	}
	return m, nil
}

func (m Model) appContentX() int {
	return m.styles.app.GetPaddingLeft()
}

func (m Model) appContentY() int {
	return m.styles.app.GetPaddingTop()
}

func (m Model) bodyRect() hitRect {
	tabs := m.renderTabs()
	footer := m.renderFooterBar()
	return hitRect{
		x: m.appContentX(),
		y: m.appContentY() + lipgloss.Height(tabs),
		w: m.footerContentWidth(),
		h: m.mainBodyHeight(tabs, "", footer),
	}
}

func (m Model) layoutCanvasRect() (hitRect, bool) {
	g := m.layoutGeometry(m.bodyRect())
	return g.stage, g.compact
}

func (m Model) layoutInspectorRect() (hitRect, bool) {
	g := m.layoutGeometry(m.bodyRect())
	return g.inspector, g.compact
}

func (m Model) profileAutomaticRect() hitRect {
	body := m.bodyRect()
	body.h = profileAutomaticPaneHeight
	if m.terminalWidth() >= 96 {
		body.w, _ = m.sidePaneWidths(35)
	}
	return body
}

func (m Model) profilesListRect() hitRect {
	body := m.bodyRect()
	body.y += profileAutomaticPaneHeight
	body.h -= profileAutomaticPaneHeight
	if m.terminalWidth() < 96 {
		listHeight := m.compactProfileListHeight(body.h)
		return hitRect{x: body.x, y: body.y, w: body.w, h: listHeight}
	}

	listWidth, _ := m.sidePaneWidths(35)
	return hitRect{x: body.x, y: body.y, w: listWidth, h: body.h}
}

func (m Model) workspaceSettingsRect() hitRect {
	body := m.bodyRect()
	if m.terminalWidth() < 96 {
		settingsHeight := clampInt(m.workspaceSettingsLineCount()+2, 6, (body.h*2)/3)
		return hitRect{x: body.x, y: body.y, w: body.w, h: settingsHeight}
	}

	leftWidth, _ := m.sidePaneWidths(35)
	return hitRect{x: body.x, y: body.y, w: leftWidth, h: body.h}
}

func (m Model) workspaceSettingsLineCount() int {
	if !m.workspaceEdit.Enabled {
		return len(workspaceFields)
	}
	count := len(workspaceFields) + 2
	itemCount := m.workspaceListItemCount()
	if itemCount == 0 {
		count++
	} else {
		count += itemCount
	}
	return count
}

func (m Model) modePickerItemIndexAt(x, y int) (int, bool) {
	if m.picker == nil {
		return 0, false
	}

	items := m.picker.List.VisibleItems()
	start, end := m.picker.List.Paginator.GetSliceBounds(len(items))
	return visibleListItemIndexAt(m.View(), x, y, items, start, end)
}

func (m Model) saveDialogItemIndexAt(x, y int) (int, bool) {
	if m.saveDialog == nil {
		return 0, false
	}

	items := m.saveDialog.List.VisibleItems()
	start, end := m.saveDialog.List.Paginator.GetSliceBounds(len(items))
	return visibleListItemIndexAt(m.View(), x, y, items, start, end)
}

func visibleListItemIndexAt(view string, x, y int, items []list.Item, start, end int) (int, bool) {
	lines := strings.Split(ansi.Strip(view), "\n")
	if y < 0 || y >= len(lines) {
		return 0, false
	}
	line := lines[y]

	for index := start; index < end; index++ {
		for _, label := range visibleListItemLabels(items[index]) {
			col := strings.Index(line, label)
			if col < 0 {
				continue
			}
			labelStart := max(0, lipgloss.Width(line[:col])-2)
			labelEnd := lipgloss.Width(line[:col]) + lipgloss.Width(label)
			if x >= labelStart && x < labelEnd {
				return index, true
			}
		}
	}

	return 0, false
}

func visibleListItemLabels(item list.Item) []string {
	labels := []string{item.FilterValue()}

	if titled, ok := item.(interface{ Title() string }); ok {
		labels = append(labels, titled.Title())
	}
	if described, ok := item.(interface{ Description() string }); ok {
		if description := described.Description(); description != "" {
			labels = append(labels, description)
		}
	}

	return labels
}

func (m Model) canvasMouseHeight() int {
	panel := m.styles.inactivePane
	canvasRect, _ := m.layoutCanvasRect()
	innerHeight := max(1, canvasRect.h-panel.GetVerticalFrameSize())
	return innerHeight
}

// restartHintAt reports a click on the stale-daemon message, which sits at the
// right end of the tab row.
func (m Model) restartHintAt(x, y int) bool {
	if !m.daemonNeedsRestart() || y != m.appContentY() {
		return false
	}
	width := m.footerContentWidth()
	hint := lipgloss.Width(m.restartHint())
	localX := x - m.appContentX()
	return localX > width-hint-2 && localX <= width
}

func (m Model) tabAt(x, y int) (mainTab, bool) {
	tabY := m.appContentY()
	tabHeight := lipgloss.Height(m.renderTabs())
	if y < tabY || y >= tabY+tabHeight {
		return tabLayout, false
	}
	localX := x - m.appContentX()
	if localX < 0 {
		return tabLayout, false
	}

	labels := []string{"Layout", "Workspaces", "Profiles"}
	cursorX := 1
	for idx, label := range labels {
		width := lipgloss.Width(fmt.Sprintf(" %d %s ", idx+1, label))
		if localX >= cursorX && localX < cursorX+width {
			return mainTab(idx), true
		}
		cursorX += width + 1
	}
	return tabLayout, false
}

func (m Model) inCanvas(x, y int, canvasRect hitRect, layout canvasGeometry) bool {
	localX, localY := m.canvasLocalPoint(x, y, canvasRect)
	return localX >= 0 && localX < layout.width && localY >= 0 && localY < layout.height
}

func (m Model) canvasLocalPoint(x, y int, canvasRect hitRect) (int, int) {
	inner := canvasRect.inner(m.styles.inactivePane)
	canvasX := inner.x
	canvasY := inner.y
	return x - canvasX, y - canvasY
}

// inspectorFieldLineAt finds the field under a row and which of its lines
// the row is, since a wrapped Scale row spans several.
func (m Model) inspectorFieldLineAt(y int, inspectorRect hitRect, compact bool, wasFocused bool) (int, int, bool) {
	if len(m.editOutputs) == 0 {
		return 0, 0, false
	}
	inner := inspectorRect.inner(m.styles.inactivePane)
	localY := y - inner.y
	if localY < 0 || localY >= inner.h {
		return 0, 0, false
	}

	layout := m.buildInspectorLayout(m.editOutputs[m.selectedOutput], inner.w, compact)
	scrollOffset := 0
	if wasFocused {
		if row, ok := layout.fieldRows[m.inspectorField]; ok {
			row += max(1, layout.fieldLines[m.inspectorField]) - 1
			scrollOffset = inspectorScrollOffset(len(layout.lines), row, inner.h)
		}
	}

	for idx := range layoutFields {
		row, ok := layout.fieldRows[idx]
		if !ok {
			continue
		}
		line := localY - (row - scrollOffset)
		if line >= 0 && line < max(1, layout.fieldLines[idx]) {
			return idx, line, true
		}
	}
	return 0, 0, false
}

func (m Model) canvasLayout(width, height int) canvasGeometry {
	// A drag keeps the transform it started with, so the stage neither
	// rescales nor recentres under the pointer; the stage refits on drop.
	if m.drag != nil && m.drag.Geometry.ok && m.drag.Geometry.width == max(20, width-2) {
		layout := m.drag.Geometry
		layout.rects = layout.rectsFor(m.editOutputs)
		return layout
	}
	rows := len(m.hiddenDisplayRows(max(1, width-4), height))
	layout := canvasLayoutFor(m.editOutputs, width, max(3, height-rows))
	layout.height = max(3, height)
	layout.offsetY += rows
	for i := range layout.rects {
		layout.rects[i].y += rows
	}
	return layout
}

// canvasLayoutFor scales a set of outputs into a terminal-cell rectangle. The
// layout tab passes its editor outputs; the profile and workspace previews
// pass the outputs they want to show, so every canvas keeps the same geometry.
func canvasLayoutFor(outputs []editableOutput, width, height int) canvasGeometry {
	layout := canvasGeometry{
		width:  max(20, width-2),
		height: max(3, height),
		cellW:  2.2,
	}

	enabled := make([]editableOutput, 0, len(outputs))
	for _, output := range outputs {
		if output.spatial() {
			enabled = append(enabled, output)
		}
	}
	if len(enabled) == 0 {
		return layout
	}

	minX, minY := enabled[0].X, enabled[0].Y
	w0, h0 := enabled[0].logicalSize()
	maxX, maxY := enabled[0].X+w0, enabled[0].Y+h0
	for _, output := range enabled[1:] {
		w, h := output.logicalSize()
		minX = min(minX, output.X)
		minY = min(minY, output.Y)
		maxX = max(maxX, output.X+w)
		maxY = max(maxY, output.Y+h)
	}

	rangeW := max(1, maxX-minX)
	rangeH := max(1, maxY-minY)
	scaleX := float64(layout.width-4) / (float64(rangeW) * layout.cellW)
	// Short stages keep one row of margin instead of two, so the cards get
	// the rows for their workspaces.
	marginY := 4
	if layout.height < 12 {
		marginY = 2
	}
	scaleY := float64(layout.height-marginY) / float64(rangeH)
	layout.scale = math.Min(scaleX, scaleY)
	if layout.scale <= 0 {
		layout.scale = 1
	}
	contentW := int(math.Round(float64(rangeW) * layout.scale * layout.cellW))
	contentH := int(math.Round(float64(rangeH) * layout.scale))
	layout.offsetX = max(1, 1+(layout.width-2-contentW)/2)
	layout.offsetY = max(1, 1+(layout.height-2-contentH)/2)
	layout.ok = true
	layout.originX, layout.originY = minX, minY
	layout.rects = layout.rectsFor(outputs)
	return layout
}

// rectsFor places outputs with this transform. A frozen drag transform uses
// it too, so a card follows the pointer while the stage stays still.
func (g canvasGeometry) rectsFor(outputs []editableOutput) []canvasRect {
	rects := make([]canvasRect, 0, len(outputs))
	for idx, output := range outputs {
		if !output.spatial() {
			continue
		}
		w, h := output.logicalSize()
		rx := g.offsetX + int(math.Round(float64(output.X-g.originX)*g.scale*g.cellW))
		ry := g.offsetY + int(math.Round(float64(output.Y-g.originY)*g.scale))
		rw := max(8, int(math.Round(float64(w)*g.scale*g.cellW)))
		rh := max(3, int(math.Round(float64(h)*g.scale)))

		if rx+rw >= g.width {
			rw = max(4, g.width-rx-1)
		}
		if ry+rh >= g.height {
			rh = max(3, g.height-ry-1)
		}

		rects = append(rects, canvasRect{index: idx, x: rx, y: ry, w: rw, h: rh})
	}
	return rects
}

func (g canvasGeometry) rectAt(x, y int) (canvasRect, bool) {
	for _, rect := range g.rects {
		if x >= rect.x && x < rect.x+rect.w && y >= rect.y && y < rect.y+rect.h {
			return rect, true
		}
	}
	return canvasRect{}, false
}

func defaultHeight(height int) int {
	if height <= 0 {
		return 28
	}
	return height
}

func (m Model) modePickerHeight() int {
	return clampInt(defaultHeight(m.height)-14, 6, 10)
}

func (m Model) terminalWidth() int {
	if m.width <= 0 {
		return 100
	}
	return max(28, m.width)
}

func (m Model) terminalHeight() int {
	if m.height <= 0 {
		return 28
	}
	return max(12, m.height)
}

func (m Model) modalMaxWidth() int {
	return max(24, m.terminalWidth()-6)
}

func (m Model) modePickerWidth() int {
	return clampInt(m.modalMaxWidth()-6, 24, 44)
}

func (m Model) saveDialogInputWidth() int {
	return clampInt(m.modalMaxWidth()-18, 16, 28)
}

func (m Model) saveDialogListWidth() int {
	return clampInt(m.modalMaxWidth()-6, 24, 52)
}

// layoutPaneWidths gives the controls their natural width and the stage the
// rest, so a wide terminal widens the arrangement instead of the form.
func (m Model) layoutPaneWidths() (int, int) {
	total := m.terminalWidth()
	inspector := clampInt(total*34/100, 18, 58)
	left, right := splitPaneWidths(total, 100-(inspector*100+total-1)/total, 18)
	return left, right
}

func (m Model) sidePaneWidths(leftPercent int) (int, int) {
	return splitPaneWidths(m.terminalWidth(), leftPercent, 16)
}

const paneGapWidth = 0

func splitPaneWidths(total int, leftPercent int, minPane int) (int, int) {
	available := max(2, total-paneGapWidth)
	left := (available * leftPercent) / 100
	right := available - left
	if available >= minPane*2 {
		if left < minPane {
			left = minPane
			right = available - left
		}
		if right < minPane {
			right = minPane
			left = available - right
		}
	}
	return max(1, left), max(1, right)
}

// updateConfirmMouse gives Keep and Revert pointer parity with y and n.
func (m Model) updateConfirmMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || m.pending == nil {
		return m, nil
	}
	switch {
	case m.visibleActionAt(msg.X, msg.Y, confirmKeepLabel):
		return m.updateConfirmKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	case m.visibleActionAt(msg.X, msg.Y, confirmRevertLabel):
		return m.updateConfirmKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	}
	return m, nil
}

const (
	deleteCancelLabel  = "[Cancel]"
	deleteConfirmLabel = "[Delete profile]"
)
