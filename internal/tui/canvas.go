package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// newCanvasCells lays out the stage: a quiet dotted field the displays sit
// on. The dots mark space without competing with the cards or their text.
func (m Model) newCanvasCells(width, height int) [][]canvasCell {
	grid := make([][]canvasCell, height)
	p := m.styles.palette
	for y := 0; y < height; y++ {
		row := make([]canvasCell, width)
		for x := 0; x < width; x++ {
			cell := canvasCell{ch: ' ', fg: p.stageDot, bg: p.canvasBg}
			if y%3 == 1 && x%6 == 2 {
				cell.ch = '·'
			}
			row[x] = cell
		}
		grid[y] = row
	}
	return grid
}

func (m Model) canvasCardStyle(output editableOutput, selected bool) canvasCardColors {
	p := m.styles.palette
	colors := canvasCardColors{
		bg:     p.cardBg,
		border: p.cardBorder,
		fg:     p.cardFg,
		muted:  p.cardMuted,
	}
	if !output.Enabled {
		colors = canvasCardColors{
			bg:     p.cardDisabledBg,
			border: p.cardDisabledBorder,
			fg:     p.cardDisabledFg,
			muted:  p.cardDisabledMuted,
		}
	}
	if selected {
		colors = canvasCardColors{
			bg:     p.cardSelectedBg,
			border: p.cardSelectedBorder,
			fg:     p.cardSelectedFg,
			muted:  p.cardSelectedMuted,
		}
	}
	if _, ok := m.canvasOutputIssue(output); ok && !selected {
		colors.border = p.warning
	}
	if m.layoutErr != nil && m.isOutputOverlapping(output) && !selected {
		colors.border = "#FF0000"
		colors.fg = "#FF0000"
	}
	return colors
}

func (m Model) canvasOutputIssue(output editableOutput) (string, bool) {
	for idx := range layoutFields {
		if issue, ok := m.layoutFieldIssue(output, idx); ok {
			return issue, true
		}
	}
	if m.layoutErr != nil && m.isOutputOverlapping(output) {
		return "overlap", true
	}
	if m.liveWithoutMode(output.Name) {
		return "no usable signal", true
	}
	if fallback, ok := m.fallbacks[output.Name]; ok && output.Enabled {
		return "running " + fallback.Running, true
	}
	return "", false
}

// liveWithoutMode reports an output Hyprland has enabled but without a mode:
// it is connected and switched on, yet shows nothing.
func (m Model) liveWithoutMode(name string) bool {
	for _, live := range m.monitors {
		if live.Name == name {
			return !live.Disabled && (live.Width <= 0 || live.Height <= 0)
		}
	}
	return false
}

// paintCard draws one monitor as a screen on the stage. The connector and
// model sit at the top left with workspace chips on the right; the mode and
// placement sit at the bottom. The selected card takes a heavy border, so
// selection never depends on color alone. The caller supplies the body lines
// once the card knows how much room it can spare for them.
func paintCard(grid [][]canvasCell, rect canvasRect, emphasized bool, colors canvasCardColors, body func(maxLines, maxWidth int) []cardLine) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}

	x1 := clampInt(rect.x, 0, len(grid[0])-1)
	y1 := clampInt(rect.y, 0, len(grid)-1)
	x2 := clampInt(rect.x+rect.w-1, 0, len(grid[0])-1)
	y2 := clampInt(rect.y+rect.h-1, 0, len(grid)-1)
	if x2-x1 < 2 || y2-y1 < 2 {
		return
	}

	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			grid[y][x] = canvasCell{ch: ' ', fg: colors.fg, bg: colors.bg}
		}
	}

	h, v, tl, tr, bl, br := '─', '│', '╭', '╮', '╰', '╯'
	if emphasized {
		h, v, tl, tr, bl, br = '━', '┃', '┏', '┓', '┗', '┛'
	}
	// Border cells keep the stage behind them: a card fill under the line
	// would show as a halo outside it, because a terminal draws box lines
	// through the middle of the cell.
	edge := func(ch rune) canvasCell {
		return canvasCell{ch: ch, fg: colors.border, bold: emphasized}
	}
	for x := x1 + 1; x < x2; x++ {
		grid[y1][x] = edge(h)
		grid[y2][x] = edge(h)
	}
	for y := y1 + 1; y < y2; y++ {
		grid[y][x1] = edge(v)
		grid[y][x2] = edge(v)
	}
	grid[y1][x1], grid[y1][x2], grid[y2][x1], grid[y2][x2] = edge(tl), edge(tr), edge(bl), edge(br)

	innerW := x2 - x1 - 1
	innerH := y2 - y1 - 1
	pad := 0
	if innerW >= 14 {
		pad = 1
	}
	left, right := x1+1+pad, x2-1-pad
	textW := right - left + 1

	lines := body(max(1, innerH), max(1, textW))
	chips, chipsOnNameRow := cardChipPlacement(lines, textW)
	if chipsOnNameRow {
		// The chips share the name row, which frees a row for the bottom block.
		lines = body(innerH+1, max(1, textW))
		chips, chipsOnNameRow = cardChipPlacement(lines, textW)
	}

	// Narrow cards cannot hold a left/right split; center what fits instead.
	if textW < 12 {
		rows := make([]cardLine, 0, len(lines))
		for _, line := range lines {
			if line.role == cardRoleWorkspaces {
				line.text = strings.Join(line.workspaces, ",")
			}
			rows = append(rows, line)
		}
		rows = rows[:min(len(rows), innerH)]
		startY := y1 + 1 + max(0, (innerH-len(rows))/2)
		for idx, line := range rows {
			paintCanvasTextCentered(grid, x1+1, x2-1, startY+idx, fitString(line.text, innerW), line.fg, colors.bg, line.bold)
		}
		return
	}

	var top, bottom []cardLine
	for _, line := range lines {
		switch line.role {
		case cardRoleWorkspaces:
			if !chipsOnNameRow {
				top = append(top, line)
			}
		case cardRoleMode, cardRolePlacement:
			bottom = append(bottom, line)
		default:
			top = append(top, line)
		}
	}
	for len(top)+len(bottom) > innerH && len(bottom) > 0 {
		bottom = bottom[:len(bottom)-1]
	}
	top = top[:min(len(top), innerH)]

	paintRow := func(y int, line cardLine) {
		if line.role == cardRoleWorkspaces {
			paintChips(grid, y, left, right, line, colors.bg, false)
			return
		}
		paintCanvasText(grid, left, right, y, fitString(line.text, textW), line.fg, colors.bg, line.bold)
	}
	for idx, line := range top {
		y := y1 + 1 + idx
		paintRow(y, line)
		if idx == 0 && chipsOnNameRow {
			paintChips(grid, y, left, right, chips, colors.bg, true)
		}
	}
	for idx, line := range bottom {
		paintRow(y2-len(bottom)+idx, line)
	}
}

// cardChipPlacement finds the workspace line and reports whether its chips fit
// to the right of the connector name.
func cardChipPlacement(lines []cardLine, width int) (cardLine, bool) {
	var name, chips cardLine
	haveName, haveChips := false, false
	for _, line := range lines {
		switch line.role {
		case cardRoleName:
			name, haveName = line, true
		case cardRoleWorkspaces:
			chips, haveChips = line, true
		}
	}
	if !haveName || !haveChips || len(chips.workspaces) == 0 {
		return chips, false
	}
	return chips, lipgloss.Width(name.text)+2+chipRunWidth(chips.workspaces, len(chips.workspaces)) <= width
}

// chipRunWidth is the width of the first n chips drawn side by side.
func chipRunWidth(ids []string, n int) int {
	width := 0
	for idx := 0; idx < n && idx < len(ids); idx++ {
		if idx > 0 {
			width++
		}
		width += lipgloss.Width(ids[idx]) + 2
	}
	return width
}

// paintChips draws workspace IDs as chips, left aligned or right aligned
// against the card edge, and ends with +N when they do not all fit.
func paintChips(grid [][]canvasCell, y, left, right int, line cardLine, cardBg string, alignRight bool) {
	ids := line.workspaces
	if len(ids) == 0 {
		return
	}
	width := right - left + 1
	shown := len(ids)
	overflow := ""
	for shown > 0 {
		overflow = ""
		if shown < len(ids) {
			overflow = fmt.Sprintf(" +%d", len(ids)-shown)
		}
		if chipRunWidth(ids, shown)+len(overflow) <= width {
			break
		}
		shown--
	}
	if shown == 0 {
		paintCanvasText(grid, left, right, y, fitString(fmt.Sprintf("+%d", len(ids)), width), line.fg, cardBg, true)
		return
	}
	total := chipRunWidth(ids, shown) + len(overflow)
	x := left
	if alignRight {
		x = right - total + 1
	}
	for idx := 0; idx < shown; idx++ {
		if idx > 0 {
			paintCanvasText(grid, x, right, y, " ", line.fg, cardBg, false)
			x++
		}
		chip := " " + ids[idx] + " "
		paintCanvasText(grid, x, right, y, chip, line.fg, blankFallback(line.bg, cardBg), true)
		x += lipgloss.Width(chip)
	}
	if overflow != "" {
		paintCanvasText(grid, x, right, y, overflow, line.fg, cardBg, true)
	}
}

// paintCanvasText writes left-aligned text between left and right.
func paintCanvasText(grid [][]canvasCell, left, right, y int, text string, fg string, bg string, bold bool) {
	if y < 0 || y >= len(grid) || left > right {
		return
	}
	x := left
	for _, r := range text {
		if x > right || x >= len(grid[y]) {
			return
		}
		if x >= 0 {
			grid[y][x] = canvasCell{ch: r, fg: fg, bg: bg, bold: bold}
		}
		x++
	}
}

func paintCanvasTextCentered(grid [][]canvasCell, left, right, y int, text string, fg string, bg string, bold bool) {
	if y < 0 || y >= len(grid) || left > right {
		return
	}
	runes := []rune(text)
	width := right - left + 1
	if len(runes) > width {
		runes = []rune(fitString(text, width))
	}
	start := left + max(0, (width-len(runes))/2)
	for idx, r := range runes {
		x := start + idx
		if x < left || x > right || x < 0 || x >= len(grid[y]) {
			continue
		}
		grid[y][x] = canvasCell{ch: r, fg: fg, bg: bg, bold: bold}
	}
}

// canvasSegment is one styled run in the canvas overlay strip.
type canvasSegment struct {
	text string
	fg   string
	bold bool
}

// paintCanvasSegments writes a styled strip along one canvas row. The layout
// always leaves the first row free of cards, so the strip never hides one.
func paintCanvasSegments(grid [][]canvasCell, y, left int, segments []canvasSegment) {
	if y < 0 || y >= len(grid) {
		return
	}
	x := left
	for _, segment := range segments {
		for _, r := range segment.text {
			if x < 0 || x >= len(grid[y]) {
				return
			}
			grid[y][x] = canvasCell{ch: r, fg: segment.fg, bg: grid[y][x].bg, bold: segment.bold}
			x++
		}
	}
}

// hiddenOutputSegments names the displays a canvas cannot draw: the ones this
// layout turns off and the ones that mirror another display.
func (m Model) hiddenOutputSegments(outputs []editableOutput, selected int, width int) []canvasSegment {
	off := make([]int, 0, len(outputs))
	mirrored := make([]int, 0, len(outputs))
	for idx, output := range outputs {
		switch {
		case !output.Enabled:
			off = append(off, idx)
		case output.MirrorOf != "":
			mirrored = append(mirrored, idx)
		}
	}
	if len(off) == 0 && len(mirrored) == 0 {
		return nil
	}

	p := m.styles.palette
	segments := make([]canvasSegment, 0, len(off)+len(mirrored)+4)
	first := true
	add := func(label string, indexes []int, name func(editableOutput) string) {
		if len(indexes) == 0 {
			return
		}
		if !first {
			segments = append(segments, canvasSegment{text: "   ", fg: p.cardMuted})
		}
		first = false
		segments = append(segments, canvasSegment{text: label, fg: p.cardMuted})
		for pos, idx := range indexes {
			if pos > 0 {
				segments = append(segments, canvasSegment{text: ", ", fg: p.cardMuted})
			}
			fg, bold := p.cardDisabledFg, false
			if idx == selected {
				fg, bold = p.cardSelectedBorder, true
			}
			segments = append(segments, canvasSegment{text: name(outputs[idx]), fg: fg, bold: bold})
		}
	}

	segments = append(segments, canvasSegment{text: " ", fg: p.cardMuted})
	add("Off: ", off, func(o editableOutput) string { return o.Name })
	add("Mirrored: ", mirrored, func(o editableOutput) string {
		return o.Name + " → " + outputNameForKeyIn(outputs, o.MirrorOf)
	})
	segments = append(segments, canvasSegment{text: " ", fg: p.cardMuted})

	used := 0
	for idx, segment := range segments {
		remaining := width - used
		if remaining <= 0 {
			return segments[:idx]
		}
		if lipgloss.Width(segment.text) > remaining {
			segments[idx].text = fitString(segment.text, remaining)
			return segments[:idx+1]
		}
		used += lipgloss.Width(segment.text)
	}
	return segments
}

func paintSnapMark(grid [][]canvasCell, rect canvasRect, edge snapEdge, highlight string) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}

	x1 := clampInt(rect.x, 0, len(grid[0])-1)
	y1 := clampInt(rect.y, 0, len(grid)-1)
	x2 := clampInt(rect.x+rect.w-1, 0, len(grid[0])-1)
	y2 := clampInt(rect.y+rect.h-1, 0, len(grid)-1)
	switch edge {
	case snapEdgeLeft:
		for y := y1; y <= y2; y++ {
			grid[y][x1] = canvasCell{ch: '┃', fg: highlight, bg: grid[y][x1].bg, bold: true}
		}
	case snapEdgeRight:
		for y := y1; y <= y2; y++ {
			grid[y][x2] = canvasCell{ch: '┃', fg: highlight, bg: grid[y][x2].bg, bold: true}
		}
	case snapEdgeTop:
		for x := x1; x <= x2; x++ {
			grid[y1][x] = canvasCell{ch: '━', fg: highlight, bg: grid[y1][x].bg, bold: true}
		}
	case snapEdgeBottom:
		for x := x1; x <= x2; x++ {
			grid[y2][x] = canvasCell{ch: '━', fg: highlight, bg: grid[y2][x].bg, bold: true}
		}
	}
}

func renderCanvasCells(grid [][]canvasCell) string {
	lines := make([]string, len(grid))
	for y, row := range grid {
		var line strings.Builder
		var run strings.Builder
		cur := canvasCell{}
		have := false
		flush := func() {
			if !have || run.Len() == 0 {
				return
			}
			style := lipgloss.NewStyle()
			if cur.fg != "" {
				style = style.Foreground(lipgloss.Color(cur.fg))
			}
			if cur.bg != "" {
				style = style.Background(lipgloss.Color(cur.bg))
			}
			if cur.bold {
				style = style.Bold(true)
			}
			line.WriteString(style.Render(run.String()))
			run.Reset()
		}
		for _, cell := range row {
			if !have || cell.fg != cur.fg || cell.bg != cur.bg || cell.bold != cur.bold {
				flush()
				cur = cell
				have = true
			}
			run.WriteRune(cell.ch)
		}
		flush()
		lines[y] = line.String()
	}
	return strings.Join(lines, "\n")
}
