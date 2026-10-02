package tui

import (
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/scaling"
)

// recommendedSuffix marks the recommended entry in a list. It stays out of the
// Scale pills, whose labels are a shared contract with the panel.
const recommendedSuffix = "  Recommended"

// recommendedMode is the advertised mode to mark, or "" when the display
// advertises none.
func (o editableOutput) recommendedMode() string {
	modes := o.HardwareModes
	if len(modes) == 0 {
		modes = o.Modes
	}
	mode, _, _, _, _ := profile.RecommendedMode(modes)
	return mode
}

// recommendedScale is the readable scale for the current mode. It reports
// false when the panel's physical size is missing or cannot be trusted.
func (o editableOutput) recommendedScale() (float64, bool) {
	return scaling.Recommend(o.Width, o.Height, o.PhysicalWidth, o.PhysicalHeight)
}
