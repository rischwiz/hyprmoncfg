package hypr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConnectorEDIDsReadsEveryCardWithThatConnector(t *testing.T) {
	root := t.TempDir()
	write := func(dir string, content []byte) {
		t.Helper()
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if content != nil {
			if err := os.WriteFile(filepath.Join(path, "edid"), content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("card0-DP-1", []byte("first"))
	write("card1-DP-1", []byte("second"))
	write("card0-DP-2", []byte("other connector"))
	write("card0-HDMI-A-1", []byte{}) // nothing plugged in
	write("card1-eDP-1", nil)         // no edid file at all
	write("card0-Virtual-DP-1", []byte("a longer name that ends the same way"))

	got := connectorEDIDs(root, "DP-1")
	if len(got) != 2 || string(got[0]) != "first" || string(got[1]) != "second" {
		t.Fatalf("EDIDs for DP-1 = %q", got)
	}
	for _, connector := range []string{"HDMI-A-1", "eDP-1", "DP-9", "", " ", "../etc", "DP-*"} {
		if got := connectorEDIDs(root, connector); len(got) != 0 {
			t.Fatalf("EDIDs for %q = %q, want none", connector, got)
		}
	}
}
