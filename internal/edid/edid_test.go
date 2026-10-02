package edid

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

// testEDID builds a synthetic EDID with valid checksums. Nothing here comes
// from a real display.
type testEDID struct {
	manufacturer string
	product      uint16
	serial       uint32
	name         string
	serialText   string
	// dataBlocks are CTA-861 data blocks, header byte included.
	dataBlocks [][]byte
	// extensionTag overrides the extension's tag; 0 means CTA-861.
	extensionTag byte
	noExtension  bool
}

func (e testEDID) bytes() []byte {
	base := make([]byte, blockSize)
	copy(base, header[:])
	if len(e.manufacturer) == 3 {
		packed := uint16(e.manufacturer[0]-'A'+1)<<10 | uint16(e.manufacturer[1]-'A'+1)<<5 | uint16(e.manufacturer[2]-'A'+1)
		base[8], base[9] = byte(packed>>8), byte(packed)
	}
	binary.LittleEndian.PutUint16(base[10:], e.product)
	binary.LittleEndian.PutUint32(base[12:], e.serial)
	// The first descriptor slot is a detailed timing: a nonzero pixel clock.
	base[54], base[55] = 0x01, 0x1D
	descriptor := func(offset int, kind byte, text string) {
		base[offset+3] = kind
		field := base[offset+5 : offset+18]
		for i := range field {
			field[i] = ' '
		}
		copy(field, text)
		if len(text) < len(field) {
			field[len(text)] = '\n'
		}
	}
	if e.name != "" {
		descriptor(72, descriptorName, e.name)
	}
	if e.serialText != "" {
		descriptor(90, descriptorSerial, e.serialText)
	}
	if e.noExtension {
		return withChecksum(base)
	}
	base[126] = 1

	extension := make([]byte, blockSize)
	extension[0] = extensionCTA
	if e.extensionTag != 0 {
		extension[0] = e.extensionTag
	}
	extension[1] = 3
	offset := 4
	for _, block := range e.dataBlocks {
		offset += copy(extension[offset:], block)
	}
	extension[2] = byte(offset)
	return append(withChecksum(base), withChecksum(extension)...)
}

func withChecksum(block []byte) []byte {
	var sum byte
	for _, b := range block[:blockSize-1] {
		sum += b
	}
	block[blockSize-1] = -sum
	return block
}

func extendedBlock(extendedTag byte, payload ...byte) []byte {
	return append([]byte{dataBlockExtended<<5 | byte(len(payload)+1), extendedTag}, payload...)
}

func TestParseReadsIdentityAndHDRMetadata(t *testing.T) {
	data := testEDID{
		manufacturer: "ACM", product: 0x1234, serial: 42, name: "Test HDR 27", serialText: "SN0001",
		dataBlocks: [][]byte{
			// PQ and HLG; max code 139, frame-average code 115, min code 20.
			extendedBlock(extendedHDRStatic, 0x0D, 0x01, 139, 115, 20),
			// BT.2020 RGB and YCC, plus DCI-P3.
			extendedBlock(extendedColorimetry, 0xC0, 0x80),
		},
	}.bytes()

	info, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Manufacturer != "ACM" || info.ProductCode != 0x1234 || info.SerialNumber != 42 || info.Name != "Test HDR 27" || info.SerialText != "SN0001" {
		t.Fatalf("identity = %+v", info)
	}
	color := info.Color
	if color.HDR == nil || !*color.HDR || color.WideColor == nil || !*color.WideColor {
		t.Fatalf("capabilities = %+v", color)
	}
	// 50 * 2^(139/32) = 1015.3; 50 * 2^(115/32) = 603.7.
	if color.MaxLuminance == nil || *color.MaxLuminance != 1015 {
		t.Fatalf("max luminance = %v, want 1015", color.MaxLuminance)
	}
	if color.MaxAvgLuminance == nil || *color.MaxAvgLuminance != 604 {
		t.Fatalf("frame-average luminance = %v, want 604", color.MaxAvgLuminance)
	}
	// 1015.3 * (20/255)^2 / 100 = 0.0625.
	if color.MinLuminance == nil || math.Abs(*color.MinLuminance-0.0625) > 0.0002 {
		t.Fatalf("min luminance = %v, want about 0.0625", color.MinLuminance)
	}
}

func TestParseLeavesUnsetWhatTheEDIDDoesNotSay(t *testing.T) {
	// An HDR block with only the mandatory two bytes: PQ, no luminance.
	short := testEDID{manufacturer: "ACM", dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x04, 0x01)}}.bytes()
	info, err := Parse(short)
	if err != nil {
		t.Fatal(err)
	}
	if info.Color.HDR == nil || !*info.Color.HDR {
		t.Fatalf("HDR = %v, want advertised", info.Color.HDR)
	}
	if info.Color.MaxLuminance != nil || info.Color.MaxAvgLuminance != nil || info.Color.MinLuminance != nil || info.Color.WideColor != nil {
		t.Fatalf("absent data was filled in: %+v", info.Color)
	}

	// Zero codes mean "no data", and a minimum without a maximum cannot be decoded.
	zeros := testEDID{manufacturer: "ACM", dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x04, 0x01, 0, 0, 20)}}.bytes()
	info, _ = Parse(zeros)
	if info.Color.MaxLuminance != nil || info.Color.MaxAvgLuminance != nil || info.Color.MinLuminance != nil {
		t.Fatalf("zero codes were decoded: %+v", info.Color)
	}

	// An SDR display: an HDR block that lists only gamma, and sRGB-era colorimetry.
	sdr := testEDID{manufacturer: "ACM", dataBlocks: [][]byte{
		extendedBlock(extendedHDRStatic, 0x01, 0x01),
		extendedBlock(extendedColorimetry, 0x03, 0x00),
	}}.bytes()
	info, _ = Parse(sdr)
	if info.Color.HDR == nil || *info.Color.HDR || info.Color.WideColor == nil || *info.Color.WideColor {
		t.Fatalf("an SDR display was reported as HDR or wide color: %+v", info.Color)
	}

	// No extension, or one this parser does not know, says nothing at all.
	for name, data := range map[string][]byte{
		"no extension":        testEDID{manufacturer: "ACM", noExtension: true}.bytes(),
		"DisplayID extension": testEDID{manufacturer: "ACM", extensionTag: 0x70, dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x04, 0x01, 139)}}.bytes(),
	} {
		info, err := Parse(data)
		if err != nil || !info.Color.Empty() {
			t.Fatalf("%s: color = %+v, %v; want nothing", name, info.Color, err)
		}
	}
}

func TestParseRejectsCorruptData(t *testing.T) {
	good := testEDID{manufacturer: "ACM", dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x04, 0x01, 139)}}.bytes()

	badBase := append([]byte(nil), good...)
	badBase[20] ^= 0xFF
	badExtension := append([]byte(nil), good...)
	badExtension[blockSize+10] ^= 0xFF
	notEDID := append([]byte(nil), good...)
	notEDID[0] = 0x42

	for name, data := range map[string][]byte{
		"empty":                  nil,
		"short":                  good[:100],
		"wrong header":           notEDID,
		"bad base checksum":      badBase,
		"bad extension checksum": badExtension,
	} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}

	// A truncated extension is ignored, not an error: the base block is intact.
	info, err := Parse(good[:blockSize+40])
	if err != nil || !info.Color.Empty() {
		t.Fatalf("truncated extension: %+v, %v", info.Color, err)
	}
}

func TestParseIgnoresADataBlockThatRunsPastTheCollection(t *testing.T) {
	data := testEDID{manufacturer: "ACM", dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x04, 0x01, 139)}}.bytes()
	// Claim the block is longer than the collection allows.
	data[blockSize+4] = dataBlockExtended<<5 | 30
	withChecksum(data[blockSize:])

	info, err := Parse(data)
	if err != nil || !info.Color.Empty() {
		t.Fatalf("an overrunning block was read: %+v, %v", info.Color, err)
	}
}

func TestMatchesMonitorRefusesOnlyAPositiveMismatch(t *testing.T) {
	named := Info{Name: "Test HDR 27", ProductCode: 0x1234, SerialText: "SN0001", SerialNumber: 42}
	for _, tc := range []struct {
		name          string
		info          Info
		model, serial string
		want          bool
	}{
		{"same model and serial", named, "Test HDR 27", "SN0001", true},
		{"spacing and case differ", named, " test  hdr 27 ", "sn0001", true},
		{"numeric serial", named, "Test HDR 27", "42", true},
		{"another model", named, "Other 32", "SN0001", false},
		{"another serial", named, "Test HDR 27", "SN9999", false},
		{"compositor reports no serial", named, "Test HDR 27", "", true},
		{"panel named by product code", Info{ProductCode: 0x0A1B}, "0x0A1B", "", true},
		{"a product code alone cannot refute", Info{ProductCode: 0x0A1B}, "0x0FFF", "", true},
		{"name differs but the product code confirms", Info{Name: "Generic", ProductCode: 0x0A1B}, "0x0A1B", "", true},
		{"nothing to compare", Info{}, "Some Panel", "123", true},
	} {
		if got := tc.info.MatchesMonitor(tc.model, tc.serial); got != tc.want {
			t.Errorf("%s: MatchesMonitor = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestForMonitorPicksTheEDIDThatBelongsToTheDisplay(t *testing.T) {
	other := testEDID{manufacturer: "ACM", name: "Other 32", dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x01, 0x01)}}.bytes()
	wanted := testEDID{manufacturer: "ACM", name: "Test HDR 27", dataBlocks: [][]byte{extendedBlock(extendedHDRStatic, 0x04, 0x01, 139)}}.bytes()

	// Two cards expose the same connector name; only one is this display.
	info, err := ForMonitor([][]byte{other, wanted}, "Test HDR 27", "")
	if err != nil || info.Name != "Test HDR 27" || info.Color.HDR == nil || !*info.Color.HDR {
		t.Fatalf("ForMonitor = %+v, %v", info, err)
	}

	if _, err := ForMonitor([][]byte{other}, "Test HDR 27", ""); !errors.Is(err, ErrOtherDisplay) {
		t.Fatalf("an EDID for another display: %v, want ErrOtherDisplay", err)
	}
	if _, err := ForMonitor(nil, "Test HDR 27", ""); !errors.Is(err, ErrNoEDID) {
		t.Fatalf("no EDID: %v, want ErrNoEDID", err)
	}
	if _, err := ForMonitor([][]byte{{1, 2, 3}}, "Test HDR 27", ""); err == nil {
		t.Fatal("a corrupt EDID was accepted")
	}
}
