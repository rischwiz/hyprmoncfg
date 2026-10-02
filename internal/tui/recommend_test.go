package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/crmne/hyprmoncfg/internal/hypr"
)

func recommendTestModel(t *testing.T) Model {
	t.Helper()
	desk := paneTestDesk
	desk.PhysicalWidth, desk.PhysicalHeight = 597, 336
	desk.AvailableModes = []string{"3840x2160@60.00Hz", "3840x2160@144.00Hz", "2560x1440@144.00Hz"}
	m := paneTestModel(t, tabLayout, []hypr.Monitor{desk}, nil)
	m.layoutFocus = layoutFocusInspector
	return m
}

func TestModePickerMarksTheRecommendedMode(t *testing.T) {
	m := recommendTestModel(t)
	m.inspectorField = 1
	opened := mustModel(t, runModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}))
	if opened.mode != modeModePicker {
		t.Fatalf("mode picker did not open: %v", opened.mode)
	}

	marked := 0
	for _, line := range strings.Split(ansi.Strip(opened.View()), "\n") {
		if !strings.Contains(line, "Recommended") {
			continue
		}
		marked++
		if !strings.Contains(line, "3840x2160@144Hz") {
			t.Fatalf("the wrong mode is marked: %q", line)
		}
	}
	if marked != 1 {
		t.Fatalf("%d modes are marked recommended, want 1", marked)
	}
}

func TestScaleRowNamesTheRecommendationWithoutChangingThePills(t *testing.T) {
	m := recommendTestModel(t)
	output := m.editOutputs[0]

	lines, spans := m.renderScaleRow(output, 60, false, true)
	plain := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "Recommended 1.5x") {
		t.Fatalf("no recommendation under the pills:\n%s", plain)
	}
	for _, span := range spans {
		if strings.Contains(span.value, "Recommended") {
			t.Fatalf("a pill value carries the marker: %q", span.value)
		}
	}
	if strings.Count(lines[0], "Recommended") != 0 {
		t.Fatalf("a pill label carries the marker: %q", ansi.Strip(lines[0]))
	}

	// No trustworthy size, no recommendation, and nothing is drawn for it.
	output.PhysicalWidth, output.PhysicalHeight = 0, 0
	lines, _ = m.renderScaleRow(output, 60, false, true)
	if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "Recommended") {
		t.Fatal("a display without a known size still shows a recommendation")
	}
}

func TestScaleListMarksTheRecommendedScale(t *testing.T) {
	m := recommendTestModel(t)
	m.openScalePicker()

	marked := 0
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "Recommended") {
			marked++
			if !strings.Contains(line, "1.5x") {
				t.Fatalf("the wrong scale is marked: %q", line)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("%d scales are marked recommended, want 1", marked)
	}
}
