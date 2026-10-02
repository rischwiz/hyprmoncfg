package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/crmne/hyprmoncfg/internal/apply"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

func (m *Model) activateInspectorField() tea.Cmd {
	if len(m.editOutputs) == 0 {
		return nil
	}

	// Choice rows cycle in place: every option is already on screen.
	if len(inspectorChoiceValues(m.inspectorField)) > 0 {
		m.cycleInspectorChoice(m.inspectorField, 1)
		return nil
	}

	switch m.inspectorField {
	case 1:
		output := m.editOutputs[m.selectedOutput]
		if len(output.Modes) == 0 {
			return nil
		}
		items := make([]list.Item, 0, len(output.Modes))
		recommended := output.recommendedMode()
		for _, mode := range output.Modes {
			label := displayModeLabel(mode)
			if mode == recommended {
				label += recommendedSuffix
			}
			items = append(items, fieldPickerItem{pickerItem: pickerItem(mode), label: label})
		}
		inner := list.NewDefaultDelegate()
		inner.ShowDescription = false
		inner.SetHeight(1)
		inner.SetSpacing(0)
		inner.Styles.NormalTitle = m.styles.value
		inner.Styles.SelectedTitle = m.styles.focused.UnsetPadding()
		inner.Styles.DimmedTitle = m.styles.subtle
		inner.Styles.FilterMatch = m.styles.badgeAccent
		delegate := arrowDelegate{inner}
		picker := list.New(items, delegate, m.modePickerWidth()-2, m.modePickerHeight())
		picker.Title = fmt.Sprintf("Mode for %s", output.Name)
		picker.SetShowHelp(false)
		picker.SetShowPagination(false)
		picker.SetShowStatusBar(false)
		picker.SetFilteringEnabled(false)
		picker.DisableQuitKeybindings()
		picker.Styles.Title = m.styles.modalTitle
		picker.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
		picker.Styles.PaginationStyle = m.styles.subtle
		picker.Styles.HelpStyle = m.styles.help
		picker.Styles.NoItems = m.styles.subtle
		picker.Select(clampIndex(output.ModeIndex, len(output.Modes)))
		m.picker = &modePickerState{
			OutputIndex: m.selectedOutput,
			FieldIndex:  -1,
			List:        picker,
		}
		m.mode = modeModePicker
		return nil
	case 2:
		// Enter on Scale opens the full sharp list, the row's More… choice.
		m.openScalePicker()
		return nil
	case 7, 8:
		output := m.editOutputs[m.selectedOutput]
		kind := numericInputPositionX
		title := fmt.Sprintf("Set Position X for %s", output.Name)
		hint := "Type the exact X position in logical pixels. Enter applies. Esc cancels."
		value := strconv.Itoa(output.X)
		if m.inspectorField == 8 {
			kind = numericInputPositionY
			title = fmt.Sprintf("Set Position Y for %s", output.Name)
			hint = "Type the exact Y position in logical pixels. Enter applies. Esc cancels."
			value = strconv.Itoa(output.Y)
		}
		return m.openNumericInput(kind, m.selectedOutput, title, hint, value)
	case 4:
		m.openFieldPicker(layoutFields[4], m.inspectorField, []string{"srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid"})
		return nil
	case 6:
		m.openFieldPicker("Rotation", m.inspectorField, []string{"normal", "90", "180", "270", "flipped", "flipped+90", "flipped+180", "flipped+270"})
		return nil
	case 9:
		targets := []string{"None"}
		for i, other := range m.editOutputs {
			if i != m.selectedOutput {
				targets = append(targets, other.displayModelLabel())
			}
		}
		m.openFieldPicker("Mirror", m.inspectorField, targets)
		return nil
	case 10:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[10], "SDR-to-HDR luminance multiplier, 0–3 (0 uses 1). Enter applies. Esc cancels.", fmt.Sprintf("%.2f", sdrMultiplier(output.SDRBrightness)))
	case 11:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[11], "SDR-to-HDR saturation multiplier, 0–3 (0 uses 1). Enter applies. Esc cancels.", fmt.Sprintf("%.2f", sdrMultiplier(output.SDRSaturation)))
	case 12:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[12], "SDR-to-HDR black level, 0–1 cd/m². Enter applies. Esc cancels.", fmt.Sprintf("%.3f", output.SDRMinLuminance))
	case 13:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputInt, m.selectedOutput, layoutFields[13], "SDR-to-HDR white level, 0–1000 cd/m². Enter applies. Esc cancels.", fmt.Sprintf("%d", output.SDRMaxLuminance))
	case 15:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputFloat, m.selectedOutput, layoutFields[15], "Display black-level metadata in cd/m². All-zero overrides use EDID.", fmt.Sprintf("%.3f", output.MinLuminance))
	case 16:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputInt, m.selectedOutput, layoutFields[16], "Display peak-luminance metadata in cd/m². All-zero overrides use EDID.", fmt.Sprintf("%d", output.MaxLuminance))
	case 17:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(numericInputInt, m.selectedOutput, layoutFields[17], "Maximum frame-average luminance metadata in cd/m². Zero uses EDID.", fmt.Sprintf("%d", output.MaxAvgLuminance))
	case 20:
		output := m.editOutputs[m.selectedOutput]
		return m.openNumericInput(
			numericInputICC,
			m.selectedOutput,
			fmt.Sprintf("%s for %s", layoutFields[20], output.Name),
			"Absolute path to an ICC device profile. Leave empty to clear. Enter applies. Esc cancels.",
			output.ICC,
		)
	default:
		m.adjustInspectorField(1)
		return nil
	}
}

func (m *Model) openFieldPicker(title string, fieldIndex int, options []string) {
	output := m.editOutputs[m.selectedOutput]
	currentValue := m.layoutFieldValue(output, fieldIndex)

	labels := make([]string, len(options))
	selected := 0
	for i, opt := range options {
		labels[i] = fieldOptionLabel(fieldIndex, opt)
		if opt == currentValue {
			selected = i
		}
	}
	m.openLabeledPicker(title, fieldIndex, options, labels, selected)
}

// openScalePicker lists every scale the Scale row can step to (the shared
// sharp list, plus the current scale when it is not sharp) and ends with
// Custom… for typing any value.
func (m *Model) openScalePicker() {
	output := m.editOutputs[m.selectedOutput]
	choices := scaling.Choices(output.Width, output.Height, output.Scale)
	labels := scaleChoiceLabels(choices)
	recommended, known := output.recommendedScale()
	options := make([]string, 0, len(choices)+1)
	for idx, choice := range choices {
		options = append(options, strconv.FormatFloat(choice, 'f', -1, 64))
		if known && choice == recommended {
			labels[idx] += recommendedSuffix
		}
	}
	options = append(options, scaleCustomValue)
	labels = append(labels, "Custom…")
	m.openLabeledPicker("Scale", 2, options, labels, scaleChoiceIndex(choices, output.Scale))
}

const scaleCustomValue = "custom"

// openScaleEntry types an exact scale, with the dialog that explains
// sharpness and offers the closest sharp value.
func (m *Model) openScaleEntry() tea.Cmd {
	output := m.editOutputs[m.selectedOutput]
	m.inspectorField = 2
	return m.openNumericInput(
		numericInputScale,
		m.selectedOutput,
		fmt.Sprintf("Set Scale for %s", output.Name),
		"Type a scale. Enter applies. Esc cancels.",
		scaling.Format(output.Scale),
	)
}

func (m *Model) openLabeledPicker(title string, fieldIndex int, options, labels []string, selected int) {
	items := make([]list.Item, 0, len(options))
	for i, opt := range options {
		items = append(items, fieldPickerItem{pickerItem(opt), labels[i]})
	}
	inner := list.NewDefaultDelegate()
	inner.ShowDescription = false
	inner.SetHeight(1)
	inner.SetSpacing(0)
	inner.Styles.NormalTitle = m.styles.value
	inner.Styles.SelectedTitle = m.styles.focused.UnsetPadding()
	inner.Styles.DimmedTitle = m.styles.subtle
	inner.Styles.FilterMatch = m.styles.badgeAccent
	delegate := arrowDelegate{inner}
	height := clampInt(len(options)+2, 4, 12)
	picker := list.New(items, delegate, m.modePickerWidth()-2, height)
	picker.Title = title
	picker.SetShowHelp(false)
	picker.SetShowPagination(false)
	picker.SetShowStatusBar(false)
	picker.SetFilteringEnabled(false)
	picker.DisableQuitKeybindings()
	picker.Styles.Title = m.styles.modalTitle
	picker.Styles.TitleBar = lipgloss.NewStyle().PaddingBottom(1)
	picker.Styles.PaginationStyle = m.styles.subtle
	picker.Styles.HelpStyle = m.styles.help
	picker.Styles.NoItems = m.styles.subtle
	picker.Select(selected)
	m.picker = &modePickerState{
		OutputIndex: m.selectedOutput,
		FieldIndex:  fieldIndex,
		List:        picker,
	}
	m.mode = modeModePicker
}

func (m Model) pickerPrompt(name string) string {
	switch {
	case m.picker.FieldIndex < 0:
		return fmt.Sprintf("Pick a display mode for %s.", name)
	case m.picker.FieldIndex == 2:
		return fmt.Sprintf("Pick a sharp scale for %s, or Custom… to type one.", name)
	default:
		return fmt.Sprintf("Pick a value for %s.", name)
	}
}

func (m Model) numericInputWidthFor(kind numericInputKind) int {
	if kind == numericInputICC {
		return clampInt(m.modalMaxWidth()-16, 20, 60)
	}
	return clampInt(m.modalMaxWidth()-16, 8, 12)
}

func (m *Model) openNumericInput(kind numericInputKind, outputIndex int, title string, hint string, value string) tea.Cmd {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 12
	if kind == numericInputICC {
		input.CharLimit = 256
	} else if kind == numericInputWorkspaceCount || kind == numericInputWorkspaceGroupSize {
		// Match the native int used by the profile model, without imposing a
		// smaller UI policy limit.
		input.CharLimit = len(strconv.Itoa(int(^uint(0) >> 1)))
	}
	input.Width = m.numericInputWidthFor(kind)
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	if kind == numericInputScale {
		input.Placeholder = "1.00"
	}
	input.SetValue(value)
	cmd := input.Focus()
	m.input = &numericInputState{
		Kind:        kind,
		OutputIndex: outputIndex,
		FieldIndex:  m.inspectorField,
		Title:       title,
		Hint:        hint,
		Input:       input,
	}
	m.mode = modeNumericInput
	return cmd
}

func (m Model) renderModePicker() string {
	if m.picker == nil || m.picker.OutputIndex < 0 || m.picker.OutputIndex >= len(m.editOutputs) {
		return ""
	}

	output := m.editOutputs[m.picker.OutputIndex]
	body := []string{
		m.styles.subtle.Render(m.pickerPrompt(output.Name)),
		"",
		m.picker.List.View(),
		"",
		m.styles.help.Render("Enter applies. Esc closes."),
	}
	return m.renderModalFrame("Select Mode", body)
}

func (m Model) renderNumericInput() string {
	if m.input == nil {
		return ""
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.input.Input.View())
	body := []string{
		m.styles.subtle.Render(m.input.Hint),
		"",
		m.styles.label.Render("Value"),
		inputBox,
	}
	_, scaleParseErr := parseScaleInput(m.input.Input.Value())
	// Scale explains its own parse errors below; other refusals, such as an
	// overlap, still need saying.
	if m.input.Input.Err != nil && (m.input.Kind != numericInputScale || scaleParseErr == nil) {
		body = append(body, "", m.styles.statusError.Render(m.input.Input.Err.Error()))
	}
	if m.input.Kind == numericInputScale {
		body = append(body, m.scaleInputFeedback()...)
	}
	return m.renderModalFrame(m.input.Title, body)
}

func (m Model) scaleInputFeedback() []string {
	if m.input == nil || m.input.OutputIndex < 0 || m.input.OutputIndex >= len(m.editOutputs) {
		return nil
	}

	output := m.editOutputs[m.input.OutputIndex]
	value, err := parseScaleInput(m.input.Input.Value())
	if err != nil {
		return []string{"", m.styles.statusError.Render(err.Error())}
	}

	logicalW, logicalH := scaling.LogicalSize(output.Width, output.Height, value)
	if scaling.Sharp(output.Width, output.Height, value) {
		return []string{
			"",
			m.styles.statusOK.Render(fmt.Sprintf(
				"Sharp: %d / %s = %d, %d / %s = %d logical px.",
				output.Width,
				scaling.Format(value),
				int(math.Round(logicalW)),
				output.Height,
				scaling.Format(value),
				int(math.Round(logicalH)),
			)),
		}
	}

	suggestion, ok := scaling.ClosestSharp(output.Width, output.Height, value)
	if !ok {
		return []string{
			"",
			m.styles.warning.Render(fmt.Sprintf(
				"⚠ Not sharp: final size has fractional px (%d / %s = %.2f, %d / %s = %.2f).",
				output.Width,
				scaling.Format(value),
				logicalW,
				output.Height,
				scaling.Format(value),
				logicalH,
			)),
		}
	}

	suggestedW, suggestedH := scaling.LogicalSize(output.Width, output.Height, suggestion)
	return []string{
		"",
		m.styles.warning.Render(fmt.Sprintf(
			"⚠ Not sharp: final size has fractional px (%d / %s = %.2f, %d / %s = %.2f).",
			output.Width,
			scaling.Format(value),
			logicalW,
			output.Height,
			scaling.Format(value),
			logicalH,
		)),
		m.styles.statusOK.Render(fmt.Sprintf(
			"Closest sharp: %s -> %d x %d logical px. Enter applies it.",
			scaling.Format(suggestion),
			int(math.Round(suggestedW)),
			int(math.Round(suggestedH)),
		)),
	}
}

func (m *Model) openProfileExecInput() tea.Cmd {
	if len(m.profiles) == 0 || m.selectedProfile < 0 || m.selectedProfile >= len(m.profiles) {
		return nil
	}

	selected := m.profiles[m.selectedProfile]
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 512
	input.Width = clampInt(m.modalMaxWidth()-16, 24, 72)
	input.TextStyle = m.styles.value
	input.PlaceholderStyle = m.styles.subtle
	input.Cursor.Style = lipgloss.NewStyle()
	input.Placeholder = "/path/to/script.sh"
	input.SetValue(selected.Exec)
	cmd := input.Focus()

	m.execInput = &profileExecInputState{
		ProfileIndex: m.selectedProfile,
		Title:        fmt.Sprintf("Post-apply command for %s", selected.Name),
		Input:        input,
	}
	m.mode = modeProfileExecInput
	return cmd
}

func (m Model) renderProfileExecInput() string {
	if m.execInput == nil {
		return ""
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.styles.palette.paneActiveBorder)).
		Padding(0, 1).
		Render(m.execInput.Input.View())
	body := []string{m.styles.label.Render("Post-apply command"), inputBox}
	if m.execInput.Err != nil {
		body = append(body, "", m.styles.statusError.MaxWidth(max(20, m.modalMaxWidth()-6)).Render(m.execInput.Err.Error()))
	}
	body = append(body, "", m.styles.help.MaxWidth(max(20, m.modalMaxWidth()-6)).Render("Enter saves the profile when the command is executable. Leave empty to clear. Esc discards it."))
	return m.renderModalFrame(m.execInput.Title, body)
}

func (m *Model) updateProfileExecInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.execInput == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.execInput = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitProfileExecInput()
	}

	var cmd tea.Cmd
	m.execInput.Input, cmd = m.execInput.Input.Update(msg)
	return m, cmd
}

func (m *Model) commitProfileExecInput() tea.Cmd {
	if m.execInput == nil {
		m.mode = modeMain
		return nil
	}
	if m.execInput.ProfileIndex < 0 || m.execInput.ProfileIndex >= len(m.profiles) {
		m.execInput = nil
		m.mode = modeMain
		return nil
	}

	execValue := strings.TrimSpace(m.execInput.Input.Value())
	if err := apply.ValidatePostApplyExec(execValue); err != nil {
		m.execInput.Err = err
		return nil
	}

	selected := &m.profiles[m.execInput.ProfileIndex]
	selected.Exec = execValue
	if strings.EqualFold(strings.TrimSpace(m.draftProfileName), strings.TrimSpace(selected.Name)) {
		m.draftExec = selected.Exec
	}
	if selected.Exec == "" {
		m.setStatusOK(fmt.Sprintf("Cleared post-apply command for %q", selected.Name))
	} else {
		m.setStatusOK(fmt.Sprintf("Updated post-apply command for %q", selected.Name))
	}
	m.execInput = nil
	m.mode = modeMain
	return m.saveProfileCmd(*selected)
}

func (m *Model) updateModePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.picker = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitModePicker()
	}

	var cmd tea.Cmd
	m.picker.List, cmd = m.picker.List.Update(msg)
	return m, cmd
}

func (m *Model) commitModePicker() tea.Cmd {
	if m.picker == nil {
		m.mode = modeMain
		return nil
	}
	if m.picker.OutputIndex < 0 || m.picker.OutputIndex >= len(m.editOutputs) {
		m.picker = nil
		m.mode = modeMain
		return nil
	}
	selected, ok := m.picker.List.SelectedItem().(interface{ Value() string })
	if !ok {
		m.picker = nil
		m.mode = modeMain
		return nil
	}

	value := selected.Value()
	picker := m.picker
	m.picker = nil
	m.mode = modeMain
	if picker.FieldIndex == 2 {
		if value == scaleCustomValue {
			return m.openScaleEntry()
		}
		m.setInspectorChoice(2, value)
		return nil
	}

	// A new mode or rotation can grow the display into a neighbour; that
	// edit is refused, as the panel's editor refuses it.
	status := ""
	m.guardLayoutEdit(func() {
		output := &m.editOutputs[picker.OutputIndex]
		oldWidth, oldHeight := output.logicalSize()
		if picker.FieldIndex >= 0 {
			m.applyFieldPickerValue(output, picker.FieldIndex, value)
			status = fmt.Sprintf("Set %s to %s for %s", layoutFields[picker.FieldIndex], fieldOptionLabel(picker.FieldIndex, value), output.Name)
		} else {
			output.ModeIndex = indexOf(output.Modes, value)
			if output.ModeIndex < 0 {
				output.ModeIndex = 0
			}
			if output.ModeUnsupported && output.ModeIndex > 0 {
				output.ModeUnsupported = false
			}
			output.applyMode(output.Modes[output.ModeIndex])
			status = fmt.Sprintf("Selected %s for %s", output.DisplayMode(), output.Name)
		}
		m.reflowAfterResize(picker.OutputIndex, oldWidth, oldHeight)
		m.layoutChanged()
		m.setStatusOK(status)
	})
	return nil
}

func (m *Model) applyFieldPickerValue(output *editableOutput, field int, value string) {
	switch field {
	case 3:
		output.Bitdepth, _ = strconv.Atoi(value)
	case 4:
		output.CM = value
	case 5:
		switch value {
		case "on":
			output.VRR = 1
		case "fullscreen":
			output.VRR = 2
		default:
			output.VRR = 0
		}
	case 6:
		for i, label := range []string{"normal", "90", "180", "270", "flipped", "flipped+90", "flipped+180", "flipped+270"} {
			if label == value {
				output.Transform = i
				break
			}
		}
	case 9:
		if value == "None" {
			output.MirrorOf = ""
		} else {
			for _, other := range m.editOutputs {
				if other.displayModelLabel() == value {
					output.MirrorOf = other.Key
					break
				}
			}
		}
	case 14:
		output.SDREOTF = value
	case 18:
		switch value {
		case "off":
			output.SupportsWideColor = -1
		case "on":
			output.SupportsWideColor = 1
		default:
			output.SupportsWideColor = 0
		}
	case 19:
		switch value {
		case "off":
			output.SupportsHDR = -1
		case "on":
			output.SupportsHDR = 1
		default:
			output.SupportsHDR = 0
		}
	}
}

func (m *Model) applyNumericFieldValue(output *editableOutput, field int, value float64) {
	switch field {
	case 10:
		output.SDRBrightness = value
	case 11:
		output.SDRSaturation = value
	case 12:
		output.SDRMinLuminance = value
	case 13:
		output.SDRMaxLuminance = int(value)
	case 15:
		output.MinLuminance = value
	case 16:
		output.MaxLuminance = int(value)
	case 17:
		output.MaxAvgLuminance = int(value)
	}
}

func parseScaleInput(raw string) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("scale is required")
	}
	if strings.HasSuffix(value, ".") {
		return 0, fmt.Errorf("scale must be a number")
	}

	scale, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("scale must be a number")
	}
	if scale < scaling.MinScale || scale > scaling.MaxScale {
		return 0, fmt.Errorf("scale must be between %s and %s", scaling.Format(scaling.MinScale), scaling.Format(scaling.MaxScale))
	}
	return scaling.Round(scale), nil
}

func (m *Model) updateNumericInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.input == nil {
		m.mode = modeMain
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.input = nil
		m.mode = modeMain
		return m, nil
	case "enter":
		return m, m.commitNumericInput()
	}

	var cmd tea.Cmd
	before := m.input.Input.Value()
	m.input.Input, cmd = m.input.Input.Update(msg)
	if m.input.Input.Value() != before {
		m.input.Input.Err = nil
	}
	return m, cmd
}

func (m *Model) commitNumericInput() tea.Cmd {
	if m.input == nil {
		m.mode = modeMain
		return nil
	}
	if m.input.Kind == numericInputWorkspaceCount || m.input.Kind == numericInputWorkspaceGroupSize {
		return m.commitWorkspaceNumericInput()
	}
	if m.input.OutputIndex < 0 || m.input.OutputIndex >= len(m.editOutputs) {
		m.input = nil
		m.mode = modeMain
		return nil
	}

	before := m.currentProfileOutputs()
	original := m.editOutputs[m.input.OutputIndex]
	output := &m.editOutputs[m.input.OutputIndex]
	oldWidth, oldHeight := output.logicalSize()
	var status string
	switch m.input.Kind {
	case numericInputScale:
		typedValue, err := parseScaleInput(m.input.Input.Value())
		if err != nil {
			m.input.Input.Err = err
			return nil
		}
		value := typedValue
		if !scaling.Sharp(output.Width, output.Height, typedValue) {
			if suggestion, ok := scaling.ClosestSharp(output.Width, output.Height, typedValue); ok {
				value = suggestion
			}
		}
		output.Scale = value
		status = fmt.Sprintf("Scale set to %s for %s", scaling.Format(value), output.Name)
		if value != typedValue {
			status += fmt.Sprintf(" (closest sharp scale for %s)", scaling.Format(typedValue))
		} else if !scaling.Sharp(output.Width, output.Height, value) {
			status += " (fractional logical size may look blurry)"
		}
	case numericInputPositionX, numericInputPositionY:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("position must be an integer")
			return nil
		}
		// Typed coordinates are exact: taken as given, or refused when they
		// would overlap another display, and the entry stays open to fix.
		x, y, axis := value, output.Y, "X"
		if m.input.Kind == numericInputPositionY {
			x, y, axis = output.X, value, "Y"
		}
		if output.MirrorOf == "" {
			moved := m.currentProfileOutputs()
			if _, err := profile.MoveOutput(moved, m.input.OutputIndex, x, y, 0); err != nil {
				m.input.Input.Err = err
				return nil
			}
		}
		output.X, output.Y = x, y
		status = fmt.Sprintf("Position %s set to %d for %s", axis, value, output.Name)
	case numericInputICC:
		output.ICC = strings.TrimSpace(m.input.Input.Value())
		if output.ICC == "" {
			status = fmt.Sprintf("ICC profile cleared for %s", output.Name)
		} else {
			status = fmt.Sprintf("ICC profile set for %s", output.Name)
		}
	case numericInputFloat:
		value, err := strconv.ParseFloat(strings.TrimSpace(m.input.Input.Value()), 64)
		if err != nil {
			m.input.Input.Err = fmt.Errorf("must be a number")
			return nil
		}
		m.applyNumericFieldValue(output, m.input.FieldIndex, value)
		status = fmt.Sprintf("%s set for %s", m.input.Title, output.Name)
	case numericInputInt:
		value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
		if err != nil {
			m.input.Input.Err = fmt.Errorf("must be an integer")
			return nil
		}
		m.applyNumericFieldValue(output, m.input.FieldIndex, float64(value))
		status = fmt.Sprintf("%s set for %s", m.input.Title, output.Name)
	}
	m.reflowAfterResize(m.input.OutputIndex, oldWidth, oldHeight)
	if err := profile.RejectNewOverlap(before, m.currentProfileOutputs()); err != nil {
		// A scale that grows the display into a neighbour is refused.
		m.editOutputs[m.input.OutputIndex] = original
		for idx := range m.editOutputs {
			m.editOutputs[idx].X, m.editOutputs[idx].Y = before[idx].X, before[idx].Y
		}
		m.input.Input.Err = err
		return nil
	}
	m.layoutChanged()
	m.setStatusOK(status)
	m.input = nil
	m.mode = modeMain
	return nil
}

func (m *Model) commitWorkspaceNumericInput() tea.Cmd {
	value, err := strconv.Atoi(strings.TrimSpace(m.input.Input.Value()))
	if err != nil || value < 1 {
		m.input.Input.Err = fmt.Errorf("must be a positive integer")
		return nil
	}

	if m.input.Kind == numericInputWorkspaceCount {
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			m.resizeManualWorkspaceRules(value)
		}
		m.workspaceEdit.MaxWorkspaces = value
		m.setStatusOK(fmt.Sprintf("Workspace count set to %d", value))
	} else {
		m.workspaceEdit.GroupSize = value
		m.workspaceEdit.LastSequentialGroupSize = value
		m.setStatusOK(fmt.Sprintf("Workspace group size set to %d", value))
	}
	m.markDirty()
	m.input = nil
	m.mode = modeMain
	return nil
}
