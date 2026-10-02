// Package prefs holds application preferences: choices about how hyprmoncfg
// behaves that belong to the person, not to any one profile.
package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/config"
	"github.com/crmne/hyprmoncfg/internal/profile"
)

// Version is the schema this build writes. A file from a newer build keeps
// working: unknown fields are ignored and known ones are validated as usual.
const Version = 1

const fileName = "preferences.json"

// DefaultPreviewTimeoutSeconds is how long a preview waits for Keep before it
// reverts when no preference is saved.
const DefaultPreviewTimeoutSeconds = 30

// PreviewTimeoutChoices are the confirmation times a person can pick.
var PreviewTimeoutChoices = []int{15, 30, 60, 120}

type Preferences struct {
	Version int `json:"version"`
	// PreviewTimeoutSeconds is the confirmation time used when a client does
	// not ask for one. A client that sends its own duration still gets it.
	PreviewTimeoutSeconds int `json:"preview_timeout_seconds"`
	// NewDisplaySide, NewDisplayAlignment and NewDisplayVRR are the defaults
	// for a display no layout knows yet. They never change a saved profile or
	// a display a layout already places.
	NewDisplaySide      profile.NewDisplaySide      `json:"new_display_side"`
	NewDisplayAlignment profile.NewDisplayAlignment `json:"new_display_alignment"`
	NewDisplayVRR       int                         `json:"new_display_vrr"`
	// NotifyNewSetup asks for one desktop notification when an unfamiliar
	// setup has been extended automatically.
	NotifyNewSetup bool `json:"notify_new_setup"`
}

func Default() Preferences {
	return Preferences{
		Version:               Version,
		PreviewTimeoutSeconds: DefaultPreviewTimeoutSeconds,
		NewDisplaySide:        profile.NewDisplayRight,
		NewDisplayAlignment:   profile.NewDisplayCenter,
		NewDisplayVRR:         0,
		NotifyNewSetup:        true,
	}
}

// ExtendOptions are these preferences as automatic extension uses them.
func (p Preferences) ExtendOptions() profile.ExtendOptions {
	return profile.ExtendOptions{Side: p.NewDisplaySide, Alignment: p.NewDisplayAlignment, VRR: p.NewDisplayVRR}
}

func (p Preferences) PreviewTimeout() time.Duration {
	return time.Duration(p.PreviewTimeoutSeconds) * time.Second
}

func (p Preferences) Validate() error {
	previewOK := false
	for _, choice := range PreviewTimeoutChoices {
		previewOK = previewOK || p.PreviewTimeoutSeconds == choice
	}
	if !previewOK {
		return fmt.Errorf("preview time must be one of 15, 30, 60, or 120 seconds, not %d", p.PreviewTimeoutSeconds)
	}
	switch p.NewDisplaySide {
	case profile.NewDisplayRight, profile.NewDisplayLeft, profile.NewDisplayAbove, profile.NewDisplayBelow:
	default:
		return fmt.Errorf("new display side must be right, left, above, or below, not %q", p.NewDisplaySide)
	}
	switch p.NewDisplayAlignment {
	case profile.NewDisplayCenter, profile.NewDisplayEdge:
	default:
		return fmt.Errorf("new display alignment must be center or edge, not %q", p.NewDisplayAlignment)
	}
	if p.NewDisplayVRR < 0 || p.NewDisplayVRR > 2 {
		return fmt.Errorf("new display VRR must be 0 (off), 1 (on), or 2 (fullscreen), not %d", p.NewDisplayVRR)
	}
	return nil
}

// Path is where preferences are stored under the hyprmoncfg config directory.
func Path(baseDir string) string {
	return filepath.Join(baseDir, fileName)
}

// Load reads saved preferences. A missing file, or no config directory, gives
// the defaults. A file that cannot be read or holds a value this build does
// not accept also gives the defaults, with an error saying why, so a damaged
// file never leaves a preview without a deadline.
func Load(baseDir string) (Preferences, error) {
	if strings.TrimSpace(baseDir) == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(Path(baseDir))
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	loaded := Default()
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Default(), fmt.Errorf("read %s: %w", Path(baseDir), err)
	}
	if err := loaded.Validate(); err != nil {
		return Default(), fmt.Errorf("%s: %w", Path(baseDir), err)
	}
	loaded.Version = Version
	return loaded, nil
}

// Save validates and writes preferences, replacing the file in one step.
func Save(baseDir string, p Preferences) (Preferences, error) {
	if strings.TrimSpace(baseDir) == "" {
		return Preferences{}, errors.New("no config directory to save preferences in")
	}
	if err := p.Validate(); err != nil {
		return Preferences{}, err
	}
	p.Version = Version
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Preferences{}, err
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return Preferences{}, err
	}
	if err := config.WriteFileAtomic(Path(baseDir), append(data, '\n'), 0o644); err != nil {
		return Preferences{}, err
	}
	return p, nil
}
