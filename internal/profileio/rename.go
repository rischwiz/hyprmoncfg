package profileio

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crmne/hyprmoncfg/internal/profile"
)

// ErrProfileExists reports that the requested name is taken, either exactly
// or by another name stored in the same file.
var ErrProfileExists = errors.New("profile already exists")

// Rename stores a profile under a new name and removes the old files. The new
// profile and its sidecars are written before anything is removed, so a
// failure leaves the original in place. It never runs the post-apply command.
func Rename(store *profile.Store, oldName string, newName string) error {
	oldName, newName = strings.TrimSpace(oldName), strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return errors.New("profile name is required")
	}
	p, err := store.Load(oldName)
	if err != nil {
		return err
	}
	if p.Name == newName {
		return nil
	}
	// A dotfile manager owns a symlinked profile. Replacing the link with a
	// file under another name would detach it from its source without saying so.
	if info, err := os.Lstat(store.PathsForName(oldName).JSON); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("profile %q is a symlink managed elsewhere; rename it at its source", p.Name)
	}

	sameFile := store.SameFile(oldName, newName)
	if !sameFile {
		if err := requireFree(store, newName); err != nil {
			return err
		}
	}
	p.Name = newName
	if err := SaveWithSidecars(store, p); err != nil {
		return err
	}
	if sameFile {
		return nil
	}
	if err := store.Delete(oldName); err != nil {
		return fmt.Errorf("saved %q but could not remove %q: %w", newName, oldName, err)
	}
	return nil
}

// Duplicate saves a copy of a profile under a new name. The copy is a new
// profile with its own creation time. Its post-apply command is copied only
// when asked, because a command written for one setup can be wrong for another.
func Duplicate(store *profile.Store, sourceName string, newName string, copyExec bool) error {
	sourceName, newName = strings.TrimSpace(sourceName), strings.TrimSpace(newName)
	if sourceName == "" || newName == "" {
		return errors.New("profile name is required")
	}
	p, err := store.Load(sourceName)
	if err != nil {
		return err
	}
	if store.SameFile(sourceName, newName) {
		return fmt.Errorf("%w: %q", ErrProfileExists, p.Name)
	}
	if err := requireFree(store, newName); err != nil {
		return err
	}
	p.Name = newName
	p.CreatedAt = time.Time{}
	if !copyExec {
		p.Exec = ""
	}
	return SaveWithSidecars(store, p)
}

func requireFree(store *profile.Store, name string) error {
	existing, err := store.Load(name)
	if err == nil {
		return fmt.Errorf("%w: %q", ErrProfileExists, existing.Name)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
