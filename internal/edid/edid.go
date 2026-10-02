// Package edid reads the parts of a display's EDID that describe its HDR and
// color capabilities. It parses bytes only; reading them from the system is
// the caller's job.
package edid

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	blockSize = 128

	extensionCTA = 0x02

	dataBlockExtended   = 7
	extendedColorimetry = 0x05
	extendedHDRStatic   = 0x06

	descriptorName   = 0xFC
	descriptorSerial = 0xFF
)

var header = [8]byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}

// Info is what an EDID says about a display's identity and color.
type Info struct {
	// Manufacturer is the three-letter PNP ID, such as "GSM".
	Manufacturer string
	ProductCode  uint16
	SerialNumber uint32
	// Name and SerialText come from the display descriptors and are empty
	// when the EDID has none, as on many laptop panels.
	Name       string
	SerialText string
	Color      Color
}

// Color holds the capabilities found in the CTA-861 extension. A nil field
// means the EDID does not say, which is different from saying no.
type Color struct {
	// HDR reports whether the display advertises an HDR transfer function
	// (PQ or HLG). Nil when there is no HDR static metadata block.
	HDR *bool `json:"hdr,omitempty"`
	// WideColor reports BT.2020 or DCI-P3 support. Nil when there is no
	// colorimetry block.
	WideColor *bool `json:"wide_color,omitempty"`
	// Luminance values are in cd/m². Each is nil when its byte is absent or
	// zero, which the standard defines as "no data".
	MaxLuminance    *int     `json:"max_luminance,omitempty"`
	MaxAvgLuminance *int     `json:"max_avg_luminance,omitempty"`
	MinLuminance    *float64 `json:"min_luminance,omitempty"`
}

// Empty reports whether the EDID said nothing about HDR or color.
func (c Color) Empty() bool {
	return c.HDR == nil && c.WideColor == nil && c.MaxLuminance == nil && c.MaxAvgLuminance == nil && c.MinLuminance == nil
}

// Parse reads a base EDID block and any CTA-861 extensions. A block whose
// checksum is wrong is rejected: numbers from a corrupt EDID must not end up
// in a profile. Extension types it does not know, such as DisplayID, are
// skipped.
func Parse(data []byte) (Info, error) {
	if len(data) < blockSize {
		return Info{}, fmt.Errorf("EDID is %d bytes, shorter than one block", len(data))
	}
	base := data[:blockSize]
	if [8]byte(base[:8]) != header {
		return Info{}, errors.New("not an EDID: wrong header")
	}
	if !checksumOK(base) {
		return Info{}, errors.New("EDID base block has a wrong checksum")
	}

	info := Info{
		Manufacturer: manufacturerID(base[8], base[9]),
		ProductCode:  binary.LittleEndian.Uint16(base[10:12]),
		SerialNumber: binary.LittleEndian.Uint32(base[12:16]),
	}
	for offset := 54; offset+18 <= 126; offset += 18 {
		descriptor := base[offset : offset+18]
		if descriptor[0] != 0 || descriptor[1] != 0 {
			continue // a detailed timing, not a display descriptor
		}
		switch descriptor[3] {
		case descriptorName:
			info.Name = descriptorText(descriptor[5:])
		case descriptorSerial:
			info.SerialText = descriptorText(descriptor[5:])
		}
	}

	extensions := int(base[126])
	for index := 1; index <= extensions; index++ {
		start := index * blockSize
		if start+blockSize > len(data) {
			break // truncated; keep what the complete blocks said
		}
		block := data[start : start+blockSize]
		if block[0] != extensionCTA {
			continue
		}
		if !checksumOK(block) {
			return Info{}, fmt.Errorf("EDID extension block %d has a wrong checksum", index)
		}
		parseCTA(block, &info.Color)
	}
	return info, nil
}

func checksumOK(block []byte) bool {
	var sum byte
	for _, b := range block {
		sum += b
	}
	return sum == 0
}

func manufacturerID(high, low byte) string {
	packed := uint16(high)<<8 | uint16(low)
	letters := []byte{byte(packed >> 10 & 0x1F), byte(packed >> 5 & 0x1F), byte(packed & 0x1F)}
	for i, letter := range letters {
		if letter < 1 || letter > 26 {
			return ""
		}
		letters[i] = 'A' + letter - 1
	}
	return string(letters)
}

func descriptorText(raw []byte) string {
	if end := strings.IndexByte(string(raw), '\n'); end >= 0 {
		raw = raw[:end]
	}
	return strings.TrimSpace(string(raw))
}

// parseCTA walks the data block collection of a CTA-861 extension, which
// runs from byte 4 up to the offset in byte 2.
func parseCTA(block []byte, color *Color) {
	end := int(block[2])
	if end < 4 || end > blockSize-1 {
		return
	}
	for offset := 4; offset < end; {
		tag, length := block[offset]>>5, int(block[offset]&0x1F)
		payload := offset + 1
		if payload+length > end {
			return
		}
		if tag == dataBlockExtended && length >= 1 {
			body := block[payload+1 : payload+length]
			switch block[payload] {
			case extendedColorimetry:
				parseColorimetry(body, color)
			case extendedHDRStatic:
				parseHDRStatic(body, color)
			}
		}
		offset = payload + length
	}
}

func parseColorimetry(body []byte, color *Color) {
	if len(body) < 1 {
		return
	}
	// Byte 0 bits 5 to 7 are the BT.2020 variants; byte 1 bit 7 is DCI-P3.
	wide := body[0]&0xE0 != 0 || (len(body) >= 2 && body[1]&0x80 != 0)
	color.WideColor = &wide
}

func parseHDRStatic(body []byte, color *Color) {
	if len(body) < 1 {
		return
	}
	// Bit 2 is SMPTE ST 2084 (PQ), bit 3 is HLG.
	hdr := body[0]&0x0C != 0
	color.HDR = &hdr

	var maxLuminance float64
	if len(body) >= 3 && body[2] != 0 {
		maxLuminance = luminance(body[2])
		value := int(math.Round(maxLuminance))
		color.MaxLuminance = &value
	}
	if len(body) >= 4 && body[3] != 0 {
		value := int(math.Round(luminance(body[3])))
		color.MaxAvgLuminance = &value
	}
	// The minimum is coded relative to the maximum, so it needs both.
	if len(body) >= 5 && body[4] != 0 && maxLuminance > 0 {
		ratio := float64(body[4]) / 255
		value := math.Round(maxLuminance*ratio*ratio/100*10000) / 10000
		color.MinLuminance = &value
	}
}

// luminance decodes CTA-861's coded value: 50 * 2^(code/32) cd/m².
func luminance(code byte) float64 {
	return 50 * math.Pow(2, float64(code)/32)
}

// MatchesMonitor reports whether this EDID can belong to a display the
// compositor describes with the given model and serial. It refuses only on a
// positive mismatch of the text descriptors, which compositors report as
// written. The numeric product code and serial can confirm a match but never
// refute one, because how a compositor formats them is not something to guess
// at. An EDID with nothing comparable is accepted: many panels carry neither
// descriptor.
func (i Info) MatchesMonitor(model string, serial string) bool {
	return matches(model, i.Name, fmt.Sprintf("0x%04X", i.ProductCode)) &&
		matches(serial, i.SerialText, fmt.Sprintf("%d", i.SerialNumber), fmt.Sprintf("0x%08X", i.SerialNumber))
}

// matches compares what the compositor reports with the EDID's descriptor
// text and the numeric forms of the same field.
func matches(reported string, descriptor string, numeric ...string) bool {
	reported, descriptor = normalize(reported), normalize(descriptor)
	if reported == "" || descriptor == "" || reported == descriptor {
		return true
	}
	for _, form := range numeric {
		if normalize(form) == reported {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// ErrNoEDID reports that the system offered no EDID for a display.
var ErrNoEDID = errors.New("no EDID could be read for this display")

// ErrOtherDisplay reports that every EDID found describes some other display.
// A connector name alone is not identity, so nothing is taken from it.
var ErrOtherDisplay = errors.New("the EDID found belongs to another display")

// ForMonitor picks, from the EDIDs read for a connector, the one that can
// belong to the display the compositor describes.
func ForMonitor(candidates [][]byte, model string, serial string) (Info, error) {
	if len(candidates) == 0 {
		return Info{}, ErrNoEDID
	}
	var firstErr error
	for _, candidate := range candidates {
		info, err := Parse(candidate)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if info.MatchesMonitor(model, serial) {
			return info, nil
		}
		if firstErr == nil {
			firstErr = ErrOtherDisplay
		}
	}
	return Info{}, firstErr
}
