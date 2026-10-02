package profile

import (
	"reflect"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestExtendConnectedPlacesDisplaysAndPlansWorkspaces(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Width: 2880, Height: 1800, Scale: 1.5, X: -1920, Y: 100}
	rotated := hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, Scale: 1, Transform: 1, Y: 100}
	projector := hypr.Monitor{Name: "HDMI-A-1", Disabled: true, AvailableModes: []string{"1920x1080@60.00Hz"}}
	other := hypr.Monitor{Name: "HDMI-A-2", Width: 1280, Height: 720, Scale: 1}
	saved := FromMonitors("desk", []hypr.Monitor{laptop, rotated})
	saved.Workspaces = WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategySequential, GroupSize: 3, MaxWorkspaces: 12}
	original := FromMonitors("desk", []hypr.Monitor{laptop, rotated})
	monitors := []hypr.Monitor{other, projector, rotated, laptop}
	got := ExtendConnected(saved, monitors)
	first, _ := got.OutputByKey(projector.HardwareKey())
	second, _ := got.OutputByKey(other.HardwareKey())
	if !first.Enabled || first.X != 1080 || first.Y != 520 || first.Width != 1920 || first.Scale != 1 {
		t.Fatalf("unexpected projector: %+v", first)
	}
	if !second.Enabled || second.X != 3000 || second.Y != 700 {
		t.Fatalf("unexpected second display: %+v", second)
	}
	rules := ResolveWorkspaceRules(got, monitors)
	if len(rules) != 12 || rules[6].OutputKey != first.Key || rules[9].OutputKey != second.Key {
		t.Fatalf("unexpected workspace plan: %+v", rules)
	}
	if !reflect.DeepEqual(saved.Outputs, original.Outputs) || len(saved.Workspaces.MonitorOrder) != 0 {
		t.Fatal("extension mutated the saved profile")
	}
	if again := ExtendConnected(got, monitors); !reflect.DeepEqual(got, again) {
		t.Fatal("extension is not idempotent")
	}
}

func TestExtendConnectedDefaultsAndExplicitDisable(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1}
	external := hypr.Monitor{Name: "HDMI-A-1", Width: 1920, Height: 1080, Scale: 1, Disabled: true}
	monitors := []hypr.Monitor{laptop, external}
	saved := FromMonitors("laptop", monitors[:1])
	got := ExtendConnected(saved, monitors)
	if !got.Workspaces.Enabled || got.Workspaces.Strategy != WorkspaceStrategySequential || got.Workspaces.GroupSize != 3 {
		t.Fatalf("unexpected defaults: %+v", got.Workspaces)
	}
	external.Disabled, external.X = false, 1920
	if _, ok := ExactStateMatch([]Profile{saved}, []hypr.Monitor{laptop, external}, nil); ok {
		t.Fatal("extended layout should be an unsaved draft")
	}
	saved.DisableUnknownOutputs = true
	if got := ExtendConnected(saved, monitors); !reflect.DeepEqual(got, saved) {
		t.Fatal("explicit disable policy must not extend the profile")
	}
	explicit := FromMonitors("off", monitors)
	if got := ExtendConnected(explicit, monitors); got.Outputs[0].Enabled != explicit.Outputs[0].Enabled || got.Outputs[1].Enabled != explicit.Outputs[1].Enabled {
		t.Fatal("explicitly disabled display was enabled")
	}
}

func TestExtendConnectedPreservesWorkspaceStrategy(t *testing.T) {
	monitors := []hypr.Monitor{{Name: "eDP-1", Width: 1920, Height: 1080, Scale: 1}, {Name: "HDMI-A-1", Width: 1920, Height: 1080, Scale: 1}}
	p := FromMonitors("laptop", monitors[:1])
	p.Workspaces = WorkspaceSettings{Enabled: true, Strategy: WorkspaceStrategyInterleave, MaxWorkspaces: 6}
	got := ExtendConnected(p, monitors)
	rules := ResolveWorkspaceRules(got, monitors)
	if len(rules) != 6 || rules[1].OutputName != "HDMI-A-1" || got.Workspaces.Strategy != WorkspaceStrategyInterleave {
		t.Fatalf("workspace preferences lost: %+v", got.Workspaces)
	}
}

func TestExtendConnectedSelectsAdvertisedPairAndConservativeSignal(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Width: 2880, Height: 1800, Scale: 1.5, X: -1920, Y: -100}
	external := hypr.Monitor{Name: "DP-1", Width: 1920, Height: 1080, Scale: 1, VRR: 1,
		ColorManagementPreset: "hdr", AvailableModes: []string{
			"1920x1080@240Hz", "invalid", "3840x2160@60Hz", "3840x2160@120Hz", "3840x2160@30Hz",
		}}
	saved := FromMonitors("laptop", []hypr.Monitor{laptop})
	got := ExtendConnected(saved, []hypr.Monitor{laptop, external})
	out, _ := got.OutputByKey(external.HardwareKey())
	if out.Width != 3840 || out.Height != 2160 || out.Refresh != 120 || out.X != 0 || out.Y != -580 {
		t.Fatalf("unexpected mode or logical centering: %+v", out)
	}
	if out.VRR != 0 || out.Bitdepth != 8 || out.CM != "srgb" || out.MirrorOf != "" || out.Transform != 0 {
		t.Fatalf("unsafe new-output defaults: %+v", out)
	}
	surviving, _ := got.OutputByKey(laptop.HardwareKey())
	if !reflect.DeepEqual(saved.Outputs[0], surviving) {
		t.Fatal("changed the surviving display")
	}
}

func TestExtendConnectedWithoutSurvivingDisplayStartsAtOrigin(t *testing.T) {
	p := New("draft", nil)
	got := ExtendConnected(p, []hypr.Monitor{{Name: "DP-1", Disabled: true, AvailableModes: []string{"1920x1080@60Hz"}}})
	if len(got.Outputs) != 1 || got.Outputs[0].X != 0 || got.Outputs[0].Y != 0 {
		t.Fatalf("unexpected initial layout: %+v", got.Outputs)
	}
}

func TestExtendConnectedRecommendsAScaleFromThePanelSize(t *testing.T) {
	laptop := hypr.Monitor{Name: "eDP-1", Make: "BOE", Model: "Panel", Width: 1920, Height: 1080, Scale: 1}
	// A 27 inch 4K display the compositor brought up at 1x.
	desk := hypr.Monitor{
		Name: "DP-1", Make: "Acme", Model: "U27", Width: 1920, Height: 1080, Scale: 1,
		PhysicalWidth: 597, PhysicalHeight: 336,
		AvailableModes: []string{"1920x1080@60.00Hz", "3840x2160@60.00Hz", "3840x2160@144.00Hz"},
	}
	// A projector whose reported size says nothing about viewing distance.
	projector := hypr.Monitor{
		Name: "HDMI-A-1", Make: "Acme", Model: "Beam", Width: 1920, Height: 1080, Scale: 1.25,
		PhysicalWidth: 2210, PhysicalHeight: 1240,
		AvailableModes: []string{"1920x1080@60.00Hz"},
	}
	monitors := []hypr.Monitor{laptop, desk, projector}
	saved := FromMonitors("laptop", monitors[:1])

	got := ExtendConnected(saved, monitors)
	byName := map[string]OutputConfig{}
	for _, output := range got.Outputs {
		byName[output.Name] = output
	}

	added := byName["DP-1"]
	if added.Width != 3840 || added.Height != 2160 || added.Refresh != 144 {
		t.Fatalf("mode = %dx%d@%v, want the largest resolution at its highest refresh", added.Width, added.Height, added.Refresh)
	}
	if added.Scale != 1.5 {
		t.Fatalf("scale = %v, want the recommended 1.5 for a 27 inch 4K panel", added.Scale)
	}
	if byName["HDMI-A-1"].Scale != 1.25 {
		t.Fatalf("projector scale = %v, want the compositor's own 1.25 kept", byName["HDMI-A-1"].Scale)
	}
	if byName["eDP-1"].Scale != 1 {
		t.Fatalf("the saved display's scale changed to %v", byName["eDP-1"].Scale)
	}
}

func TestRecommendedModeIsAnAdvertisedPair(t *testing.T) {
	// 144 Hz exists only at 1440p here; the recommendation must not pair it
	// with the 4K resolution.
	mode, width, height, refresh, ok := RecommendedMode([]string{
		"2560x1440@144.00Hz", "3840x2160@60.00Hz", "3840x2160@30.00Hz", "preferred",
	})
	if !ok || mode != "3840x2160@60.00Hz" || width != 3840 || height != 2160 || refresh != 60 {
		t.Fatalf("recommended = %q %dx%d@%v %v", mode, width, height, refresh, ok)
	}
	if _, _, _, _, ok := RecommendedMode(nil); ok {
		t.Fatal("a display that advertises nothing got a recommendation")
	}
}
