package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/crmne/hyprmoncfg/internal/apply"
)

func (m Model) renderSavePrompt() string {
	if m.saveDialog == nil {
		return m.renderModalFrame("Save Profile", nil)
	}
	title := "Save Profile"
	body := make([]string, 0, 12)
	if m.saveDialog.Purpose == saveDialogQuit {
		title = "Save Before Quitting"
		body = append(body,
			m.styles.warning.Render("You have unsaved monitor changes."),
			m.styles.subtle.Render("Save and apply them before quitting."),
			"",
		)
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.saveDialog.Input.View())
	body = append(body,
		m.styles.label.Render("Name"),
		inputBox,
		"",
		m.saveDialog.List.View(),
		"",
		m.styles.label.Render("Action"),
		m.renderSaveActionButtons(),
		"",
	)
	if status := m.renderErrorStatus(); status != "" {
		body = append(body, status, "")
	}
	// Wrap the help instead of cutting it off. Tall terminals keep the dialog
	// narrow; short ones spend width to save rows.
	helpWidth := max(20, m.modalMaxWidth()-6)
	if m.terminalHeight() >= 30 {
		helpWidth = min(helpWidth, 56)
	}
	body = append(body, m.styles.help.Width(helpWidth).Render("Type to filter names. Up/Down selects an existing profile. Left/Right or Tab switches action. Enter confirms. Esc cancels."))
	return m.renderModalFrame(title, body)
}

func (m Model) renderSaveConfirm() string {
	consequence := "The existing profile will be replaced with the current draft."
	if m.saveDialog != nil && m.saveDialog.Action == saveActionApply {
		consequence = "The existing profile will be replaced and then applied to the live layout."
	} else if m.saveDialog != nil && m.saveDialog.Action == saveActionSaveQuit {
		consequence = "The existing profile will be replaced, applied, then hyprmoncfg will quit."
	}

	body := []string{
		m.styles.warning.Render(fmt.Sprintf("Overwrite profile %q?", m.saveOverwrite)),
		m.styles.subtle.Render(consequence),
		"",
		m.styles.help.Render("Enter or y overwrites. Esc or n cancels."),
	}
	return m.renderModalFrame("Confirm Overwrite", body)
}

// renderConfirm is the Keep/Revert step of a preview. It says what is being
// kept, drains a meter toward the daemon's deadline, and keeps both actions
// visible as buttons at every terminal size. The deadline is authoritative;
// the meter only shows it.
func (m Model) renderConfirm() string {
	if m.pending == nil {
		return m.renderModalFrame("Keep this layout?", nil)
	}

	remaining := m.pending.deadline.Sub(m.clock())
	if remaining < 0 {
		remaining = 0
	}
	seconds := int(math.Ceil(remaining.Seconds()))
	total := m.pending.total
	if total < remaining || total <= 0 {
		total = apply.DefaultPreviewTimeout
		if remaining > total {
			total = remaining
		}
	}

	title := "Keep this layout?"
	subject := "Your changes"
	if name := strings.TrimSpace(m.pending.profile.Name); name != "" && name != "draft" {
		title = "Keep this profile?"
		subject = name
	}

	width := clampInt(m.modalMaxWidth()-6, 20, 58)
	unit := "seconds"
	if seconds == 1 {
		unit = "second"
	}
	// Short terminals drop the spacing rows and the modal padding, so Keep
	// and Revert stay on screen below the editor's normal minimum size.
	compact := m.terminalHeight() < 20
	gap := func() []string {
		if compact {
			return nil
		}
		return []string{""}
	}
	body := []string{m.styles.value.Render(fitString(subject+" · the previous layout returns in "+strconv.Itoa(seconds)+" "+unit, width))}
	body = append(body, gap()...)
	body = append(body, m.renderCountdownMeter(remaining, total, width))

	if stage := m.confirmStageRows(width); stage > 0 {
		outputs := m.profileEditableOutputs(m.pending.profile)
		canvas := m.renderStaticCanvas(outputs, width, stage, func(output editableOutput) canvasCard {
			colors := m.staticCardStyle()
			return canvasCard{colors: colors, body: func(maxLines, maxWidth int) []cardLine {
				return m.monitorCardLines(output, nil, monitorCardProfile, maxLines, maxWidth, colors, "", m.styles.palette.warning)
			}}
		})
		body = append(body, "", fitBlock(canvas, width, stage))
	}

	body = append(body, gap()...)
	body = append(body, m.renderConfirmButtons(width))
	if status := m.renderErrorStatus(); status != "" {
		body = append(body, status)
	}
	body = append(body, gap()...)
	body = append(body, m.styles.help.Width(width).Render(m.confirmApplyHelp()))
	if compact {
		lines := append([]string{m.styles.modalTitle.Render(title)}, body...)
		return m.styles.modal.Padding(0, 1).MaxWidth(m.modalMaxWidth()).Render(strings.Join(lines, "\n"))
	}
	return m.renderModalFrame(title, body)
}

// confirmStageRows sizes the miniature of the layout being kept, which only
// appears when the terminal has rows to spare for it.
func (m Model) confirmStageRows(width int) int {
	spare := m.terminalHeight() - 20
	if spare < 6 || m.pending == nil {
		return 0
	}
	return clampInt(stageRowsFor(m.profileEditableOutputs(m.pending.profile), width), 5, min(12, spare))
}

// renderCountdownMeter drains from left to right as the deadline nears and
// ends with the seconds left, so the countdown does not rely on color.
func (m Model) renderCountdownMeter(remaining, total time.Duration, width int) string {
	seconds := int(math.Ceil(remaining.Seconds()))
	label := fmt.Sprintf(" %2ds", seconds)
	bar := max(4, width-lipgloss.Width(label))
	filled := 0
	if total > 0 {
		filled = int(math.Round(float64(bar) * float64(remaining) / float64(total)))
	}
	filled = clampInt(filled, 0, bar)
	fill := m.styles.statusOK
	if seconds <= 5 {
		fill = m.styles.warning
	}
	return fill.Render(strings.Repeat("━", filled)) +
		withFG(lipgloss.NewStyle(), m.styles.palette.meterEmpty).Render(strings.Repeat("─", bar-filled)) +
		fill.Render(label)
}

const (
	confirmKeepLabel   = "[Keep]"
	confirmRevertLabel = "[Revert]"
)

func (m Model) renderConfirmButtons(width int) string {
	buttons := m.styles.value.Render(confirmRevertLabel) + "  " + m.styles.focused.UnsetPadding().Render(confirmKeepLabel)
	return lipgloss.PlaceHorizontal(width, lipgloss.Right, buttons)
}

// answerKey normalizes a yes/no keypress. A prompt like "Press y" is answered
// with Shift held often enough that a case-sensitive match reads as the dialog
// being broken.
func answerKey(msg tea.KeyMsg) string {
	key := msg.String()
	if len([]rune(key)) == 1 {
		return strings.ToLower(key)
	}
	return key
}

func (m Model) confirmApplyHelp() string {
	if m.quitAfterApply {
		return "Enter or y keeps it and quits. Esc or n reverts."
	}
	return "Enter or y keeps it. Esc or n reverts."
}

func (m Model) renderToast() string {
	if m.toast == nil || strings.TrimSpace(m.toast.message) == "" {
		return ""
	}
	style := m.styles.toast
	if m.toast.err {
		style = m.styles.toastError
	}
	return style.MaxWidth(max(24, m.terminalWidth()-8)).Render(m.toast.message)
}

func (m Model) renderErrorStatus() string {
	if m.status == "" || !m.statusErr {
		return ""
	}
	return m.styles.statusError.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(m.status)
}

func (m Model) renderModalFrame(title string, body []string) string {
	lines := []string{m.styles.modalTitle.Render(title)}
	if len(body) > 0 {
		lines = append(lines, "", strings.Join(body, "\n"))
	}
	return m.styles.modal.MaxWidth(m.modalMaxWidth()).Render(strings.Join(lines, "\n"))
}

func (m Model) renderModalScreen(overlay string) string {
	if strings.TrimSpace(overlay) == "" {
		return m.renderMain()
	}

	width := m.terminalWidth()
	height := m.terminalHeight()
	toast := m.renderToast()
	toastHeight := 0
	if toast != "" {
		toastHeight = lipgloss.Height(toast) + 1
	}

	tabs := m.renderTabs()
	bodyHeight := max(3, height-lipgloss.Height(tabs)-toastHeight)
	centered := lipgloss.Place(width-2, bodyHeight, lipgloss.Center, lipgloss.Center, overlay)
	body := m.styles.modalBackdrop.Width(width).Height(bodyHeight).Render(centered)
	if toast != "" {
		body = strings.Join([]string{
			body,
			lipgloss.PlaceHorizontal(max(1, width-2), lipgloss.Center, toast),
		}, "\n")
	}
	return strings.Join([]string{tabs, body}, "\n")
}

func (m Model) unsavedBadge() string {
	if m.dirty && !m.draftSaved {
		return m.styles.warning.Render("Changes not applied")
	}
	if m.dirty && m.draftSaved {
		return m.styles.badgeOn.Render("Saved Draft")
	}
	return m.styles.subtle.Render("Current setup")
}
