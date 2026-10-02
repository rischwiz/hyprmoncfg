package icc

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// testProfile builds a minimal ICC file: a header, a one-entry tag table and
// a description tag. Nothing here comes from a real profile.
func testProfile(class string, descriptionTag []byte) []byte {
	header := make([]byte, headerSize)
	copy(header[12:], class)
	copy(header[36:], "acsp")
	if descriptionTag == nil {
		return append(header, 0, 0, 0, 0)
	}
	table := make([]byte, 4+12)
	binary.BigEndian.PutUint32(table, 1)
	copy(table[4:], "desc")
	binary.BigEndian.PutUint32(table[8:], uint32(headerSize+len(table)))
	binary.BigEndian.PutUint32(table[12:], uint32(len(descriptionTag)))
	return append(append(header, table...), descriptionTag...)
}

func textDescription(text string) []byte {
	tag := make([]byte, 12)
	copy(tag, "desc")
	binary.BigEndian.PutUint32(tag[8:], uint32(len(text)+1))
	return append(append(tag, text...), 0)
}

func multiLocalized(text string) []byte {
	units := utf16.Encode([]rune(text))
	tag := make([]byte, 28)
	copy(tag, "mluc")
	binary.BigEndian.PutUint32(tag[8:], 1)
	binary.BigEndian.PutUint32(tag[12:], 12)
	copy(tag[16:], "enUS")
	binary.BigEndian.PutUint32(tag[20:], uint32(len(units)*2))
	binary.BigEndian.PutUint32(tag[24:], 28)
	for _, unit := range units {
		tag = binary.BigEndian.AppendUint16(tag, unit)
	}
	return tag
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListFindsDisplayProfilesByTheirOwnNames(t *testing.T) {
	user, system := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(user, "desk.icc"), testProfile("mntr", textDescription("Desk Calibrated")))
	writeFile(t, filepath.Join(system, "vendor", "Panel.ICM"), testProfile("mntr", multiLocalized("Acme Panel P3")))
	writeFile(t, filepath.Join(system, "unnamed.icc"), testProfile("mntr", nil))

	// Everything below must be left out.
	writeFile(t, filepath.Join(system, "printer.icc"), testProfile("prtr", textDescription("Office Printer")))
	writeFile(t, filepath.Join(system, "notes.txt"), testProfile("mntr", textDescription("Wrong extension")))
	writeFile(t, filepath.Join(system, "garbage.icc"), []byte("not a profile"))
	writeFile(t, filepath.Join(system, "empty.icc"), nil)
	writeFile(t, filepath.Join(system, "a", "b", "c", "d", "deep.icc"), testProfile("mntr", textDescription("Too Deep")))

	got := List([]string{user, system, filepath.Join(system, "missing")})
	want := []Profile{
		{Path: filepath.Join(system, "vendor", "Panel.ICM"), Name: "Acme Panel P3"},
		{Path: filepath.Join(user, "desk.icc"), Name: "Desk Calibrated"},
		{Path: filepath.Join(system, "unnamed.icc"), Name: "unnamed"},
	}
	if len(got) != len(want) {
		t.Fatalf("List = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List[%d] = %+v, want %+v", i, got[i], want[i])
		}
		if !filepath.IsAbs(got[i].Path) {
			t.Fatalf("path is not absolute: %q", got[i].Path)
		}
	}
}

func TestListShowsAFileReachableTwiceOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real", "desk.icc")
	writeFile(t, real, testProfile("mntr", textDescription("Desk Calibrated")))
	if err := os.MkdirAll(filepath.Join(dir, "links"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "links", "desk.icc")); err != nil {
		t.Fatal(err)
	}

	got := List([]string{filepath.Join(dir, "real"), filepath.Join(dir, "links")})
	if len(got) != 1 || got[0].Path != real {
		t.Fatalf("List = %+v, want the profile once", got)
	}
}

func TestDescriptionsAreKeptToOnePrintableLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "odd.icc"), testProfile("mntr", textDescription("Line one\nLine two\x1b[31m ")))
	// A description tag that claims more text than the file holds.
	broken := textDescription("Short")
	binary.BigEndian.PutUint32(broken[8:], 4000)
	writeFile(t, filepath.Join(dir, "broken.icc"), testProfile("mntr", broken))

	got := List([]string{dir})
	if len(got) != 2 {
		t.Fatalf("List = %+v", got)
	}
	names := map[string]string{}
	for _, profile := range got {
		names[filepath.Base(profile.Path)] = profile.Name
	}
	if names["odd.icc"] != "Line oneLine two[31m" {
		t.Fatalf("control characters survived: %q", names["odd.icc"])
	}
	if names["broken.icc"] != "broken" {
		t.Fatalf("a broken description did not fall back to the file name: %q", names["broken.icc"])
	}
}

func TestDirsPutThePersonsOwnProfilesFirst(t *testing.T) {
	t.Setenv("HOME", "/home/example")
	t.Setenv("XDG_DATA_HOME", "")
	got := Dirs()
	want := []string{
		"/home/example/.local/share/icc", "/home/example/.color/icc",
		"/usr/local/share/color/icc", "/usr/share/color/icc", "/var/lib/colord/icc",
	}
	if len(got) != len(want) {
		t.Fatalf("Dirs = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Dirs[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	t.Setenv("XDG_DATA_HOME", "/data")
	if got := Dirs(); got[0] != "/data/icc" {
		t.Fatalf("XDG_DATA_HOME was not used: %v", got)
	}
}
