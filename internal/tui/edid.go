package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/crmne/hyprmoncfg/internal/edid"
	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// edidDetectField is the Color tab's action row. It has no value of its own:
// activating it fills the display capability fields above it.
const (
	edidDetectField = 21
	edidDetectLabel = "[Detect]"
)

// detectEDIDColor fills the selected display's HDR and color capability
// fields from its own EDID. It changes the draft only, leaves alone whatever
// the EDID does not state, and refuses an EDID that belongs to another
// display: a connector name is not identity.
func (m *Model) detectEDIDColor() {
	if len(m.editOutputs) == 0 {
		return
	}
	output := &m.editOutputs[m.selectedOutput]
	read := m.readEDIDs
	if read == nil {
		read = hypr.ConnectorEDIDs
	}
	info, err := edid.ForMonitor(read(output.Name), output.Model, output.Serial)
	switch {
	case errors.Is(err, edid.ErrNoEDID):
		m.setStatusErr(fmt.Sprintf("No EDID could be read for %s; nothing was changed", output.Name))
		return
	case errors.Is(err, edid.ErrOtherDisplay):
		m.setStatusErr(fmt.Sprintf("The EDID on %s belongs to another display; nothing was changed", output.Name))
		return
	case err != nil:
		m.setStatusErr(fmt.Sprintf("The EDID of %s could not be used (%v); nothing was changed", output.Name, err))
		return
	}

	color := info.Color
	if color.Empty() {
		m.setStatusErr(fmt.Sprintf("The EDID of %s says nothing about HDR or wide color; nothing was changed", output.Name))
		return
	}

	triState := func(supported bool) int {
		if supported {
			return 1
		}
		return -1
	}
	onOff := func(supported bool) string {
		if supported {
			return "on"
		}
		return "off"
	}
	var filled, missing []string
	if color.HDR != nil {
		output.SupportsHDR = triState(*color.HDR)
		filled = append(filled, "HDR "+onOff(*color.HDR))
	} else {
		missing = append(missing, "HDR")
	}
	if color.WideColor != nil {
		output.SupportsWideColor = triState(*color.WideColor)
		filled = append(filled, "wide color "+onOff(*color.WideColor))
	} else {
		missing = append(missing, "wide color")
	}
	if color.MaxLuminance != nil {
		output.MaxLuminance = *color.MaxLuminance
		filled = append(filled, fmt.Sprintf("peak %d", *color.MaxLuminance))
	} else {
		missing = append(missing, "peak")
	}
	if color.MaxAvgLuminance != nil {
		output.MaxAvgLuminance = *color.MaxAvgLuminance
		filled = append(filled, fmt.Sprintf("frame-average %d", *color.MaxAvgLuminance))
	} else {
		missing = append(missing, "frame-average")
	}
	if color.MinLuminance != nil {
		output.MinLuminance = *color.MinLuminance
		filled = append(filled, fmt.Sprintf("black %s", displayNumber(*color.MinLuminance, 4)))
	} else {
		missing = append(missing, "black")
	}

	m.layoutChanged()
	status := fmt.Sprintf("From the EDID of %s: %s", output.Name, strings.Join(filled, ", "))
	if len(missing) > 0 {
		status += ". Not stated, left as is: " + strings.Join(missing, ", ")
	}
	m.setStatusOK(status)
}
