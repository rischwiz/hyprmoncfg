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
}

func Default() Preferences {
	return Preferences{Version: Version, PreviewTimeoutSeconds: DefaultPreviewTimeoutSeconds}
}

func (p Preferences) PreviewTimeout() time.Duration {
	return time.Duration(p.PreviewTimeoutSeconds) * time.Second
}

func (p Preferences) Validate() error {
	for _, choice := range PreviewTimeoutChoices {
		if p.PreviewTimeoutSeconds == choice {
			return nil
		}
	}
	return fmt.Errorf("preview time must be one of 15, 30, 60, or 120 seconds, not %d", p.PreviewTimeoutSeconds)
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
