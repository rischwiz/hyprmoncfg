package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderProfilesView(height int) string {
	summaries := m.profileMatchSummaries()
	automaticWidth := m.profileAutomaticRect().w
	automatic := m.renderTitledPane(paneToneStatic, "Automatic profile selection", m.profileAutomaticRow(max(1, automaticWidth-m.styles.inactivePane.GetHorizontalFrameSize())), automaticWidth)

	if m.terminalWidth() < 96 {
		height -= profileAutomaticPaneHeight
		// Compact: stack vertically, list gets enough for profiles, details gets the rest.
		width := m.terminalWidth() - m.styles.app.GetHorizontalFrameSize()
		listHeight := m.compactProfileListHeight(height)
		left := m.renderProfileListPane(summaries, width, listHeight)
		right := m.renderProfileDetailPanes(summaries, width, max(3, height-listHeight))
		return lipgloss.JoinVertical(lipgloss.Left, automatic, left, right)
	}

	listWidth, detailWidth := m.sidePaneWidths(35)
	left := lipgloss.JoinVertical(lipgloss.Left, automatic, m.renderProfileListPane(summaries, listWidth, height-profileAutomaticPaneHeight))
	right := m.renderProfileDetailPanes(summaries, detailWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGapWidth), right)
}

func (m Model) compactProfileListHeight(height int) int {
	return clampInt(len(m.profiles)+profileListHeaderRows+m.profileListActionRows()+2, 7, max(7, height/3))
}

func (m Model) renderProfileListPane(summaries []profileMatchSummary, width, height int) string {
	style := m.paneStyle(paneToneFocused)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())
	innerHeight := max(1, height-style.GetVerticalFrameSize())
	cols := m.profileListColumns(innerWidth)
	rows := m.profileListRows(summaries, cols)
	actionRows := m.profileListActionRows()
	actions := m.styles.value.Render(fitString(profileActionsPrimary+" "+profileActionsSecondary, innerWidth))
	if actionRows == 2 {
		actions = m.styles.value.Render(fitString(profileActionsPrimary, innerWidth)) + "\n" +
			m.styles.value.Render(fitString(profileActionsSecondary, innerWidth))
	}
	if len(m.profiles) == 0 {
		actions = strings.Repeat("\n", actionRows-1)
	}
	lines := append([]string{m.profileListHeader(cols), ""}, rows[min(m.profileListScroll(innerHeight), len(rows)-1):]...)
	body := fitBlock(strings.Join(lines, "\n"), innerWidth, max(1, innerHeight-actionRows)) + "\n" + actions
	return m.renderTitledPane(paneToneFocused, "Saved Profiles", body, width)
}

// profileCanvasMinHeight is the smallest pane that still shows a readable
// monitor card; below it the details pane keeps the text only.
const profileCanvasMinHeight = 8

func (m Model) renderProfileDetailPanes(summaries []profileMatchSummary, width, height int) string {
	style := m.paneStyle(paneToneStatic)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())

	if len(m.profiles) == 0 {
		body := fitBlock(m.styles.subtle.Render("(no saved profiles)"), innerWidth, max(1, height-style.GetVerticalFrameSize()))
		return m.renderTitledPane(paneToneStatic, "Profile Details", body, width)
	}

	selected := m.profiles[m.selectedProfile]
	infoLines := m.renderDetailRows(m.profileDetailRows(selected, summaries[m.selectedProfile], innerWidth))
	stage, info := m.stageAndTextHeights(m.profileEditableOutputs(selected), width, height, len(infoLines))
	if stage == 0 {
		body := fitBlock(strings.Join(infoLines, "\n"), innerWidth, max(1, height-style.GetVerticalFrameSize()))
		return m.renderTitledPane(paneToneStatic, "Profile Details", body, width)
	}

	canvasInner := max(1, stage-style.GetVerticalFrameSize())
	canvas := fitBlock(m.renderProfileCanvas(selected, innerWidth, canvasInner), innerWidth, canvasInner)
	details := fitBlock(strings.Join(infoLines, "\n"), innerWidth, max(1, info-style.GetVerticalFrameSize()))
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderTitledPane(paneToneStatic, "Monitor Layout", canvas, width),
		m.renderTitledPane(paneToneStatic, "Profile Details", details, width),
	)
}

// stageAndTextHeights splits a preview column between a stage on top and
// the text below it. The text gets the rows it needs, up to half the column;
// the stage gets the rest and centres the arrangement in it. A column too
// short for a readable stage keeps only the text.
func (m Model) stageAndTextHeights(outputs []editableOutput, width, height, textLines int) (int, int) {
	frame := m.styles.staticPane.GetVerticalFrameSize()
	text := clampInt(textLines+frame, 1+frame, max(1+frame, height/2))
	if height-text < profileCanvasMinHeight {
		return 0, height
	}
	return height - text, text
}
