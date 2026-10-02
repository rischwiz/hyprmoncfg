package profile

import "github.com/crmne/hyprmoncfg/internal/hypr"

// RecommendedMode picks the mode to suggest for a display from the modes it
// advertises: the largest resolution, at the highest refresh advertised for
// that resolution. It is a supported pair, never two independent maxima.
func RecommendedMode(modes []string) (mode string, width, height int, refresh float64, ok bool) {
	bestArea := 0
	for _, candidate := range modes {
		w, h, hz, parsed := hypr.ParseMode(candidate)
		if !parsed || w <= 0 || h <= 0 || hz <= 0 {
			continue
		}
		if area := w * h; area > bestArea || (area == bestArea && hz > refresh) {
			mode, width, height, refresh, ok = candidate, w, h, hz, true
			bestArea = area
		}
	}
	return mode, width, height, refresh, ok
}
