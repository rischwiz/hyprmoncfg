package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

// syntheticEDID builds an EDID with valid checksums for a display named
// name. hdrBlock and colorimetryBlock are CTA-861 extended data block
// payloads (after the extended tag); nil leaves the block out. Nothing here
// comes from a real display.
func syntheticEDID(name string, hdrBlock, colorimetryBlock []byte) []byte {
	checksum := func(block []byte) {
		var sum byte
		for _, b := range block[:127] {
			sum += b
		}
		block[127] = -sum
	}
	base := make([]byte, 128)
	copy(base, []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00})
	base[8], base[9] = 0x04, 0x6D // "ACM"
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
	offset := 4
	for tag, payload := range map[byte][]byte{0x06: hdrBlock, 0x05: colorimetryBlock} {
		if payload == nil {
			continue
		}
		extension[offset] = 7<<5 | byte(len(payload)+1)
		extension[offset+1] = tag
		offset += 2 + copy(extension[offset+2:], payload)
	}
	extension[2] = byte(offset)
	checksum(extension)
	return append(base, extension...)
}

func edidTestModel(t *testing.T, edids map[string][][]byte) Model {
	t.Helper()
	m := paneTestModel(t, tabLayout, []hypr.Monitor{paneTestDesk}, nil)
	m.layoutFocus = layoutFocusInspector
	m.inspectorTab = inspectorTabColor
	m.inspectorField = edidDetectField
	m.readEDIDs = func(connector string) [][]byte { return edids[connector] }
	return m
}

func TestDetectFillsCapabilityFieldsFromTheDisplaysEDID(t *testing.T) {
	// PQ, peak code 139 (1015 cd/m²), frame-average code 115 (604), black code 20; BT.2020.
	data := syntheticEDID("MPG321UR-QD", []byte{0x04, 0x01, 139, 115, 20}, []byte{0x80, 0x00})
	m := edidTestModel(t, map[string][][]byte{"DP-1": {data}})

	detected := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	output := detected.editOutputs[0]
	if output.SupportsHDR != 1 || output.SupportsWideColor != 1 {
		t.Fatalf("HDR %d, wide color %d; want both on", output.SupportsHDR, output.SupportsWideColor)
	}
	if output.MaxLuminance != 1015 || output.MaxAvgLuminance != 604 {
		t.Fatalf("peak %d, frame-average %d; want 1015 and 604", output.MaxLuminance, output.MaxAvgLuminance)
	}
	if output.MinLuminance < 0.06 || output.MinLuminance > 0.065 {
		t.Fatalf("black = %v, want about 0.0625", output.MinLuminance)
	}
	if !detected.dirty || detected.statusErr {
		t.Fatalf("draft dirty %v, status %q", detected.dirty, detected.status)
	}
	for _, want := range []string{"From the EDID of DP-1", "HDR on", "wide color on", "peak 1015", "frame-average 604"} {
		if !strings.Contains(detected.status, want) {
			t.Fatalf("status %q does not mention %q", detected.status, want)
		}
	}
	if strings.Contains(detected.status, "Not stated") {
		t.Fatalf("a complete EDID was reported as incomplete: %q", detected.status)
	}
}

func TestDetectLeavesAloneWhatTheEDIDDoesNotState(t *testing.T) {
	// PQ only: no luminance bytes and no colorimetry block.
	data := syntheticEDID("MPG321UR-QD", []byte{0x04, 0x01}, nil)
	m := edidTestModel(t, map[string][][]byte{"DP-1": {data}})
	m.editOutputs[0].MaxLuminance = 400
	m.editOutputs[0].SupportsWideColor = -1

	detected := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	output := detected.editOutputs[0]
	if output.SupportsHDR != 1 {
		t.Fatalf("HDR = %d, want on", output.SupportsHDR)
	}
	if output.MaxLuminance != 400 || output.SupportsWideColor != -1 {
		t.Fatalf("values the EDID does not state were changed: peak %d, wide color %d", output.MaxLuminance, output.SupportsWideColor)
	}
	if !strings.Contains(detected.status, "Not stated, left as is: wide color, peak, frame-average, black") {
		t.Fatalf("status does not list what was left alone: %q", detected.status)
	}
}

func TestDetectRefusesAnEDIDThatBelongsToAnotherDisplay(t *testing.T) {
	data := syntheticEDID("Other 27", []byte{0x04, 0x01, 139, 115, 20}, []byte{0x80, 0x00})
	m := edidTestModel(t, map[string][][]byte{"DP-1": {data}})

	refused := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	output := refused.editOutputs[0]
	if output.SupportsHDR != 0 || output.MaxLuminance != 0 || refused.dirty {
		t.Fatalf("another display's EDID was applied: %+v", output)
	}
	if !refused.statusErr || !strings.Contains(refused.status, "belongs to another display") {
		t.Fatalf("status = %q", refused.status)
	}
}

func TestDetectSaysWhenThereIsNothingToUse(t *testing.T) {
	for name, tc := range map[string]struct {
		edids [][]byte
		want  string
	}{
		"no EDID":         {nil, "No EDID could be read for DP-1"},
		"corrupt EDID":    {[][]byte{{1, 2, 3}}, "could not be used"},
		"no HDR or color": {[][]byte{syntheticEDID("MPG321UR-QD", nil, nil)}, "says nothing about HDR or wide color"},
	} {
		m := edidTestModel(t, map[string][][]byte{"DP-1": tc.edids})
		got := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
		if !got.statusErr || !strings.Contains(got.status, tc.want) || !strings.Contains(got.status, "nothing was changed") {
			t.Fatalf("%s: status = %q", name, got.status)
		}
		if got.dirty {
			t.Fatalf("%s: the draft was marked changed", name)
		}
	}
}

func TestColorTabShowsTheDetectActionUnderTheCapabilityFields(t *testing.T) {
	m := edidTestModel(t, nil)
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 32}} {
		m.width, m.height = size.width, size.height
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "[Detect]") {
			t.Fatalf("%dx%d: no Detect action on the Color tab:\n%s", size.width, size.height, view)
		}
		if strings.Contains(view, "[Detect ") || strings.Contains(view, "Enter Detect") {
			t.Fatalf("%dx%d: the action label carries a key hint", size.width, size.height)
		}
	}
	fields := inspectorFieldsForTab(inspectorTabColor)
	position := map[int]int{}
	for index, field := range fields {
		position[field] = index
	}
	if position[edidDetectField] != position[19]+1 {
		t.Fatalf("Detect is not directly under HDR capability: %v", fields)
	}

	// Arrow keys on an action change nothing.
	if got := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})); got.dirty {
		t.Fatal("an arrow key on the Detect row marked the draft changed")
	}
}
