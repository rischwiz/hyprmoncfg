package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
)

func TestDisplayStatusDoesNotCountModelessDisplaysAsWorking(t *testing.T) {
	var out bytes.Buffer
	writeDisplayStatus(&out, []appstatus.MonitorSummary{
		{Name: "DP-1", Enabled: true, Width: 2560, Height: 1440},
		{Name: "DP-2", Enabled: true, Width: 2560, Height: 1440},
		{Name: "HDMI-A-1", Enabled: true},
		{Name: "DP-3", Enabled: true},
	})
	for _, want := range []string{
		"Displays: 4 enabled (2 without a usable mode), 4 connected\n",
		"Display HDMI-A-1: no usable mode (0x0)\n",
		"Display DP-3: no usable mode (0x0)\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestDisplayStatusExplainsASteppedDownDisplay(t *testing.T) {
	var out bytes.Buffer
	writeDisplayStatus(&out, []appstatus.MonitorSummary{
		{Name: "DP-1", Enabled: true, Width: 2560, Height: 1600},
		{Name: "DP-2", Enabled: true, Width: 3840, Height: 2160, Fallback: &appstatus.MonitorFallback{Reason: "dropping", Running: "at 120 Hz without VRR"}},
	})
	if !strings.HasPrefix(out.String(), "Displays: 2 enabled, 2 connected\n") {
		t.Fatalf("healthy displays should keep the plain summary:\n%s", out.String())
	}
	want := "Display DP-2: kept disconnecting right after connecting at its saved settings, so it runs at 120 Hz without VRR. The saved profile is unchanged; applying a profile tries its saved settings again.\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, out.String())
	}
}

func stubStatusDocument(t *testing.T, document appstatus.Document) {
	t.Helper()
	original := statusDocument
	statusDocument = func(context.Context, string) (appstatus.Document, error) {
		return document, nil
	}
	t.Cleanup(func() { statusDocument = original })
}

func TestMonitorsJSONPrintsTheStatusMonitorList(t *testing.T) {
	stubStatusDocument(t, appstatus.Document{
		Monitors: []appstatus.MonitorSummary{{Name: "DP-1", Enabled: true, Width: 2560, Height: 1440}},
		Profiles: []appstatus.ProfileSummary{{Name: "desk"}},
	})
	dir := t.TempDir()
	cmd := newMonitorsCmd(&dir)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("monitors --json: %v", err)
	}

	var got []appstatus.MonitorSummary
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not a monitor list: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].Name != "DP-1" || got[0].Width != 2560 {
		t.Fatalf("monitors = %+v", got)
	}
}

func TestProfilesJSONPrintsTheStatusProfileList(t *testing.T) {
	stubStatusDocument(t, appstatus.Document{
		Monitors: []appstatus.MonitorSummary{{Name: "DP-1"}},
		Profiles: []appstatus.ProfileSummary{{Name: "desk", MatchScore: 120, Active: true}},
	})
	dir := t.TempDir()
	cmd := newProfilesCmd(&dir)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profiles --json: %v", err)
	}

	var got []appstatus.ProfileSummary
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not a profile list: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].Name != "desk" || got[0].MatchScore != 120 || !got[0].Active {
		t.Fatalf("profiles = %+v", got)
	}
}
