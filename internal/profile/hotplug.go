package profile

import (
	"math"
	"sort"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// NewDisplaySide is where an unfamiliar display joins the existing layout.
type NewDisplaySide string

const (
	NewDisplayRight NewDisplaySide = "right"
	NewDisplayLeft  NewDisplaySide = "left"
	NewDisplayAbove NewDisplaySide = "above"
	NewDisplayBelow NewDisplaySide = "below"
)

// NewDisplayAlignment lines an unfamiliar display up with its neighbour:
// centered on it, or flush with its top edge (beside it) or left edge (above
// or below it).
type NewDisplayAlignment string

const (
	NewDisplayCenter NewDisplayAlignment = "center"
	NewDisplayEdge   NewDisplayAlignment = "edge"
)

// ExtendOptions are the defaults for displays a layout does not know. The
// zero value is the built-in behavior: to the right, centered, VRR off.
type ExtendOptions struct {
	Side      NewDisplaySide
	Alignment NewDisplayAlignment
	VRR       int
}

// ExtendConnected adds displays absent from a saved profile to its right edge.
// The returned layout is ephemeral; the saved profile is never modified.
func ExtendConnected(p Profile, monitors []hypr.Monitor) Profile {
	return ExtendConnectedWith(p, monitors, ExtendOptions{})
}

// ExtendConnectedWith is ExtendConnected with a person's defaults for new
// displays. Only displays the layout omits are affected, so extending an
// already extended layout changes nothing whatever the options.
func ExtendConnectedWith(p Profile, monitors []hypr.Monitor, opts ExtendOptions) Profile {
	if p.DisableUnknownOutputs {
		return p
	}
	p.Outputs = append([]OutputConfig(nil), p.Outputs...)
	p.Workspaces.MonitorOrder = append([]string(nil), p.Workspaces.MonitorOrder...)
	p.Workspaces.Rules = append([]WorkspaceRule(nil), p.Workspaces.Rules...)
	omitted := OmittedMonitors(p, monitors)
	if len(omitted) == 0 {
		return p
	}
	resolver := NewMonitorResolver(monitors)
	// anchor is the display a new one is placed against: the outermost one on
	// the chosen side, then each display just added.
	var anchor struct{ x, y, w, h int }
	found := false
	beyond := func(x, y, w, h int) bool {
		switch opts.Side {
		case NewDisplayLeft:
			return x < anchor.x
		case NewDisplayAbove:
			return y < anchor.y
		case NewDisplayBelow:
			return y+h > anchor.y+anchor.h
		default:
			return x+w > anchor.x+anchor.w
		}
	}
	connected := make([]OutputConfig, 0, len(p.Outputs))
	for _, out := range p.Outputs {
		live, ok := resolver.ResolveOutput(out)
		if !ok {
			continue
		}
		connected = append(connected, out)
		if !out.Enabled || out.MirrorOf != "" {
			continue
		}
		m := hypr.Monitor{Width: out.Width, Height: out.Height, Scale: out.Scale, Transform: out.Transform}
		if m.Width <= 0 || m.Height <= 0 {
			m.Width, m.Height = live.Width, live.Height
		}
		w, h := m.LogicalSize()
		if !found || beyond(out.X, out.Y, w, h) {
			anchor.x, anchor.y, anchor.w, anchor.h, found = out.X, out.Y, w, h, true
		}
	}
	p.Outputs = connected
	if len(p.Workspaces.MonitorOrder) == 0 {
		for _, idx := range outputIndicesByLayout(p.Outputs) {
			if p.Outputs[idx].Enabled && p.Outputs[idx].MirrorOf == "" {
				p.Workspaces.MonitorOrder = append(p.Workspaces.MonitorOrder, p.Outputs[idx].Key)
			}
		}
	}
	sort.SliceStable(omitted, func(i, j int) bool {
		if omitted[i].IsInternal() != omitted[j].IsInternal() {
			return omitted[i].IsInternal()
		}
		return omitted[i].Name < omitted[j].Name
	})
	counts := hypr.MonitorMatchCounts(monitors)
	for _, m := range omitted {
		// Choose a supported pair, not independent resolution/refresh maxima.
		bestArea, bestRefresh := 0, 0.0
		for _, mode := range m.AvailableModes {
			if w, h, hz, ok := hypr.ParseMode(mode); ok && w > 0 && h > 0 && hz > 0 {
				if area := w * h; area > bestArea || (area == bestArea && hz > bestRefresh) {
					m.Width, m.Height, m.RefreshRate = w, h, hz
					bestArea, bestRefresh = area, hz
				}
			}
		}
		if m.Scale <= 0 {
			m.Scale = 1
		}
		m.Disabled, m.MirrorOf, m.Transform = false, "", 0
		w, h := m.LogicalSize()
		m.X, m.Y = 0, 0
		if found {
			// Offset along the shared edge: centered, or flush with it.
			along := func(anchorSize, size int) int {
				if opts.Alignment == NewDisplayEdge {
					return 0
				}
				return int(math.Round(float64(anchorSize-size) / 2))
			}
			switch opts.Side {
			case NewDisplayLeft:
				m.X, m.Y = anchor.x-w, anchor.y+along(anchor.h, h)
			case NewDisplayAbove:
				m.X, m.Y = anchor.x+along(anchor.w, w), anchor.y-h
			case NewDisplayBelow:
				m.X, m.Y = anchor.x+along(anchor.w, w), anchor.y+anchor.h
			default:
				m.X, m.Y = anchor.x+anchor.w, anchor.y+along(anchor.h, h)
			}
		}
		out := FromMonitors("draft", []hypr.Monitor{m}).Outputs[0]
		out.VRR, out.Bitdepth, out.CM = opts.VRR, 8, "srgb"
		out.Key = hypr.MonitorOutputKey(m, counts)
		p.Outputs = append(p.Outputs, out)
		p.Workspaces.MonitorOrder = append(p.Workspaces.MonitorOrder, out.Key)
		anchor.x, anchor.y, anchor.w, anchor.h, found = m.X, m.Y, w, h, true
	}
	if !p.Workspaces.Enabled {
		p.Workspaces.Enabled = true
		p.Workspaces.Strategy = WorkspaceStrategySequential
		p.Workspaces.GroupSize = 3
		p.Workspaces.MaxWorkspaces = 9
	}
	p.Normalize()
	return p
}
