package prefs

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadWithoutAFileGivesTheDefaults(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	if err != nil || got != Default() {
		t.Fatalf("Load = %+v, %v; want defaults", got, err)
	}
	if got.PreviewTimeout() != 30*time.Second {
		t.Fatalf("default preview time = %v, want 30s", got.PreviewTimeout())
	}
	if _, err := os.Stat(Path(dir)); !os.IsNotExist(err) {
		t.Fatal("loading created a preferences file")
	}
	if got, err := Load(""); err != nil || got != Default() {
		t.Fatalf("Load without a config directory = %+v, %v", got, err)
	}
}

func TestSaveRoundTripsEveryOfferedPreviewTime(t *testing.T) {
	dir := t.TempDir()
	for _, seconds := range PreviewTimeoutChoices {
		saved, err := Save(dir, Preferences{PreviewTimeoutSeconds: seconds})
		if err != nil {
			t.Fatalf("save %d: %v", seconds, err)
		}
		loaded, err := Load(dir)
		if err != nil || loaded != saved || loaded.PreviewTimeoutSeconds != seconds || loaded.Version != Version {
			t.Fatalf("after saving %d: %+v, %v", seconds, loaded, err)
		}
	}
}

func TestSaveRefusesAPreviewTimeThatIsNotOffered(t *testing.T) {
	dir := t.TempDir()
	if _, err := Save(dir, Preferences{PreviewTimeoutSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	for _, seconds := range []int{0, -1, 10, 45, 86400} {
		if _, err := Save(dir, Preferences{PreviewTimeoutSeconds: seconds}); err == nil {
			t.Fatalf("saved a preview time of %d", seconds)
		}
	}
	if loaded, _ := Load(dir); loaded.PreviewTimeoutSeconds != 60 {
		t.Fatalf("a refused save changed the file: %+v", loaded)
	}
}

func TestLoadFallsBackToDefaultsAndSaysWhy(t *testing.T) {
	for name, content := range map[string]string{
		"not JSON":             "{",
		"a time never offered": `{"version": 1, "preview_timeout_seconds": 7}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(Path(dir), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := Load(dir)
		if err == nil || got != Default() {
			t.Fatalf("%s: Load = %+v, %v; want defaults and an error", name, got, err)
		}
	}
}

func TestLoadKeepsKnownFieldsFromANewerFile(t *testing.T) {
	dir := t.TempDir()
	newer := `{"version": 9, "preview_timeout_seconds": 120, "added_later": true}`
	if err := os.WriteFile(Path(dir), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.PreviewTimeoutSeconds != 120 {
		t.Fatalf("Load = %+v, %v", got, err)
	}
}

func TestSavedFileIsReadable(t *testing.T) {
	dir := t.TempDir()
	if _, err := Save(dir, Default()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"preview_timeout_seconds": 30`) || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("unexpected file:\n%s", data)
	}
}
