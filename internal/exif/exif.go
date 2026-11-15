// Package exif parses EXIF metadata out of JPEG files with no
// third-party dependencies. It implements just enough of the TIFF/EXIF
// spec to be genuinely useful: both byte orders, the IFD0 / Exif / GPS
// IFD chains, and the tag types cameras actually emit (BYTE, ASCII,
// SHORT, LONG, RATIONAL, UNDEFINED, SLONG, SRATIONAL).
package exif

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// TIFF tag data types.
const (
	typeByte      = 1
	typeASCII     = 2
	typeShort     = 3
	typeLong      = 4
	typeRational  = 5
	typeUndefined = 7
	typeSLong     = 9
	typeSRational = 10
)

var typeSizes = map[uint16]uint32{
	typeByte:      1,
	typeASCII:     1,
	typeShort:     2,
	typeLong:      4,
	typeRational:  8,
	typeUndefined: 1,
	typeSLong:     4,
	typeSRational: 8,
}

// Well-known tags.
const (
	TagImageWidth       = 0x0100
	TagImageLength      = 0x0101
	TagMake             = 0x010F
	TagModel            = 0x0110
	TagOrientation      = 0x0112
	TagXResolution      = 0x011A
	TagYResolution      = 0x011B
	TagDateTime         = 0x0132
	TagExifIFDPointer   = 0x8769
	TagGPSIFDPointer    = 0x8825
	TagExposureTime     = 0x829A
	TagFNumber          = 0x829D
	TagISOSpeedRatings  = 0x8827
	TagDateTimeOriginal = 0x9003
	TagFocalLength      = 0x920A
	TagGPSLatitudeRef   = 0x0001
	TagGPSLatitude      = 0x0002
	TagGPSLongitudeRef  = 0x0003
	TagGPSLongitude     = 0x0004
	TagGPSAltitudeRef   = 0x0005
	TagGPSAltitude      = 0x0006
)

// Rational is one TIFF RATIONAL / SRATIONAL value.
type Rational struct {
	Num int64
	Den int64
}

// Float returns the value as a float64 (0 when the denominator is 0).
func (r Rational) Float() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

// String renders the rational the way photographers expect it:
// "1/125" for an exposure time, "2.8" for an f-number.
func (r Rational) String() string {
	if r.Den == 0 {
		return fmt.Sprintf("%d/0", r.Num)
	}
	if r.Den == 1 {
		return fmt.Sprintf("%d", r.Num)
	}
	if r.Num < r.Den {
		return fmt.Sprintf("%d/%d", r.Num, r.Den)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", r.Float()), "0"), ".")
}

// Value is one decoded IFD entry. Raw holds the payload bytes in file
// byte order; the accessor methods decode them on demand.
type Value struct {
	Type  uint16
	Count uint32
	Raw   []byte
	order binary.ByteOrder
}

// AsString returns the value as text. ASCII tags are trimmed of their
// NUL terminator; anything else is rendered as a comma-joined list.
func (v Value) AsString() string {
	if v.Type == typeASCII {
		return strings.TrimRight(string(v.Raw), "\x00")
	}
	return v.Display()
}

// Display renders the value for humans.
func (v Value) Display() string {
	switch v.Type {
	case typeASCII:
		return strings.TrimRight(string(v.Raw), "\x00")
	case typeShort:
		parts := make([]string, 0, v.Count)
		for i := uint32(0); i < v.Count; i++ {
			parts = append(parts, fmt.Sprintf("%d", v.order.Uint16(v.Raw[i*2:])))
		}
		return strings.Join(parts, ", ")
	case typeLong:
		parts := make([]string, 0, v.Count)
		for i := uint32(0); i < v.Count; i++ {
			parts = append(parts, fmt.Sprintf("%d", v.order.Uint32(v.Raw[i*4:])))
		}
		return strings.Join(parts, ", ")
	case typeSLong:
		parts := make([]string, 0, v.Count)
		for i := uint32(0); i < v.Count; i++ {
			parts = append(parts, fmt.Sprintf("%d", int32(v.order.Uint32(v.Raw[i*4:]))))
		}
		return strings.Join(parts, ", ")
	case typeRational, typeSRational:
		rs := v.Rationals()
		parts := make([]string, 0, len(rs))
		for _, r := range rs {
			parts = append(parts, r.String())
		}
		return strings.Join(parts, ", ")
	case typeByte, typeUndefined:
		if len(v.Raw) == 1 {
			return fmt.Sprintf("%d", v.Raw[0])
		}
		return fmt.Sprintf("%v", []byte(v.Raw))
	default:
		return fmt.Sprintf("<%d bytes, type %d>", len(v.Raw), v.Type)
	}
}

// Uint returns the first numeric component of the value, if it has one.
func (v Value) Uint() (uint64, bool) {
	switch v.Type {
	case typeByte, typeUndefined:
		if len(v.Raw) > 0 {
			return uint64(v.Raw[0]), true
		}
	case typeShort:
		if len(v.Raw) >= 2 {
			return uint64(v.order.Uint16(v.Raw)), true
		}
	case typeLong:
		if len(v.Raw) >= 4 {
			return uint64(v.order.Uint32(v.Raw)), true
		}
	case typeSLong:
		if len(v.Raw) >= 4 {
			return uint64(int32(v.order.Uint32(v.Raw))), true
		}
	}
	return 0, false
}

// Rationals decodes the value as a list of rationals (empty for
// non-rational types).
func (v Value) Rationals() []Rational {
	if v.Type != typeRational && v.Type != typeSRational {
		return nil
	}
	out := make([]Rational, 0, v.Count)
	for i := uint32(0); i < v.Count; i++ {
		b := v.Raw[i*8:]
		if v.Type == typeRational {
			out = append(out, Rational{
				Num: int64(v.order.Uint32(b)),
				Den: int64(v.order.Uint32(b[4:])),
			})
		} else {
			out = append(out, Rational{
				Num: int64(int32(v.order.Uint32(b))),
				Den: int64(int32(v.order.Uint32(b[4:]))),
			})
		}
	}
	return out
}

// Metadata is the parsed EXIF payload of one image. Tags are kept per
// IFD so callers can tell camera settings (Exif IFD) apart from file
// info (IFD0) and location (GPS IFD).
type Metadata struct {
	IFD0      map[uint16]Value
	Exif      map[uint16]Value
	GPS       map[uint16]Value
	ByteOrder binary.ByteOrder
}

// Get looks a tag up in the Exif IFD first, then IFD0.
func (m *Metadata) Get(tag uint16) (Value, bool) {
	if v, ok := m.Exif[tag]; ok {
		return v, true
	}
	v, ok := m.IFD0[tag]
	return v, ok
}

// Orientation returns the EXIF orientation value (1-8), defaulting to 1
// (no rotation) when the tag is absent or invalid.
func (m *Metadata) Orientation() int {
	if v, ok := m.IFD0[TagOrientation]; ok {
		if u, ok := v.Uint(); ok && u >= 1 && u <= 8 {
			return int(u)
		}
	}
	return 1
}

// GPSCoordinates decodes the GPS IFD into decimal degrees. The boolean
// result is false when either coordinate is missing.
func (m *Metadata) GPSCoordinates() (lat, lon float64, ok bool) {
	latVals, ok1 := m.GPS[TagGPSLatitude]
	latRef, ok2 := m.GPS[TagGPSLatitudeRef]
	lonVals, ok3 := m.GPS[TagGPSLongitude]
	lonRef, ok4 := m.GPS[TagGPSLongitudeRef]
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return 0, 0, false
	}
	lats := latVals.Rationals()
	lons := lonVals.Rationals()
	if len(lats) < 3 || len(lons) < 3 {
		return 0, 0, false
	}
	lat = dmsToDecimal(lats[0], lats[1], lats[2])
	lon = dmsToDecimal(lons[0], lons[1], lons[2])
	if strings.ToUpper(latRef.AsString()) == "S" {
		lat = -lat
	}
	if strings.ToUpper(lonRef.AsString()) == "W" {
		lon = -lon
	}
	return lat, lon, true
}

func dmsToDecimal(d, m, s Rational) float64 {
	return d.Float() + m.Float()/60 + s.Float()/3600
}

// Parse reads a JPEG from r and returns its EXIF metadata.
func Parse(r io.Reader) (*Metadata, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseJPEG(data)
}

// ParseJPEG parses EXIF out of raw JPEG bytes.
func ParseJPEG(data []byte) (*Metadata, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errors.New("exif: not a JPEG file (missing SOI marker)")
	}
	pos := 2
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			return nil, errors.New("exif: corrupt JPEG segment stream")
		}
		marker := data[pos+1]
		for marker == 0xFF { // skip padding bytes
			pos++
			if pos+1 >= len(data) {
				return nil, errors.New("exif: truncated JPEG segment stream")
			}
			marker = data[pos+1]
		}
		if marker == 0xDA { // SOS: entropy-coded scan data follows
			break
		}
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			pos += 2 // standalone markers have no length field
			continue
		}
		if pos+4 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if length < 2 || pos+2+length > len(data) {
			return nil, errors.New("exif: truncated JPEG segment")
		}
		segment := data[pos+4 : pos+2+length]
		if marker == 0xE1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			return ParseTIFF(segment[6:])
		}
		pos += 2 + length
	}
	return nil, errors.New("exif: no EXIF APP1 segment found")
}

// ParseTIFF parses a raw TIFF payload (the bytes after the
// "Exif\x00\x00" header in an APP1 segment).
func ParseTIFF(data []byte) (*Metadata, error) {
	if len(data) < 8 {
		return nil, errors.New("exif: TIFF header too short")
	}
	var order binary.ByteOrder
	switch string(data[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, errors.New("exif: bad TIFF byte-order mark")
	}
	if order.Uint16(data[2:4]) != 42 {
		return nil, errors.New("exif: bad TIFF magic (expected 42)")
	}
	m := &Metadata{
		IFD0:      map[uint16]Value{},
		Exif:      map[uint16]Value{},
		GPS:       map[uint16]Value{},
		ByteOrder: order,
	}
	ifd0, _, err := parseIFD(data, order, order.Uint32(data[4:8]))
	if err != nil {
		return nil, err
	}
	m.IFD0 = ifd0
	if v, ok := ifd0[TagExifIFDPointer]; ok {
		if off, ok := v.Uint(); ok {
			if exifIFD, _, err := parseIFD(data, order, uint32(off)); err == nil {
				m.Exif = exifIFD
			}
		}
	}
	if v, ok := ifd0[TagGPSIFDPointer]; ok {
		if off, ok := v.Uint(); ok {
			if gpsIFD, _, err := parseIFD(data, order, uint32(off)); err == nil {
				m.GPS = gpsIFD
			}
		}
	}
	return m, nil
}

// parseIFD decodes one image file directory at the given offset. It
// returns the tag map and the offset of the next IFD (0 when there is
// none). Unknown tag types are skipped rather than failing the parse.
func parseIFD(data []byte, order binary.ByteOrder, offset uint32) (map[uint16]Value, uint32, error) {
	if int(offset)+2 > len(data) {
		return nil, 0, errors.New("exif: IFD offset out of bounds")
	}
	count := int(order.Uint16(data[offset:]))
	entries := make(map[uint16]Value, count)
	pos := int(offset) + 2
	for i := 0; i < count; i++ {
		if pos+12 > len(data) {
			return nil, 0, errors.New("exif: truncated IFD entry")
		}
		tag := order.Uint16(data[pos:])
		typ := order.Uint16(data[pos+2:])
		cnt := order.Uint32(data[pos+4:])
		size, known := typeSizes[typ]
		pos += 12
		if !known {
			continue
		}
		total := size * cnt
		var raw []byte
		if total <= 4 {
			raw = data[pos-4 : pos-4+int(total)]
		} else {
			off := order.Uint32(data[pos-4:])
			if int(off)+int(total) > len(data) {
				return nil, 0, errors.New("exif: tag value out of bounds")
			}
			raw = data[off : off+total]
		}
		entries[tag] = Value{
			Type:  typ,
			Count: cnt,
			Raw:   append([]byte(nil), raw...),
			order: order,
		}
	}
	var next uint32
	if pos+4 <= len(data) {
		next = order.Uint32(data[pos:])
	}
	return entries, next, nil
}

// StripEXIF returns a copy of the JPEG with the EXIF APP1 segment
// removed. The second result reports whether a segment was removed.
func StripEXIF(data []byte) ([]byte, bool, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, false, errors.New("exif: not a JPEG file (missing SOI marker)")
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[0:2]...)
	pos := 2
	removed := false
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			break
		}
		marker := data[pos+1]
		for marker == 0xFF {
			pos++
			if pos+1 >= len(data) {
				break
			}
			marker = data[pos+1]
		}
		if marker == 0xDA { // SOS: copy the rest verbatim
			out = append(out, data[pos:]...)
			return out, removed, nil
		}
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			out = append(out, data[pos:pos+2]...)
			pos += 2
			continue
		}
		if pos+4 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if length < 2 || pos+2+length > len(data) {
			return nil, false, errors.New("exif: truncated JPEG segment")
		}
		segment := data[pos+4 : pos+2+length]
		if marker == 0xE1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			removed = true // drop this segment entirely
		} else {
			out = append(out, data[pos:pos+2+length]...)
		}
		pos += 2 + length
	}
	out = append(out, data[pos:]...)
	return out, removed, nil
}
