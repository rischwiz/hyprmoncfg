package tui

import (
	"math"
	"strconv"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

func editableOutputFromProfile(saved profile.OutputConfig, live hypr.Monitor, hasLive bool) editableOutput {
	output := editableOutput{
		Key:               saved.Key,
		MatchKey:          saved.MatchIdentity(),
		Name:              saved.Name,
		Description:       saved.Description,
		Make:              saved.Make,
		Model:             saved.Model,
		Serial:            saved.Serial,
		Enabled:           saved.Enabled,
		Width:             saved.Width,
		Height:            saved.Height,
		Refresh:           saved.Refresh,
		X:                 saved.X,
		Y:                 saved.Y,
		Scale:             scaling.Round(scaling.Clamp(saved.Scale)),
		VRR:               saved.VRR,
		Transform:         saved.Transform,
		IsInternal:        isInternalOutputName(saved.Name),
		MirrorOf:          saved.MirrorOf,
		Bitdepth:          saved.Bitdepth,
		CM:                saved.CM,
		SDRBrightness:     saved.SDRBrightness,
		SDRSaturation:     saved.SDRSaturation,
		SDRMinLuminance:   saved.SDRMinLuminance,
		SDRMaxLuminance:   saved.SDRMaxLuminance,
		MinLuminance:      saved.MinLuminance,
		MaxLuminance:      saved.MaxLuminance,
		SupportsWideColor: saved.SupportsWideColor,
		SupportsHDR:       saved.SupportsHDR,
		MaxAvgLuminance:   saved.MaxAvgLuminance,
		SDREOTF:           saved.SDREOTF,
		ICC:               saved.ICC,
	}

	mode := saved.NormalizedMode()
	if hasLive {
		output.Description = live.Description
		output.PhysicalWidth = live.PhysicalWidth
		output.PhysicalHeight = live.PhysicalHeight
		output.Focused = live.Focused
		output.DPMSStatus = live.DPMSStatus
		output.IsInternal = live.IsInternal()
		output.ActiveWorkspace = live.ActiveWorkspace.Name
		output.Modes = normalizeModes(live.AvailableModes, mode)
		output.HardwareModes = append([]string(nil), live.AvailableModes...)
		output.ModeUnsupported = len(live.AvailableModes) > 0 && indexOf(live.AvailableModes, mode) < 0
	} else {
		output.Modes = normalizeModes(nil, mode)
	}
	output.ModeIndex = indexOf(output.Modes, mode)
	if output.ModeIndex < 0 {
		output.ModeIndex = 0
	}
	if len(output.Modes) > 0 {
		output.applyMode(output.Modes[output.ModeIndex])
	}
	return output
}

func workspaceEditorFromSettings(settings profile.WorkspaceSettings, outputs []editableOutput) workspaceEditor {
	mirroredKeys := make(map[string]bool)
	for _, output := range outputs {
		if output.MirrorOf != "" {
			mirroredKeys[output.Key] = true
		}
	}

	order := append([]string(nil), settings.MonitorOrder...)
	if len(order) == 0 {
		order = workspaceOrderFromEditorRules(settings.Rules, outputs)
	}
	if len(order) == 0 {
		for _, output := range outputs {
			if output.MirrorOf == "" {
				order = append(order, output.Key)
			}
		}
	}

	seen := make(map[string]bool, len(order))
	normalized := make([]string, 0, len(outputs))
	for _, key := range order {
		if key == "" || seen[key] || mirroredKeys[key] {
			continue
		}
		normalized = append(normalized, key)
		seen[key] = true
	}
	for _, output := range outputs {
		if !seen[output.Key] && !mirroredKeys[output.Key] {
			normalized = append(normalized, output.Key)
			seen[output.Key] = true
		}
	}

	strategy := settings.Strategy
	if strategy == "" {
		if len(settings.Rules) > 0 {
			strategy = profile.WorkspaceStrategyManual
		} else {
			strategy = profile.WorkspaceStrategySequential
		}
	}

	maxWorkspaces := settings.MaxWorkspaces
	if maxWorkspaces <= 0 {
		maxWorkspaces = 9
	}
	if strategy == profile.WorkspaceStrategyManual {
		if inferred := numericManualWorkspaceMaximum(settings.Rules); inferred > 0 {
			maxWorkspaces = inferred
		}
	}
	groupSize := settings.GroupSize
	if groupSize <= 0 {
		groupSize = defaultWorkspaceGroupSize
	}
	lastSequentialGroupSize := groupSize
	if strategy != profile.WorkspaceStrategySequential && lastSequentialGroupSize <= 1 {
		lastSequentialGroupSize = defaultWorkspaceGroupSize
	}

	return workspaceEditor{
		PersistAll:              settings.PersistAll,
		Enabled:                 settings.Enabled,
		Strategy:                strategy,
		MaxWorkspaces:           maxWorkspaces,
		GroupSize:               groupSize,
		LastSequentialGroupSize: lastSequentialGroupSize,
		MonitorOrder:            normalized,
		Rules:                   append([]profile.WorkspaceRule(nil), settings.Rules...),
		ManualRulesInitialized:  strategy == profile.WorkspaceStrategyManual && len(settings.Rules) > 0,
	}
}

func numericManualWorkspaceMaximum(rules []profile.WorkspaceRule) int {
	maximum := 0
	for _, rule := range rules {
		value, err := strconv.Atoi(strings.TrimSpace(rule.Workspace))
		if err == nil && value > maximum {
			maximum = value
		}
	}
	return maximum
}

func workspaceOrderFromEditorRules(rules []profile.WorkspaceRule, outputs []editableOutput) []string {
	if len(rules) == 0 || len(outputs) == 0 {
		return nil
	}

	byName := make(map[string]string, len(outputs))
	byKey := make(map[string]editableOutput, len(outputs))
	for _, output := range outputs {
		byName[output.Name] = output.Key
		byKey[output.Key] = output
	}

	order := make([]string, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		key := strings.TrimSpace(rule.OutputKey)
		if _, ok := byKey[key]; !ok {
			if mapped, ok := byName[strings.TrimSpace(rule.OutputName)]; ok {
				key = mapped
			}
		}
		if key == "" || seen[key] {
			continue
		}
		if output, ok := byKey[key]; ok && output.MirrorOf == "" {
			order = append(order, key)
			seen[key] = true
		}
	}
	return order
}

func (w workspaceEditor) settings() profile.WorkspaceSettings {
	return profile.WorkspaceSettings{
		PersistAll:    w.PersistAll,
		Enabled:       w.Enabled,
		Strategy:      w.Strategy,
		MaxWorkspaces: w.MaxWorkspaces,
		GroupSize:     w.GroupSize,
		MonitorOrder:  append([]string(nil), w.MonitorOrder...),
		Rules:         append([]profile.WorkspaceRule(nil), w.Rules...),
	}
}

func (o *editableOutput) applyMode(mode string) {
	width, height, refresh, ok := hypr.ParseMode(mode)
	if !ok {
		return
	}
	o.Width = width
	o.Height = height
	o.Refresh = refresh
}

func (o editableOutput) DisplayMode() string {
	if len(o.Modes) > 0 && o.ModeIndex >= 0 && o.ModeIndex < len(o.Modes) {
		return strings.TrimSpace(o.Modes[o.ModeIndex])
	}
	return hypr.FormatMode(o.Width, o.Height, o.Refresh)
}

func (o editableOutput) profileOutput() profile.OutputConfig {
	return profile.OutputConfig{
		Key:               o.Key,
		MatchKey:          o.MatchKey,
		Name:              o.Name,
		Description:       o.Description,
		Make:              o.Make,
		Model:             o.Model,
		Serial:            o.Serial,
		Enabled:           o.Enabled,
		Mode:              o.DisplayMode(),
		Width:             o.Width,
		Height:            o.Height,
		Refresh:           o.Refresh,
		X:                 o.X,
		Y:                 o.Y,
		Scale:             scaling.Round(scaling.Clamp(o.Scale)),
		VRR:               o.VRR,
		Transform:         o.Transform,
		MirrorOf:          o.MirrorOf,
		Bitdepth:          o.Bitdepth,
		CM:                o.CM,
		SDRBrightness:     o.SDRBrightness,
		SDRSaturation:     o.SDRSaturation,
		SDRMinLuminance:   o.SDRMinLuminance,
		SDRMaxLuminance:   o.SDRMaxLuminance,
		MinLuminance:      o.MinLuminance,
		MaxLuminance:      o.MaxLuminance,
		SupportsWideColor: o.SupportsWideColor,
		SupportsHDR:       o.SupportsHDR,
		MaxAvgLuminance:   o.MaxAvgLuminance,
		SDREOTF:           o.SDREOTF,
		ICC:               o.ICC,
	}
}

// spatial reports whether the output has a place on the canvas: it is on,
// shows its own image, and has a mode. An enabled output without a mode shows
// nothing, so it gets a row of its own instead of an invisible rectangle.
func (o editableOutput) spatial() bool {
	return o.Enabled && o.MirrorOf == "" && o.Width > 0 && o.Height > 0
}

func (o editableOutput) logicalSize() (int, int) {
	scale := scaling.Round(scaling.Clamp(o.Scale))
	width := int(math.Round(float64(o.Width) / scale))
	height := int(math.Round(float64(o.Height) / scale))
	if o.Transform%2 == 1 {
		width, height = height, width
	}
	return max(1, width), max(1, height)
}

const unknownModelLabel = "(unknown)"

func (o editableOutput) displayModelLabel() string {
	if label := strings.TrimSpace(o.Make + " " + o.Model); label != "" {
		return label
	}
	if model := strings.TrimSpace(o.Model); model != "" {
		return model
	}
	// Hyprland may report a placeholder description (e.g. "mirror-0") for
	// monitors that are actively mirroring. Skip Description in that case.
	if o.MirrorOf == "" {
		if desc := strings.TrimSpace(o.Description); desc != "" {
			return desc
		}
	}
	return unknownModelLabel
}

func isInternalOutputName(name string) bool {
	return hypr.IsInternalConnector(name)
}

// cardLineRole says where a line belongs on a monitor card: identity at the
// top, the mode and placement at the bottom, workspaces as chips on the right.
type cardLineRole int

const (
	cardRoleName cardLineRole = iota
	cardRoleIssue
	cardRoleModel
	cardRoleMode
	cardRolePlacement
	cardRoleWorkspaces
)

type cardLine struct {
	text string
	fg   string
	bg   string
	bold bool
	role cardLineRole
	// workspaces carries the bare IDs behind a workspace line, which the
	// canvas draws as chips; text keeps the shared "1, 2, 3" form.
	workspaces []string
}

func (o editableOutput) cardLines(maxLines int, fg string, muted string) []cardLine {
	return o.cardLinesWithIssue(maxLines, fg, muted, "", "")
}

func (o editableOutput) cardLinesWithIssue(maxLines int, fg string, muted string, issue string, issueFG string) []cardLine {
	if maxLines <= 0 {
		return nil
	}
	name := o.Name
	if issue != "" {
		name += " ⚠"
	}
	lines := []cardLine{{text: name, fg: fg, bold: true, role: cardRoleName}}
	if issue != "" {
		lines = append(lines, cardLine{text: "⚠ " + issue, fg: issueFG, bold: true, role: cardRoleIssue})
	}
	lines = append(lines,
		cardLine{text: o.modelSizeLabel(), fg: muted, role: cardRoleModel},
		cardLine{text: displayModeLabel(o.DisplayMode()), fg: muted, role: cardRoleMode},
		cardLine{text: o.placementLabel(), fg: muted, role: cardRolePlacement})
	return lines[:min(maxLines, len(lines))]
}
