// Package icc finds the ICC display profiles installed on the system, so an
// editor can offer them by name instead of asking for a path.
package icc

import (
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

const (
	headerSize = 128
	// maxProfiles and maxDepth bound a scan of directories this package does
	// not own. maxTags and maxDescription bound what is read from one file.
	maxProfiles    = 200
	maxDepth       = 3
	maxTags        = 200
	maxDescription = 4096
)

// Profile is one installed display profile.
type Profile struct {
	// Path is absolute, as Hyprland's icc setting needs it.
	Path string `json:"path"`
	// Name is the profile's own description, or its file name when it has none.
	Name string `json:"name"`
}

// Dirs lists where ICC profiles are conventionally installed: the person's
// own directories first, then the system's and colord's.
func Dirs() []string {
	var dirs []string
	home, _ := os.UserHomeDir()
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	if dataHome != "" {
		dirs = append(dirs, filepath.Join(dataHome, "icc"))
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".color", "icc"))
	}
	return append(dirs, "/usr/local/share/color/icc", "/usr/share/color/icc", "/var/lib/colord/icc")
}

// List returns the display profiles found under the given directories, by
// name. Files that are not ICC profiles, or are profiles for another kind of
// device such as a printer, are left out. Missing directories are skipped.
func List(dirs []string) []Profile {
	seen := make(map[string]bool)
	var found []Profile
	for _, dir := range dirs {
		root, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if len(found) >= maxProfiles {
				return fs.SkipAll
			}
			if entry.IsDir() {
				if depth(root, path) > maxDepth {
					return fs.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".icc", ".icm":
			default:
				return nil
			}
			// The same file can be reachable through more than one directory.
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || seen[resolved] {
				return nil
			}
			name, ok := displayProfileName(path)
			if !ok {
				return nil
			}
			seen[resolved] = true
			found = append(found, Profile{Path: path, Name: name})
			return nil
		})
	}
	sort.SliceStable(found, func(i, j int) bool {
		if a, b := strings.ToLower(found[i].Name), strings.ToLower(found[j].Name); a != b {
			return a < b
		}
		return found[i].Path < found[j].Path
	})
	return found
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// displayProfileName reads just enough of a file to tell whether it is an ICC
// display profile and what it calls itself.
func displayProfileName(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	header := make([]byte, headerSize+4)
	if _, err := io.ReadFull(file, header); err != nil {
		return "", false
	}
	// Bytes 36 to 39 are the file signature, bytes 12 to 15 the device class.
	if string(header[36:40]) != "acsp" || string(header[12:16]) != "mntr" {
		return "", false
	}

	fallback := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	tags := binary.BigEndian.Uint32(header[headerSize:])
	if tags > maxTags {
		return fallback, true
	}
	table := make([]byte, tags*12)
	if _, err := io.ReadFull(file, table); err != nil {
		return fallback, true
	}
	for offset := 0; offset+12 <= len(table); offset += 12 {
		if string(table[offset:offset+4]) != "desc" {
			continue
		}
		start := binary.BigEndian.Uint32(table[offset+4:])
		size := binary.BigEndian.Uint32(table[offset+8:])
		if size < 12 || size > maxDescription {
			break
		}
		data := make([]byte, size)
		if _, err := file.ReadAt(data, int64(start)); err != nil {
			break
		}
		if name := description(data); name != "" {
			return name, true
		}
		break
	}
	return fallback, true
}

// description decodes a profile description tag: the ASCII textDescription
// of version 2 profiles or the first record of a version 4 multi-localized
// string.
func description(data []byte) string {
	switch string(data[:4]) {
	case "desc":
		length := int(binary.BigEndian.Uint32(data[8:]))
		if length <= 0 || 12+length > len(data) {
			return ""
		}
		return clean(string(data[12 : 12+length]))
	case "mluc":
		if len(data) < 28 || binary.BigEndian.Uint32(data[8:]) == 0 {
			return ""
		}
		length := int(binary.BigEndian.Uint32(data[20:]))
		start := int(binary.BigEndian.Uint32(data[24:]))
		if length <= 0 || length%2 != 0 || start < 0 || start+length > len(data) {
			return ""
		}
		units := make([]uint16, length/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(data[start+2*i:])
		}
		return clean(string(utf16.Decode(units)))
	}
	return ""
}

// clean keeps a description to one printable line.
func clean(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}
