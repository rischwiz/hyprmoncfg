package daemon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/icc"
)

// The editor document lists installed profiles for a picker and leaves the
// key out when there are none, so older clients see what they always did.
func TestEditorDocumentListsInstalledICCProfiles(t *testing.T) {
	document := appstatus.BuildEditor(nil, nil, nil)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "icc_profiles") {
		t.Fatalf("an empty profile list was written out: %s", encoded)
	}

	document.ICCProfiles = []icc.Profile{{Path: "/usr/share/color/icc/desk.icc", Name: "Desk Calibrated"}}
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	want := `"icc_profiles":[{"path":"/usr/share/color/icc/desk.icc","name":"Desk Calibrated"}]`
	if !strings.Contains(string(encoded), want) {
		t.Fatalf("missing %s in %s", want, encoded)
	}
}
