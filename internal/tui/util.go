package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func outputTypeLabel(output editableOutput) string {
	if output.IsInternal {
		return "Internal display"
	}
	return "External display"
}

func fitBlock(text string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	wrapper := lipgloss.NewStyle().Width(width).MaxWidth(width)
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, height)
	for _, line := range raw {
		rendered := wrapper.Render(line)
		lines = append(lines, strings.Split(rendered, "\n")...)
		if len(lines) >= height {
			break
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// padBlock sizes a block whose lines never need wrapping, such as the canvas
// grid: it pads or cuts each line and the line count. It is the cheap
// counterpart of fitBlock, which re-wraps every line, and it runs on every
// frame of a drag.
func padBlock(text string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for idx, line := range lines {
		w := ansi.StringWidth(line)
		switch {
		case w > width:
			lines[idx] = ansi.Truncate(line, width, "")
		case w < width:
			lines[idx] = line + strings.Repeat(" ", width-w)
		}
	}
	return strings.Join(lines, "\n")
}

// Lipgloss Width includes padding, but adds borders and margins outside it.
// Convert the total pane width allocated by the layout into the value Width
// expects while leaving the pane's internal padding intact.
func styleRenderWidth(total int, style lipgloss.Style) int {
	return max(1, total-style.GetHorizontalMargins()-style.GetHorizontalBorderSize())
}

func normalizeModes(modes []string, current string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(modes)+1)
	add := func(mode string) {
		mode = strings.TrimSpace(mode)
		if mode == "" || seen[mode] {
			return
		}
		seen[mode] = true
		out = append(out, mode)
	}

	add(current)
	for _, mode := range modes {
		add(mode)
	}
	return out
}

func indexOf(values []string, target string) int {
	target = strings.TrimSpace(target)
	for idx, value := range values {
		if strings.TrimSpace(value) == target {
			return idx
		}
	}
	return -1
}

func fitString(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func blankFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultProfileName() string {
	return "profile-" + time.Now().Format("20060102-150405")
}

func (m *Model) showSnapHint(hint *snapHintState) tea.Cmd {
	if hint == nil {
		m.snap = nil
		return nil
	}
	m.snapSeq++
	hint.Token = m.snapSeq
	m.snap = hint
	return clearSnapCmd(hint.Token)
}

func clearSnapCmd(token int) tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg {
		return clearSnapMsg{token: token}
	})
}

func clearToastCmd(token int) tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return clearToastMsg{token: token}
	})
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func targetLabel(name string) string {
	if strings.TrimSpace(name) == "" || name == "draft" {
		return "Draft changes"
	}
	return fmt.Sprintf("Profile %q", name)
}

func boolText(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func vrrLabel(v int) string {
	switch v {
	case 1:
		return "on"
	case 2:
		return "fullscreen"
	default:
		return "off"
	}
}

func triStateLabel(v int) string {
	switch v {
	case -1:
		return "off"
	case 1:
		return "on"
	default:
		return "auto"
	}
}

func transformLabel(v int) string {
	switch v {
	case 0:
		return "normal"
	case 1:
		return "90"
	case 2:
		return "180"
	case 3:
		return "270"
	case 4:
		return "flip"
	case 5:
		return "flip-90"
	case 6:
		return "flip-180"
	case 7:
		return "flip-270"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func blankStrategy(strategy profile.WorkspaceStrategy) profile.WorkspaceStrategy {
	if strategy == "" {
		return profile.WorkspaceStrategySequential
	}
	return strategy
}

func wrapIndex(idx, length int) int {
	if length <= 0 {
		return 0
	}
	for idx < 0 {
		idx += length
	}
	return idx % length
}

func wrapValue(value, minValue, maxValue int) int {
	if maxValue < minValue {
		return minValue
	}
	rangeSize := maxValue - minValue + 1
	for value < minValue {
		value += rangeSize
	}
	for value > maxValue {
		value -= rangeSize
	}
	return value
}

func clampIndex(idx, length int) int {
	if length <= 0 {
		return 0
	}
	if idx < 0 {
		return length - 1
	}
	if idx >= length {
		return 0
	}
	return idx
}

func layoutMoveDelta(key string) (dx, dy int, ok bool) {
	switch key {
	case "left", "h":
		return -100, 0, true
	case "right", "l":
		return 100, 0, true
	case "up", "k":
		return 0, -100, true
	case "down", "j":
		return 0, 100, true
	case "shift+left":
		return -10, 0, true
	case "shift+right":
		return 10, 0, true
	case "shift+up":
		return 0, -10, true
	case "shift+down":
		return 0, 10, true
	case "ctrl+left":
		return -1, 0, true
	case "ctrl+right":
		return 1, 0, true
	case "ctrl+up":
		return 0, -1, true
	case "ctrl+down":
		return 0, 1, true
	case "H":
		return -500, 0, true
	case "L":
		return 500, 0, true
	case "K":
		return 0, -500, true
	case "J":
		return 0, 500, true
	default:
		return 0, 0, false
	}
}

func layoutSnapDirection(key string) (snapDirection, bool) {
	switch key {
	case "alt+left":
		return snapDirectionLeft, true
	case "alt+right":
		return snapDirectionRight, true
	case "alt+up":
		return snapDirectionUp, true
	case "alt+down":
		return snapDirectionDown, true
	default:
		return snapDirectionLeft, false
	}
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func adjustPositiveInt(value, delta int) int {
	maxInt := int(^uint(0) >> 1)
	if delta > 0 && value > maxInt-delta {
		return maxInt
	}
	if delta < 0 && value < 1-delta {
		return 1
	}
	return max(1, value+delta)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func focusedOutputIndex(outputs []editableOutput) int {
	for idx, output := range outputs {
		if output.Focused {
			return idx
		}
	}
	return -1
}

func outputIndexByKey(outputs []editableOutput, key string) int {
	for idx, output := range outputs {
		if output.Key == key {
			return idx
		}
	}
	return 0
}
