package scaling

import "testing"

func TestRecommendPicksAReadablePresetForThePanel(t *testing.T) {
	for _, tc := range []struct {
		name                string
		width, height       int
		physWidth, physHigh int
		want                float64
	}{
		{"27 inch 1440p", 2560, 1440, 597, 336, 1},
		{"24 inch 1080p", 1920, 1080, 531, 299, 1},
		{"27 inch 4K", 3840, 2160, 597, 336, 1.5},
		{"32 inch 4K", 3840, 2160, 710, 400, 1.25},
		{"14 inch 2880x1800 laptop", 2880, 1800, 302, 189, 2},
		{"13 inch 1080p laptop", 1920, 1080, 294, 165, 1.5},
		{"rotated size report", 3840, 2160, 336, 597, 1.5},
	} {
		got, ok := Recommend(tc.width, tc.height, tc.physWidth, tc.physHigh)
		if !ok || got != tc.want {
			t.Errorf("%s: Recommend = %v, %v; want %v", tc.name, got, ok, tc.want)
		}
		if !Sharp(tc.width, tc.height, got) {
			t.Errorf("%s: recommended scale %v is not sharp", tc.name, got)
		}
	}
}

func TestRecommendFallsBackToOneWithoutATrustworthySize(t *testing.T) {
	for _, tc := range []struct {
		name                string
		width, height       int
		physWidth, physHigh int
	}{
		{"no physical size", 1920, 1080, 0, 0},
		{"no mode", 0, 0, 597, 336},
		{"aspect ratio stored as size", 3840, 2160, 16, 9},
		{"projector reporting a wall", 1920, 1080, 2210, 1240},
		{"size that disagrees with the mode", 1920, 1080, 300, 300},
	} {
		got, ok := Recommend(tc.width, tc.height, tc.physWidth, tc.physHigh)
		if ok || got != 1 {
			t.Errorf("%s: Recommend = %v, %v; want 1, false", tc.name, got, ok)
		}
	}
}
