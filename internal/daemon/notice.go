package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/notify"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// newSetupAction is the one action a new-setup notification offers.
const newSetupAction = "Adjust and save…"

// newSetupNotice describes displays that automatic extension has just added
// to a layout that did not know them. It reports false when nothing was added
// or the added displays were already live where the layout puts them, as
// after a daemon restart, so only a real change is announced.
func newSetupNotice(target profile.Profile, effective profile.Profile, monitors []hypr.Monitor, side profile.NewDisplaySide) (notify.Notification, bool) {
	resolver := profile.NewMonitorResolver(monitors)
	placed := make(map[string]profile.OutputConfig, len(effective.Outputs))
	for _, output := range effective.Outputs {
		if live, ok := resolver.ResolveOutput(output); ok {
			placed[live.Name] = output
		}
	}

	var added []hypr.Monitor
	changed := false
	for _, monitor := range profile.OmittedMonitors(target, monitors) {
		output, ok := placed[monitor.Name]
		if !ok || !output.Enabled {
			continue
		}
		added = append(added, monitor)
		if monitor.Disabled || monitor.X != output.X || monitor.Y != output.Y ||
			monitor.Width != output.Width || monitor.Height != output.Height {
			changed = true
		}
	}
	if len(added) == 0 || !changed {
		return notify.Notification{}, false
	}

	summary := fmt.Sprintf("%d displays connected", len(added))
	if len(added) == 1 {
		summary = displayNoticeName(added[0]) + " connected"
	}
	where := map[profile.NewDisplaySide]string{
		profile.NewDisplayLeft:  "to the left of",
		profile.NewDisplayAbove: "above",
		profile.NewDisplayBelow: "below",
	}[side]
	if where == "" {
		where = "to the right of"
	}
	body := fmt.Sprintf("Added %s your layout.", where)
	if name := strings.TrimSpace(target.Name); name != "" && name != "draft" {
		body += fmt.Sprintf(" Your %s profile is unchanged.", name)
	} else {
		body += " Nothing is saved yet."
	}
	return notify.Notification{Summary: summary, Body: body, ActionLabel: newSetupAction}, true
}

func displayNoticeName(monitor hypr.Monitor) string {
	if model := strings.TrimSpace(monitor.Model); model != "" {
		return model
	}
	return monitor.Name
}

// announceNewSetup sends one notification after a verified apply added
// unfamiliar displays. It is called only once the apply has succeeded, sends
// at most once per monitor set, respects the preference, and never blocks the
// event loop on the notification server.
func (s *Service) announceNewSetup(target profile.Profile, effective profile.Profile, monitors []hypr.Monitor, monitorSet string) {
	if s.cfg.Notifier == nil {
		return
	}
	preferences := s.loadPreferences()
	notice, ok := newSetupNotice(target, effective, monitors, preferences.NewDisplaySide)
	if !ok {
		if len(profile.OmittedMonitors(target, monitors)) == 0 {
			// The setup is a known one again; the next unfamiliar one is news.
			s.notifiedSet = ""
		}
		return
	}
	if !preferences.NotifyNewSetup || s.notifiedSet == monitorSet {
		return
	}
	s.notifiedSet = monitorSet

	notifier, logf := s.cfg.Notifier, s.cfg.Logf
	send := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := notifier.Notify(ctx, notice); err != nil {
			logf("new setup notification not delivered: %v", err)
		}
	}
	if s.cfg.NotifyInline {
		send()
		return
	}
	go send()
}
