// Command exiftool inspects, strips and thumbnails JPEG images based
// on their EXIF metadata.
//
//	Usage:
//	  exiftool inspect <photo.jpg> [--json]   print EXIF tags
//	  exiftool gps <photo.jpg>                print GPS coordinates
//	  exiftool strip <in.jpg> <out.jpg>       copy without EXIF data
//	  exiftool thumb <in> <out> [--width N] [--height N]
//	                                          make an orientation-aware thumbnail
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Subhanmalik138/go-exif-tool/internal/exif"
	"github.com/Subhanmalik138/go-exif-tool/internal/thumb"
)

const usage = `exiftool — inspect, strip and thumbnail JPEG images by EXIF metadata

Usage:
  exiftool inspect <photo.jpg> [--json]   print EXIF tags
  exiftool gps <photo.jpg>                print GPS coordinates
  exiftool strip <in.jpg> <out.jpg>       copy the image without EXIF data
  exiftool thumb <in> <out> [--width N] [--height N]
                                          make an orientation-aware thumbnail

Options for thumb:
  --width N    max thumbnail width  (default 320)
  --height N   max thumbnail height (default 320)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "inspect":
		err = cmdInspect(os.Args[2:])
	case "gps":
		err = cmdGPS(os.Args[2:])
	case "strip":
		err = cmdStrip(os.Args[2:])
	case "thumb":
		err = cmdThumb(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "exiftool: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "exiftool: %v\n", err)
		os.Exit(1)
	}
}

// reorderFlags moves flag arguments (anything starting with "-") ahead
// of positional arguments, so flags work after filenames too:
// `exiftool thumb in.jpg out.jpg --width 640` behaves like
// `exiftool thumb --width 640 in.jpg out.jpg`. The stdlib flag package
// stops parsing at the first positional argument, which would otherwise
// silently turn trailing flags into extra filenames. valueFlags names
// the flags that take a separate value (e.g. width, height); those are
// moved together with the following argument, while boolean flags move
// alone. "--flag=value" forms are always moved as one unit.
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = append(rest, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
		}
		flags = append(flags, a)
		if valueFlags[name] && !strings.Contains(a, "=") && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, rest...)
}

func loadMetadata(path string) (*exif.Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return exif.Parse(f)
}

func cmdInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "output tags as JSON")
	if err := fs.Parse(reorderFlags(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect needs exactly one file argument")
	}
	m, err := loadMetadata(fs.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(m)
	}
	printTags("Camera / file", m.IFD0)
	printTags("Camera settings", m.Exif)
	printTags("GPS", m.GPS)
	fmt.Printf("\nOrientation: %d (%s)\n", m.Orientation(), exif.OrientationDescription(m.Orientation()))
	if lat, lon, ok := m.GPSCoordinates(); ok {
		fmt.Printf("GPS: %.6f, %.6f\n", lat, lon)
	}
	return nil
}

func printTags(section string, tags map[uint16]exif.Value) {
	if len(tags) == 0 {
		return
	}
	keys := make([]uint16, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	fmt.Printf("\n[%s]\n", section)
	for _, k := range keys {
		fmt.Printf("  %-24s %s\n", exif.TagName(k), tags[k].Display())
	}
}

func printJSON(m *exif.Metadata) error {
	out := map[string]any{}
	add := func(prefix string, tags map[uint16]exif.Value) {
		for k, v := range tags {
			out[prefix+exif.TagName(k)] = v.Display()
		}
	}
	add("", m.IFD0)
	add("", m.Exif)
	add("GPS ", m.GPS)
	out["Orientation"] = m.Orientation()
	if lat, lon, ok := m.GPSCoordinates(); ok {
		out["GPS Latitude"] = lat
		out["GPS Longitude"] = lon
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func cmdGPS(args []string) error {
	fs := flag.NewFlagSet("gps", flag.ContinueOnError)
	if err := fs.Parse(reorderFlags(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("gps needs exactly one file argument")
	}
	m, err := loadMetadata(fs.Arg(0))
	if err != nil {
		return err
	}
	lat, lon, ok := m.GPSCoordinates()
	if !ok {
		return fmt.Errorf("no GPS coordinates in %s", fs.Arg(0))
	}
	fmt.Printf("%.6f,%.6f\n", lat, lon)
	return nil
}

func cmdStrip(args []string) error {
	fs := flag.NewFlagSet("strip", flag.ContinueOnError)
	if err := fs.Parse(reorderFlags(args, nil)); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("strip needs an input and an output file")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	stripped, removed, err := exif.StripEXIF(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(fs.Arg(1), stripped, 0o644); err != nil {
		return err
	}
	if removed {
		fmt.Printf("stripped EXIF: %s -> %s (%d bytes saved)\n",
			fs.Arg(0), fs.Arg(1), len(data)-len(stripped))
	} else {
		fmt.Printf("no EXIF segment found in %s; copied unchanged\n", fs.Arg(0))
	}
	return nil
}

func cmdThumb(args []string) error {
	fs := flag.NewFlagSet("thumb", flag.ContinueOnError)
	width := fs.Int("width", 320, "max thumbnail width")
	height := fs.Int("height", 320, "max thumbnail height")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"width": true, "height": true})); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("thumb needs an input and an output file")
	}
	if *width <= 0 || *height <= 0 {
		return fmt.Errorf("width and height must be positive")
	}
	in, out := fs.Arg(0), fs.Arg(1)

	orientation := 1
	if m, err := loadMetadata(in); err == nil {
		orientation = m.Orientation()
	}
	// loadMetadata failing just means "no EXIF" — thumbnails still work.

	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decoding %s: %w", in, err)
	}
	tn := thumb.Thumbnail(src, *width, *height, orientation)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// filepath.Dir returns "." for a bare filename; MkdirAll(".") is a no-op.
	w, err := os.Create(out)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := jpeg.Encode(w, tn, &jpeg.Options{Quality: 85}); err != nil {
		return err
	}
	b := tn.Bounds()
	fmt.Printf("thumbnail: %s -> %s (%dx%d, orientation %d applied)\n",
		in, out, b.Dx(), b.Dy(), orientation)
	return nil
}
