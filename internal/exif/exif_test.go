package exif

import (
	"encoding/binary"
	"math"
	"testing"
)

// --- synthetic TIFF/JPEG builder (test-only) ----------------------------

// tiffEntry is one IFD entry. If ptr >= 0, the entry is a LONG pointer to
// ifds[ptr] and value is ignored.
type tiffEntry struct {
	tag   uint16
	typ   uint16
	value []byte
	ptr   int
}

type tiffIFD struct{ entries []tiffEntry }

func buildTIFF(order binary.ByteOrder, ifds []tiffIFD) []byte {
	// Pass 1: compute each IFD's offset.
	offsets := make([]uint32, len(ifds))
	off := uint32(8)
	for i, ifd := range ifds {
		offsets[i] = off
		blobs := uint32(0)
		for _, e := range ifd.entries {
			if e.ptr >= 0 {
				continue // pointer values are inline LONGs
			}
			if len(e.value) > 4 {
				blobs += uint32(len(e.value))
			}
		}
		off += 2 + uint32(12*len(ifd.entries)) + 4 + blobs
	}
	// Pass 2: write.
	buf := &byteWriter{order: order}
	buf.u16(0) // placeholder for byte-order mark; fixed below
	buf.u16(42)
	buf.u32(offsets[0])
	for i, ifd := range ifds {
		buf.u16(uint16(len(ifd.entries)))
		blobBase := offsets[i] + 2 + uint32(12*len(ifd.entries)) + 4
		blobOff := blobBase
		for _, e := range ifd.entries {
			buf.u16(e.tag)
			buf.u16(e.typ)
			if e.ptr >= 0 {
				buf.u32(1)
				buf.u32(offsets[e.ptr])
				continue
			}
			buf.u32(uint32(len(e.value)) / typeSize(e.typ))
			if len(e.value) <= 4 {
				padded := make([]byte, 4)
				copy(padded, e.value)
				buf.raw(padded)
			} else {
				buf.u32(blobOff)
				blobOff += uint32(len(e.value))
			}
		}
		buf.u32(0) // no next IFD
		for _, e := range ifd.entries {
			if e.ptr >= 0 || len(e.value) <= 4 {
				continue
			}
			buf.raw(e.value)
		}
	}
	out := buf.b
	if order == binary.LittleEndian {
		out[0], out[1] = 'I', 'I'
	} else {
		out[0], out[1] = 'M', 'M'
	}
	return out
}

func typeSize(t uint16) uint32 {
	switch t {
	case typeByte, typeASCII, typeUndefined:
		return 1
	case typeShort:
		return 2
	case typeLong, typeSLong:
		return 4
	case typeRational, typeSRational:
		return 8
	}
	return 1
}

type byteWriter struct {
	b     []byte
	order binary.ByteOrder
}

func (w *byteWriter) raw(b []byte) { w.b = append(w.b, b...) }
func (w *byteWriter) u16(v uint16) {
	var b [2]byte
	w.order.PutUint16(b[:], v)
	w.b = append(w.b, b[:]...)
}
func (w *byteWriter) u32(v uint32) {
	var b [4]byte
	w.order.PutUint32(b[:], v)
	w.b = append(w.b, b[:]...)
}

// value encoders (in the given byte order)
func encShort(order binary.ByteOrder, vs ...uint16) []byte {
	b := make([]byte, 2*len(vs))
	for i, v := range vs {
		order.PutUint16(b[i*2:], v)
	}
	return b
}
func encLong(order binary.ByteOrder, vs ...uint32) []byte {
	b := make([]byte, 4*len(vs))
	for i, v := range vs {
		order.PutUint32(b[i*4:], v)
	}
	return b
}
func encRational(order binary.ByteOrder, pairs ...[2]uint32) []byte {
	b := make([]byte, 8*len(pairs))
	for i, p := range pairs {
		order.PutUint32(b[i*8:], p[0])
		order.PutUint32(b[i*8+4:], p[1])
	}
	return b
}
func encASCII(s string) []byte { return append([]byte(s), 0) }

// buildTestJPEG returns a minimal JPEG carrying the given TIFF payload
// in an APP1 segment, followed by a dummy APP0 segment and SOS.
func buildTestJPEG(tiff []byte) []byte {
	j := []byte{0xFF, 0xD8} // SOI
	seg := append([]byte("Exif\x00\x00"), tiff...)
	j = append(j, 0xFF, 0xE1, byte((len(seg)+2)>>8), byte(len(seg)+2))
	j = append(j, seg...)
	j = append(j, 0xFF, 0xE0, 0x00, 0x10) // APP0 header
	j = append(j, []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")...)
	j = append(j, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00) // SOS
	j = append(j, 0x11, 0x22, 0x33)                                           // fake scan data
	j = append(j, 0xFF, 0xD9)                                                 // EOI
	return j
}

// sampleTIFF builds a realistic camera payload: IFD0 (make/model/
// orientation/pointers), Exif IFD (exposure, f-number, ISO, date) and a
// GPS IFD (37°46'30"N, 122°25'10"W).
func sampleTIFF(order binary.ByteOrder) []byte {
	ifd0 := tiffIFD{entries: []tiffEntry{
		{TagMake, typeASCII, encASCII("Canon"), -1},
		{TagModel, typeASCII, encASCII("EOS 5D Mark IV"), -1},
		{TagOrientation, typeShort, encShort(order, 6), -1},
		{TagDateTime, typeASCII, encASCII("2024:01:02 03:04:05"), -1},
		{TagExifIFDPointer, typeLong, nil, 1},
		{TagGPSIFDPointer, typeLong, nil, 2},
		{0x9999, 99, []byte{1, 2, 3}, -1}, // unknown type: must be skipped
	}}
	exifIFD := tiffIFD{entries: []tiffEntry{
		{TagExposureTime, typeRational, encRational(order, [2]uint32{1, 125}), -1},
		{TagFNumber, typeRational, encRational(order, [2]uint32{28, 10}), -1},
		{TagISOSpeedRatings, typeShort, encShort(order, 400), -1},
		{TagDateTimeOriginal, typeASCII, encASCII("2024:01:02 03:04:05"), -1},
		{TagFocalLength, typeRational, encRational(order, [2]uint32{50, 1}), -1},
	}}
	gpsIFD := tiffIFD{entries: []tiffEntry{
		{TagGPSLatitudeRef, typeASCII, encASCII("N"), -1},
		{TagGPSLatitude, typeRational, encRational(order,
			[2]uint32{37, 1}, [2]uint32{46, 1}, [2]uint32{3000, 100}), -1},
		{TagGPSLongitudeRef, typeASCII, encASCII("W"), -1},
		{TagGPSLongitude, typeRational, encRational(order,
			[2]uint32{122, 1}, [2]uint32{25, 1}, [2]uint32{1000, 100}), -1},
	}}
	return buildTIFF(order, []tiffIFD{ifd0, exifIFD, gpsIFD})
}

// --- tests ---------------------------------------------------------------

func TestParseJPEG_LittleEndian(t *testing.T) {
	m, err := ParseJPEG(buildTestJPEG(sampleTIFF(binary.LittleEndian)))
	if err != nil {
		t.Fatalf("ParseJPEG: %v", err)
	}
	if got := m.IFD0[TagMake].AsString(); got != "Canon" {
		t.Errorf("Make = %q, want Canon", got)
	}
	if got := m.IFD0[TagModel].AsString(); got != "EOS 5D Mark IV" {
		t.Errorf("Model = %q", got)
	}
	if got := m.Orientation(); got != 6 {
		t.Errorf("Orientation = %d, want 6", got)
	}
	if got := m.Exif[TagExposureTime].Display(); got != "1/125" {
		t.Errorf("ExposureTime = %q, want 1/125", got)
	}
	if got := m.Exif[TagFNumber].Display(); got != "2.8" {
		t.Errorf("FNumber = %q, want 2.8", got)
	}
	if iso, ok := m.Exif[TagISOSpeedRatings].Uint(); !ok || iso != 400 {
		t.Errorf("ISO = %d, ok=%v, want 400", iso, ok)
	}
	if _, ok := m.IFD0[0x9999]; ok {
		t.Error("unknown-type tag should have been skipped")
	}
}

func TestParseJPEG_BigEndian(t *testing.T) {
	m, err := ParseJPEG(buildTestJPEG(sampleTIFF(binary.BigEndian)))
	if err != nil {
		t.Fatalf("ParseJPEG (MM): %v", err)
	}
	if got := m.IFD0[TagMake].AsString(); got != "Canon" {
		t.Errorf("Make = %q, want Canon", got)
	}
	lat, lon, ok := m.GPSCoordinates()
	if !ok {
		t.Fatal("GPSCoordinates returned not-ok")
	}
	if math.Abs(lat-37.775) > 1e-6 {
		t.Errorf("lat = %f, want 37.775", lat)
	}
	if math.Abs(lon-(-122.419444)) > 1e-6 {
		t.Errorf("lon = %f, want -122.419444", lon)
	}
}

func TestGPSCoordinates_SouthEast(t *testing.T) {
	// Rebuild the payload with S/E refs and check the signs flip.
	order := binary.LittleEndian
	ifd0 := tiffIFD{entries: []tiffEntry{
		{TagGPSIFDPointer, typeLong, nil, 1},
	}}
	gpsIFD := tiffIFD{entries: []tiffEntry{
		{TagGPSLatitudeRef, typeASCII, encASCII("S"), -1},
		{TagGPSLatitude, typeRational, encRational(order,
			[2]uint32{10, 1}, [2]uint32{0, 1}, [2]uint32{0, 1}), -1},
		{TagGPSLongitudeRef, typeASCII, encASCII("E"), -1},
		{TagGPSLongitude, typeRational, encRational(order,
			[2]uint32{20, 1}, [2]uint32{30, 1}, [2]uint32{0, 1}), -1},
	}}
	m, err := ParseTIFF(buildTIFF(order, []tiffIFD{ifd0, gpsIFD}))
	if err != nil {
		t.Fatalf("ParseTIFF: %v", err)
	}
	lat, lon, ok := m.GPSCoordinates()
	if !ok {
		t.Fatal("GPSCoordinates returned not-ok")
	}
	if lat != -10 || lon != 20.5 {
		t.Errorf("got (%v, %v), want (-10, 20.5)", lat, lon)
	}
}

func TestParse_NoEXIF(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	jpeg = append(jpeg, []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")...)
	jpeg = append(jpeg, 0xFF, 0xD9)
	if _, err := ParseJPEG(jpeg); err == nil {
		t.Error("expected error for JPEG without EXIF")
	}
}

func TestParse_NotJPEG(t *testing.T) {
	if _, err := ParseJPEG([]byte("this is not a jpeg")); err == nil {
		t.Error("expected error for non-JPEG input")
	}
}

func TestParseTIFF_BadMagic(t *testing.T) {
	bad := []byte{'I', 'I', 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00}
	bad[2] = 0x2B // magic 43, not 42
	if _, err := ParseTIFF(bad); err == nil {
		t.Error("expected error for bad TIFF magic")
	}
}

func TestParseTIFF_Truncated(t *testing.T) {
	if _, err := ParseTIFF([]byte{'I', 'I', 0x2A, 0x00}); err == nil {
		t.Error("expected error for truncated TIFF header")
	}
}

func TestOrientation_Default(t *testing.T) {
	m := &Metadata{IFD0: map[uint16]Value{}}
	if got := m.Orientation(); got != 1 {
		t.Errorf("Orientation = %d, want 1 when tag is absent", got)
	}
}

func TestRationalDisplay(t *testing.T) {
	cases := []struct {
		r    Rational
		want string
	}{
		{Rational{1, 125}, "1/125"},
		{Rational{28, 10}, "2.8"},
		{Rational{50, 1}, "50"},
		{Rational{0, 1}, "0"},
	}
	for _, c := range cases {
		if got := c.r.String(); got != c.want {
			t.Errorf("Rational%v.String() = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestStripEXIF(t *testing.T) {
	orig := buildTestJPEG(sampleTIFF(binary.LittleEndian))
	stripped, removed, err := StripEXIF(orig)
	if err != nil {
		t.Fatalf("StripEXIF: %v", err)
	}
	if !removed {
		t.Error("StripEXIF should report the segment as removed")
	}
	if len(stripped) >= len(orig) {
		t.Error("stripped JPEG should be smaller than the original")
	}
	if _, err := ParseJPEG(stripped); err == nil {
		t.Error("stripped JPEG should no longer parse as having EXIF")
	}
	// The APP0 JFIF segment must survive the strip: SOI(2) + marker(2)
	// + length(2), so "JFIF" starts at offset 6.
	if len(stripped) < 10 || string(stripped[6:10]) != "JFIF" {
		t.Error("strip removed more than the EXIF segment")
	}
}

func TestStripEXIF_NoEXIF(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	jpeg = append(jpeg, []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")...)
	jpeg = append(jpeg, 0xFF, 0xD9)
	out, removed, err := StripEXIF(jpeg)
	if err != nil {
		t.Fatalf("StripEXIF: %v", err)
	}
	if removed {
		t.Error("StripEXIF should report nothing removed")
	}
	if len(out) != len(jpeg) {
		t.Error("JPEG without EXIF should be copied unchanged")
	}
}

func TestTagName(t *testing.T) {
	if got := TagName(TagMake); got != "Make" {
		t.Errorf("TagName(Make) = %q", got)
	}
	if got := TagName(0x1234); got != "Tag 0x1234" {
		t.Errorf("TagName(unknown) = %q", got)
	}
}
