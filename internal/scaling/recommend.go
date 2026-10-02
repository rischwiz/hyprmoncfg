package scaling

import "math"

const (
	// recommendTargetDensity is the logical pixels per inch a recommended
	// scale aims for. It is a starting point, not a measured threshold.
	recommendTargetDensity = 110.0
	// Panels outside this diagonal range are treated as unknown. Projectors
	// and many TVs report a placeholder or a size that says nothing about how
	// far away people sit.
	recommendMinDiagonalInches = 5.0
	recommendMaxDiagonalInches = 60.0
	// recommendMaxAspectError rejects physical sizes whose shape disagrees
	// with the mode, such as an aspect ratio stored where millimetres belong.
	recommendMaxAspectError = 0.15
)

// Recommend suggests a readable sharp scale for a mode on a panel of the
// reported physical size: the preset closest to the target density, the lower
// one on a tie so text is never larger than needed. It reports false, with a
// scale of 1, when the size is missing or implausible.
func Recommend(width, height, physicalWidthMM, physicalHeightMM int) (float64, bool) {
	if width <= 0 || height <= 0 || physicalWidthMM <= 0 || physicalHeightMM <= 0 {
		return 1, false
	}
	pixelAspect := float64(width) / float64(height)
	physicalAspect := float64(physicalWidthMM) / float64(physicalHeightMM)
	// A rotated panel can report its size in either orientation.
	if math.Abs(pixelAspect-physicalAspect)/pixelAspect > recommendMaxAspectError &&
		math.Abs(pixelAspect-1/physicalAspect)/pixelAspect > recommendMaxAspectError {
		return 1, false
	}
	diagonalInches := math.Hypot(float64(physicalWidthMM), float64(physicalHeightMM)) / 25.4
	if diagonalInches < recommendMinDiagonalInches || diagonalInches > recommendMaxDiagonalInches {
		return 1, false
	}

	wanted := math.Hypot(float64(width), float64(height)) / diagonalInches / recommendTargetDensity
	best, bestDistance := 1.0, math.Inf(1)
	for _, choice := range PresetChoices(width, height) {
		if distance := math.Abs(choice - wanted); distance < bestDistance {
			best, bestDistance = choice, distance
		}
	}
	return best, true
}
