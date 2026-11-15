package exif

// tagNames maps well-known EXIF tags to human-readable labels. GPS tags
// live in the same number space as the others (0x0001-0x0006) and never
// collide with IFD0/Exif tags, so one table covers all three IFDs.
var tagNames = map[uint16]string{
	// IFD0
	0x0100: "Image Width",
	0x0101: "Image Height",
	0x010E: "Image Description",
	0x010F: "Make",
	0x0110: "Model",
	0x0112: "Orientation",
	0x011A: "X Resolution",
	0x011B: "Y Resolution",
	0x0128: "Resolution Unit",
	0x0131: "Software",
	0x0132: "Date/Time",
	0x013B: "Artist",
	0x0213: "YCbCr Positioning",
	0x8769: "Exif IFD Pointer",
	0x8825: "GPS IFD Pointer",
	// Exif IFD
	0x829A: "Exposure Time",
	0x829D: "F-Number",
	0x8822: "Exposure Program",
	0x8827: "ISO Speed",
	0x9000: "Exif Version",
	0x9003: "Date/Time Original",
	0x9004: "Date/Time Digitized",
	0x9101: "Components Configuration",
	0x9102: "Compressed Bits Per Pixel",
	0x9201: "Shutter Speed Value",
	0x9202: "Aperture Value",
	0x9203: "Brightness Value",
	0x9204: "Exposure Bias",
	0x9205: "Max Aperture Value",
	0x9206: "Subject Distance",
	0x9207: "Metering Mode",
	0x9208: "Light Source",
	0x9209: "Flash",
	0x920A: "Focal Length",
	0x927C: "Maker Note",
	0x9286: "User Comment",
	0xA000: "Flashpix Version",
	0xA001: "Color Space",
	0xA002: "Pixel X Dimension",
	0xA003: "Pixel Y Dimension",
	0xA217: "Sensing Method",
	0xA401: "Custom Rendered",
	0xA402: "Exposure Mode",
	0xA403: "White Balance",
	0xA404: "Digital Zoom Ratio",
	0xA405: "Focal Length (35mm)",
	0xA406: "Scene Capture Type",
	0xA40C: "Contrast",
	0xA40D: "Saturation",
	0xA40E: "Sharpness",
	0xA420: "Image Unique ID",
	// GPS IFD
	0x0000: "GPS Version",
	0x0001: "GPS Latitude Ref",
	0x0002: "GPS Latitude",
	0x0003: "GPS Longitude Ref",
	0x0004: "GPS Longitude",
	0x0005: "GPS Altitude Ref",
	0x0006: "GPS Altitude",
	0x0007: "GPS Time Stamp",
	0x000C: "GPS Date Stamp",
}

// TagName returns the human-readable label for a tag, or a hex fallback
// for tags the table does not know.
func TagName(tag uint16) string {
	if name, ok := tagNames[tag]; ok {
		return name
	}
	return "Tag 0x" + hex4(tag)
}

func hex4(v uint16) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{
		digits[v>>12&0xF],
		digits[v>>8&0xF],
		digits[v>>4&0xF],
		digits[v&0xF],
	})
}

// orientationDescriptions explains what each EXIF orientation value
// means for display.
var orientationDescriptions = map[int]string{
	1: "Normal",
	2: "Flip horizontal",
	3: "Rotate 180°",
	4: "Flip vertical",
	5: "Transpose (flip horizontal, rotate 90° CCW)",
	6: "Rotate 90° CW",
	7: "Transverse (flip horizontal, rotate 90° CW)",
	8: "Rotate 90° CCW",
}

// OrientationDescription returns a plain-English description of an
// EXIF orientation value.
func OrientationDescription(o int) string {
	if d, ok := orientationDescriptions[o]; ok {
		return d
	}
	return "Unknown"
}
