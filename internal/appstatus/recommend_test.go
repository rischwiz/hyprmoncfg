package appstatus

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func TestEditorDisplaysCarryRecommendations(t *testing.T) {
	desk := hypr.Monitor{
		Name: "DP-1", Make: "Acme", Model: "U27", Width: 3840, Height: 2160, RefreshRate: 60, Scale: 1,
		PhysicalWidth: 597, PhysicalHeight: 336,
		AvailableModes: []string{"3840x2160@60.00Hz", "3840x2160@144.00Hz"},
	}
	unknownSize := hypr.Monitor{
		Name: "HDMI-A-1", Make: "Acme", Model: "Beam", Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1,
		AvailableModes: []string{"1920x1080@60.00Hz"},
	}

	document := BuildEditor(nil, []hypr.Monitor{desk, unknownSize}, nil)
	if len(document.Displays) != 2 {
		t.Fatalf("displays = %d", len(document.Displays))
	}
	if got := document.Displays[0]; got.RecommendedMode != "3840x2160@144.00Hz" || got.RecommendedScale != 1.5 {
		t.Fatalf("desk recommendations = %q, %v", got.RecommendedMode, got.RecommendedScale)
	}

	// Without a trustworthy size there is no scale to recommend, and the
	// field is left out instead of reporting a made-up 1x.
	encoded, err := json.Marshal(document.Displays[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "recommended_scale") {
		t.Fatalf("unknown panel size still reported a scale: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"recommended_mode":"1920x1080@60.00Hz"`) {
		t.Fatalf("recommended mode missing: %s", encoded)
	}
}
