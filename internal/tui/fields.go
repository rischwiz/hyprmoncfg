package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

func (m Model) layoutFieldValue(output editableOutput, field int) string {
	switch field {
	case 0:
		return boolText(output.Enabled)
	case 1:
		return displayModeLabel(output.DisplayMode())
	case 2:
		return scaling.Format(output.Scale)
	case 3:
		if output.Bitdepth == 0 {
			return "8"
		}
		return fmt.Sprintf("%d", output.Bitdepth)
	case 4:
		if output.CM == "" {
			return "srgb"
		}
		return output.CM
	case 5:
		return vrrLabel(output.VRR)
	case 6:
		return transformLabel(output.Transform)
	case 7:
		return fmt.Sprintf("%d", output.X)
	case 8:
		return fmt.Sprintf("%d", output.Y)
	case 9:
		if output.MirrorOf == "" {
			return "None"
		}
		for _, other := range m.editOutputs {
			if other.Key == output.MirrorOf {
				return other.displayModelLabel()
			}
		}
		return output.MirrorOf
	case 10:
		return fmt.Sprintf("%.2f", sdrMultiplier(output.SDRBrightness))
	case 11:
		return fmt.Sprintf("%.2f", sdrMultiplier(output.SDRSaturation))
	case 12:
		return fmt.Sprintf("%.3f", output.SDRMinLuminance)
	case 13:
		return fmt.Sprintf("%d", output.SDRMaxLuminance)
	case 14:
		if output.SDREOTF == "" {
			return "default"
		}
		return output.SDREOTF
	case 15:
		return fmt.Sprintf("%.3f", output.MinLuminance)
	case 16:
		return fmt.Sprintf("%d", output.MaxLuminance)
	case 17:
		return fmt.Sprintf("%d", output.MaxAvgLuminance)
	case 18:
		return triStateLabel(output.SupportsWideColor)
	case 19:
		return triStateLabel(output.SupportsHDR)
	case 20:
		if output.ICC == "" {
			return "None"
		}
		return output.ICC
	case edidDetectField:
		return edidDetectLabel
	default:
		return ""
	}
}

func (m Model) layoutFieldIssue(output editableOutput, field int) (string, bool) {
	switch field {
	case 1:
		if output.ModeUnsupported || output.ModeIndex < 0 || (len(output.Modes) > 0 && output.ModeIndex >= len(output.Modes)) {
			return "unsupported", true
		}
	case 2:
		if output.Enabled && !scaling.Sharp(output.Width, output.Height, output.Scale) {
			return "fractional px", true
		}
	case 3:
		if output.Bitdepth != 0 && output.Bitdepth != 8 && output.Bitdepth != 10 {
			return "invalid", true
		}
	case 4:
		if !validStringOption(output.CM, "", "srgb", "auto", "wide", "hdr", "hdredid", "dcip3", "dp3", "adobe", "edid") {
			return "invalid", true
		}
	case 5:
		if output.VRR < 0 || output.VRR > 2 {
			return "invalid", true
		}
	case 6:
		if output.Transform < 0 || output.Transform > 7 {
			return "invalid", true
		}
	case 9:
		if output.MirrorOf != "" {
			if output.MirrorOf == output.Key {
				return "self mirror", true
			}
			if !m.outputKeyExists(output.MirrorOf) {
				return "missing target", true
			}
		}
	case 10:
		if output.SDRBrightness < 0 || output.SDRBrightness > 3 {
			return "out of range", true
		}
	case 11:
		if output.SDRSaturation < 0 || output.SDRSaturation > 3 {
			return "out of range", true
		}
	case 12:
		if output.SDRMinLuminance < 0 || output.SDRMinLuminance > 1 {
			return "out of range", true
		}
	case 13:
		if output.SDRMaxLuminance < 0 || output.SDRMaxLuminance > 1000 {
			return "out of range", true
		}
	case 14:
		if !validStringOption(output.SDREOTF, "", "default", "gamma22", "srgb") {
			return "invalid", true
		}
	case 15:
		if output.MinLuminance < 0 || output.MinLuminance > 1000 {
			return "out of range", true
		}
	case 16:
		if output.MaxLuminance < 0 || output.MaxLuminance > 2000 {
			return "out of range", true
		}
	case 17:
		if output.MaxAvgLuminance < 0 || output.MaxAvgLuminance > 2000 {
			return "out of range", true
		}
	case 18:
		if output.SupportsWideColor < -1 || output.SupportsWideColor > 1 {
			return "invalid", true
		}
	case 19:
		if output.SupportsHDR < -1 || output.SupportsHDR > 1 {
			return "invalid", true
		}
	case 20:
		icc := strings.TrimSpace(output.ICC)
		if icc != "" && !filepath.IsAbs(icc) {
			return "needs abs path", true
		}
	}
	return "", false
}

func (m Model) outputKeyExists(key string) bool {
	for _, output := range m.editOutputs {
		if output.Key == key {
			return true
		}
	}
	return false
}

func validStringOption(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, option := range allowed {
		if value == option {
			return true
		}
	}
	return false
}

func (m Model) workspaceFieldValue(field int) string {
	if field > 0 && !m.workspaceEdit.Enabled {
		return "—"
	}
	switch field {
	case 3:
		if m.workspaceEdit.Strategy == profile.WorkspaceStrategyManual {
			return "Custom (per rule)"
		}
		if m.workspaceEdit.PersistAll {
			return "All assigned"
		}
		return "First per display"
	case 0:
		return workspaceStrategyLabel(m.workspaceStrategyChoice())
	case 1:
		return fmt.Sprintf("%d", m.workspaceEdit.MaxWorkspaces)
	case 2:
		if m.workspaceEdit.Strategy != profile.WorkspaceStrategySequential {
			return "—"
		}
		return fmt.Sprintf("%d", m.workspaceEdit.GroupSize)
	default:
		return ""
	}
}

var layoutFields = []string{
	"Enabled",
	"Mode",
	"Scale",
	"Color depth (bpc)",
	"Color space / EOTF",
	"VRR",
	"Rotation",
	"Position X",
	"Position Y",
	"Mirror",
	"SDR luminance scale",
	"SDR saturation scale",
	"SDR black level (cd/m²)",
	"SDR white level (cd/m²)",
	"SDR EOTF",
	"Display black (cd/m²)",
	"Display peak (cd/m²)",
	"Max frame-average (cd/m²)",
	"WCG capability",
	"HDR capability",
	"ICC device profile",
	"Capabilities from EDID",
}

const advancedFieldStart = 10

func layoutFieldShortLabel(field int) string {
	switch field {
	case 0:
		// "On" would read as a value beside the On/Off choice row.
		return "Enabled"
	case 3:
		return "Depth (bpc)"
	case 4:
		return "Space/EOTF"
	case 10:
		return "SDR lum. x"
	case 11:
		return "SDR sat. x"
	case 12:
		return "SDR black"
	case 13:
		return "SDR white"
	case 15:
		return "Disp. black"
	case 16:
		return "Disp. peak"
	case 17:
		return "Frame avg."
	case 18:
		return "WCG cap."
	case 19:
		return "HDR cap."
	case 20:
		return "ICC profile"
	case edidDetectField:
		return "From EDID"
	case 6:
		return "Rot"
	case 7:
		return "X"
	case 8:
		return "Y"
	case 9:
		return "Mirror"
	default:
		return layoutFields[field]
	}
}

var workspaceFields = []string{
	"Strategy",
	"Max workspaces",
	"Group size",
	"Persistence",
}

func (m Model) isOutputOverlapping(o editableOutput) bool {
	if !o.Enabled || o.MirrorOf != "" {
		return false
	}
	w1, h1 := o.logicalSize()
	x1_1, y1_1 := o.X, o.Y
	x2_1, y2_1 := o.X+w1, o.Y+h1

	for _, other := range m.editOutputs {
		if other.Name == o.Name || !other.Enabled || other.MirrorOf != "" {
			continue
		}

		w2, h2 := other.logicalSize()
		x1_2, y1_2 := other.X, other.Y
		x2_2, y2_2 := other.X+w2, other.Y+h2

		if x1_1 < x2_2 && x2_1 > x1_2 &&
			y1_1 < y2_2 && y2_1 > y1_2 {
			return true
		}
	}
	return false
}
