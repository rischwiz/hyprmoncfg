package daemon

import (
	"testing"

	"github.com/crmne/hyprmoncfg/internal/appstatus"
	"github.com/crmne/hyprmoncfg/internal/hypr"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// daemonTestEDID is a synthetic EDID with valid checksums for a display named
// name that advertises PQ with a peak luminance code of 139 (1015 cd/m²).
func daemonTestEDID(name string) []byte {
	checksum := func(block []byte) {
		var sum byte
		for _, b := range block[:127] {
			sum += b
		}
		block[127] = -sum
	}
	base := make([]byte, 128)
	copy(base, []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00})
	base[54], base[55] = 0x01, 0x1D
	base[72+3] = 0xFC
	field := base[72+5 : 72+18]
	for i := range field {
		field[i] = ' '
	}
	copy(field, name)
	if len(name) < len(field) {
		field[len(name)] = '\n'
	}
	base[126] = 1
	checksum(base)

	extension := make([]byte, 128)
	extension[0], extension[1] = 0x02, 3
	copy(extension[4:], []byte{7<<5 | 4, 0x06, 0x04, 0x01, 139})
	extension[2] = 9
	checksum(extension)
	return append(base, extension...)
}

func TestEditorDocumentCarriesEDIDColorOnlyForTheDisplayItDescribes(t *testing.T) {
	desk := hypr.Monitor{Name: "DP-1", Make: "Acme", Model: "Test HDR 27", Width: 3840, Height: 2160, Scale: 1}
	side := hypr.Monitor{Name: "DP-2", Make: "Acme", Model: "Side 24", Width: 1920, Height: 1080, Scale: 1, X: 3840}
	plain := hypr.Monitor{Name: "HDMI-A-1", Make: "Acme", Model: "Plain", Width: 1920, Height: 1080, Scale: 1, X: 5760}
	monitors := []hypr.Monitor{desk, side, plain}

	svc := New(nil, profile.NewStore(t.TempDir()), Config{})
	svc.readEDIDs = func(connector string) [][]byte {
		switch connector {
		case "DP-1":
			return [][]byte{daemonTestEDID("Test HDR 27")}
		case "DP-2":
			// The EDID on this connector describes some other display.
			return [][]byte{daemonTestEDID("Somebody Else")}
		}
		return nil
	}

	document := appstatus.BuildEditor(nil, monitors, nil)
	svc.attachEDIDColor(&document, monitors)

	color := document.Displays[0].EDIDColor
	if color == nil || color.HDR == nil || !*color.HDR || color.MaxLuminance == nil || *color.MaxLuminance != 1015 {
		t.Fatalf("DP-1 EDID color = %+v", color)
	}
	if document.Displays[1].EDIDColor != nil {
		t.Fatalf("another display's EDID was attached to DP-2: %+v", document.Displays[1].EDIDColor)
	}
	if document.Displays[2].EDIDColor != nil {
		t.Fatalf("a display with no EDID got one: %+v", document.Displays[2].EDIDColor)
	}
}
