package hypr

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxEDIDBytes bounds what is read from sysfs: a base block and up to 255
// extensions is the most an EDID can be.
const maxEDIDBytes = 256 * 128

// ConnectorEDIDs returns the raw EDID of every DRM connector with this name.
// More than one graphics card can expose the same connector name, so callers
// get every candidate and decide which one describes their display. A
// connector with nothing plugged in has an empty EDID and is left out.
func ConnectorEDIDs(connector string) [][]byte {
	return connectorEDIDs(defaultDRMSysRoot, connector)
}

func connectorEDIDs(sysRoot string, connector string) [][]byte {
	connector = strings.TrimSpace(connector)
	if connector == "" || strings.ContainsAny(connector, "/*?[") {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(sysRoot, "card*-"+connector))
	if err != nil {
		return nil
	}
	sort.Strings(matches)

	var found [][]byte
	for _, dir := range matches {
		// The glob also matches longer names that end the same way, such as
		// card0-HDMI-A-1 for a request for A-1.
		if _, name, ok := strings.Cut(filepath.Base(dir), "-"); !ok || name != connector {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "edid"))
		if err != nil || len(data) == 0 || len(data) > maxEDIDBytes {
			continue
		}
		found = append(found, data)
	}
	return found
}
