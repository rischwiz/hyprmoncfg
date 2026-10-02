package apply

import (
	"context"
	"fmt"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
	"github.com/crmne/hyprmoncfg/internal/render"
)

// Plan is what Apply would write and run for a profile, without doing either.
type Plan struct {
	MonitorsPath string
	RootPath     string
	Format       config.HyprConfigFormat
	// Rendered is the generated monitor config Apply would write.
	Rendered string
	// WorkspaceCommands are the live commands Apply would send after the
	// reload, computed against the monitors as they are now.
	WorkspaceCommands []string
}

// Plan resolves and renders a profile the way Apply does and stops there. It
// only reads from Hyprland and writes nothing, so it needs no writer lock.
func (e Engine) Plan(ctx context.Context, p profile.Profile, monitors []hypr.Monitor) (Plan, error) {
	p = profile.ExtendConnected(p, monitors)
	if e.Client == nil {
		return Plan{}, fmt.Errorf("nil hypr client")
	}
	if err := ValidateLayout(p.Outputs); err != nil {
		return Plan{}, err
	}

	version, err := query(ctx, e, "version", e.Client.Version)
	if err != nil {
		return Plan{}, err
	}
	supportsV2, err := query(ctx, e, "monitor-v2", e.Client.SupportsMonitorV2)
	if err != nil {
		return Plan{}, err
	}
	resolved, err := config.ResolveHyprlandConfig(firstNonEmpty(version.Version, version.Tag), e.MonitorsConfPath, e.HyprlandConfigPath)
	if err != nil {
		return Plan{}, err
	}
	rendered, err := render.RenderConfig(p, monitors, render.Options{Format: resolved.Format, UseMonitorV2: supportsV2})
	if err != nil {
		return Plan{}, err
	}

	return Plan{
		MonitorsPath:      resolved.MonitorsPath,
		RootPath:          resolved.RootPath,
		Format:            resolved.Format,
		Rendered:          rendered,
		WorkspaceCommands: workspaceCommandsForProfile(p, monitors, resolved.Format == config.HyprConfigLua),
	}, nil
}
