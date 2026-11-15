# go-exif-tool

A zero-dependency Go CLI that reads, strips and thumbnails JPEG images
using their EXIF metadata. The EXIF parser is hand-rolled — no image
library, no CGo — so the whole tool is one static binary.

## What it does

- **`inspect`** — print every EXIF tag (camera, settings, GPS) as a
  readable table, or as JSON with `--json`. Exposure times render as
  `1/125`, f-numbers as `2.8`, GPS as decimal degrees.
- **`gps`** — print just the coordinates (`37.775000,-122.419444`), for
  scripts and pipelines.
- **`strip`** — copy a JPEG with the EXIF APP1 segment removed (privacy
  before sharing), leaving every other segment byte-identical.
- **`thumb`** — make an orientation-aware thumbnail: the EXIF
  orientation tag is applied *before* scaling, so a photo shot sideways
  comes out portrait, not squashed landscape.

## Usage

```sh
# Build
go build -o exiftool ./cmd/exiftool

# Read the metadata of a photo
./exiftool inspect photo.jpg
./exiftool inspect photo.jpg --json

# Just the location
./exiftool gps photo.jpg
# 37.775000,-122.419444

# Remove all EXIF before posting
./exiftool strip photo.jpg clean.jpg

# 320px thumbnail (default), correctly oriented
./exiftool thumb photo.jpg thumb.jpg
./exiftool thumb photo.jpg thumb.jpg --width 640 --height 480
```

Example `inspect` output:

```
[Camera / file]
  Make                     Canon
  Model                    EOS 5D Mark IV
  Orientation              6
  Date/Time                2024:01:02 03:04:05

[Camera settings]
  Exposure Time            1/125
  F-Number                 2.8
  ISO Speed                400
  Date/Time Original       2024:01:02 03:04:05
  Focal Length             50

[GPS]
  GPS Latitude Ref         N
  GPS Latitude             37, 46, 30
  GPS Longitude Ref        W
  GPS Longitude            122, 25, 10

Orientation: 6 (Rotate 90° CW)
GPS: 37.775000, -122.419444
```

## Design decisions

1. **Hand-rolled TIFF parser, stdlib only.** EXIF is a small, stable
   binary format: a TIFF header, then IFD tables of 12-byte entries.
   Parsing it directly keeps the binary dependency-free and makes the
   byte-order handling explicit — the parser reads both `II`
   (little-endian) and `MM` (big-endian) payloads, which is the part
   most wrappers get wrong.
2. **JPEG segment walker, not a full decoder.** Metadata lives in the
   APP1 segment before the scan data, so `inspect` and `strip` never
   decode pixels. The walker stops at the SOS marker and copies
   everything else verbatim, which is what makes `strip` lossless for
   the image itself.
3. **Orientation applied before resize.** A sideways photo is 400×200
   on disk but 200×400 when displayed. Scaling first and rotating
   later would produce a thumbnail with the wrong aspect ratio; the
   transform runs first, then the image is fitted into the requested
   box.
4. **Bilinear interpolation for thumbnails.** The stdlib's
   `image/draw` only offers nearest-neighbour scaling, which looks
   blocky on photos. A small bilinear sampler is ~40 lines and
   produces visibly smoother thumbnails.

## Project layout

```
cmd/exiftool/        CLI: inspect, gps, strip, thumb
internal/exif/       JPEG segment walker, TIFF/EXIF parser, GPS decoding, strip
internal/thumb/      bilinear resize, all 8 EXIF orientation transforms
```

## Tests

```sh
go test ./...
```

The EXIF tests build synthetic JPEGs from scratch — a tiny TIFF
builder assembles IFD0 / Exif / GPS IFDs in both byte orders — so the
parser is exercised without checking any real photo into the repo.
Thumbnail tests use coordinate-encoded gradient images, so a wrong
rotation or flip fails the assertion instead of just looking odd.

## Tech stack

- Go 1.23, standard library only
- `go test` (no test framework)
