package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

func (m Model) renderWorkspaceView(height int) string {
	leftStyle := m.paneStyle(paneToneFocused)

	if m.terminalWidth() < 96 {
		// Compact: stack vertically, settings get enough room, preview gets the rest
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		settingsHeight := clampInt(m.workspaceSettingsLineCount()+2, 6, (height*2)/3)
		innerW := max(1, width-leftStyle.GetHorizontalFrameSize())
		innerH := max(1, settingsHeight-leftStyle.GetVerticalFrameSize())
		settings := m.workspaceSettingsVisibleLines(innerH)
		leftBody := fitBlock(strings.Join(settings, "\n"), innerW, innerH)
		left := m.renderTitledPane(paneToneFocused, "Workspace Planner", leftBody, width)
		right := m.renderWorkspacePreviewPanes(width, max(3, height-settingsHeight))
		return lipgloss.JoinVertical(lipgloss.Left, left, right)
	}

	leftWidth, rightWidth := m.sidePaneWidths(35)
	innerH := max(1, height-leftStyle.GetVerticalFrameSize())
	settings := m.workspaceSettingsVisibleLines(innerH)
	leftBody := fitBlock(strings.Join(settings, "\n"), max(1, leftWidth-leftStyle.GetHorizontalFrameSize()), innerH)
	left := m.renderTitledPane(paneToneFocused, "Workspace Planner", leftBody, leftWidth)
	right := m.renderWorkspacePreviewPanes(rightWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGapWidth), right)
}

// workspaceSettingsLines renders the planner form. Its row order is the one
// workspaceSettingsRect relies on for mouse hits, so keep them in step.
func (m Model) workspaceSettingsLines() []string {
	lines := make([]string, 0, m.workspaceSettingsLineCount())
	for line := 0; line < m.workspaceSettingsLineCount(); line++ {
		lines = append(lines, m.workspaceSettingsLine(line))
	}
	return lines
}

func (m Model) workspaceSettingsLine(line int) string {
	if line >= 0 && line < len(workspaceFields) {
		value := m.workspaceFieldValue(line)
		prefix := "  "
		if line == m.workspaceEdit.SelectedField {
			value = m.styles.focused.Render(value)
		} else {
			value = m.styles.value.Render(value)
		}
		return prefix + m.styles.label.Render(fmt.Sprintf("%-14s", workspaceFields[line])) + " " + value
	}
	if line == len(workspaceFields) {
		return ""
	}
	if line == len(workspaceFields)+1 {
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			return m.styles.label.Render("Workspace → display") + "  " + m.styles.subtle.Render("←/→ assigns")
		}
		return m.styles.label.Render("Monitor order") + "  " + m.styles.subtle.Render("←/→ reorders")
	}

	item := line - len(workspaceFields) - 2
	if item < 0 || item >= m.workspaceListItemCount() {
		return m.styles.subtle.Render("  (none)")
	}
	if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
		rule := m.workspaceEdit.Rules[item]
		label := m.manualWorkspaceRuleOutputLabel(rule)
		prefix := "  "
		if len(workspaceFields)+item == m.workspaceEdit.SelectedField {
			label = m.styles.focused.Render(label)
		} else {
			label = m.styles.value.Render(label)
		}
		workspace := m.styles.subtle.Render(fmt.Sprintf("%-14s", "Workspace "+blankFallback(rule.Workspace, "?")))
		return prefix + workspace + " " + label
	}

	label := m.outputLabelForKey(m.workspaceEdit.MonitorOrder[item])
	prefix := "  "
	if len(workspaceFields)+item == m.workspaceEdit.SelectedField {
		prefix = m.styles.statusOK.Render("> ")
		label = m.styles.focused.Render(label)
	} else {
		label = m.styles.value.Render(label)
	}
	return fmt.Sprintf("%s%s %s", prefix, m.styles.subtle.Render(fmt.Sprintf("%d.", item+1)), label)
}

func (m Model) workspaceSelectedLine() int {
	if m.workspaceEdit.SelectedField < len(workspaceFields) {
		return m.workspaceEdit.SelectedField
	}
	return len(workspaceFields) + 2 + m.workspaceEdit.SelectedField - len(workspaceFields)
}

func (m Model) workspaceSettingsScrollOffset(height int) int {
	return inspectorScrollOffset(m.workspaceSettingsLineCount(), m.workspaceSelectedLine(), height)
}

func (m Model) workspaceSettingsVisibleLines(height int) []string {
	offset := m.workspaceSettingsScrollOffset(height)
	end := min(m.workspaceSettingsLineCount(), offset+max(1, height))
	lines := make([]string, 0, max(0, end-offset))
	for line := offset; line < end; line++ {
		lines = append(lines, m.workspaceSettingsLine(line))
	}
	return lines
}

// renderWorkspacePreviewPanes shows the resolved plan as an aligned list and,
// when there is room, the same plan laid over the monitor arrangement.
func (m Model) renderWorkspacePreviewPanes(width, height int) string {
	style := m.paneStyle(paneToneStatic)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())

	settings := m.workspaceEdit.settings()
	outputs := m.currentProfileOutputs()
	var rules []profile.WorkspaceRule
	if settings.Enabled {
		rules = settings.Rules
		if settings.Strategy != profile.WorkspaceStrategyManual && settings.Strategy != "" {
			rules = m.workspacePlan(outputs, settings)
		}
	}

	planLines := make([]string, 0, len(outputs)+2)
	if !settings.Enabled {
		planLines = append(planLines, m.styles.subtle.Render("Off: hyprmoncfg writes no workspace rules."))
	} else if rows := m.workspacePlanRows(rules, outputs, innerWidth); len(rows) > 0 {
		planLines = append(planLines, rows...)
	} else {
		planLines = append(planLines, m.styles.subtle.Render("(no workspace rules configured)"))
	}

	stage, planHeight := m.stageAndTextHeights(m.editOutputs, width, height, len(planLines))
	if stage == 0 {
		body := fitBlock(strings.Join(planLines, "\n"), innerWidth, max(1, height-style.GetVerticalFrameSize()))
		return m.renderTitledPane(paneToneStatic, "Workspace Plan", body, width)
	}

	plan := fitBlock(strings.Join(planLines, "\n"), innerWidth, max(1, planHeight-style.GetVerticalFrameSize()))
	canvasInner := max(1, stage-style.GetVerticalFrameSize())
	canvas := fitBlock(m.renderWorkspaceCanvas(workspacePlanByConnector(rules), innerWidth, canvasInner), innerWidth, canvasInner)
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderTitledPane(paneToneStatic, "Monitor Layout", canvas, width),
		m.renderTitledPane(paneToneStatic, "Workspace Plan", plan, width),
	)
}
