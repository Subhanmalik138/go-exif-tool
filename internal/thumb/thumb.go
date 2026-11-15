// Package thumb builds orientation-aware thumbnails: it applies the
// EXIF orientation transform first, then scales the image down with
// bilinear interpolation. No third-party dependencies.
package thumb

import (
	"image"
	"image/color"
)

// Thumbnail scales src so it fits inside maxW x maxH, preserving the
// aspect ratio, after applying the EXIF orientation transform. An
// orientation of 1 (or anything out of range) means "no transform".
// Images already smaller than the box are returned unchanged.
func Thumbnail(src image.Image, maxW, maxH, orientation int) image.Image {
	img := ApplyOrientation(src, orientation)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return img
	}
	scale := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	return Resize(img, int(float64(w)*scale+0.5), int(float64(h)*scale+0.5))
}

// Resize scales src to exactly newW x newH with bilinear interpolation.
func Resize(src image.Image, newW, newH int) image.Image {
	if newW <= 0 || newH <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	for y := 0; y < newH; y++ {
		// Map the centre of the destination pixel into source space.
		sy := (float64(y)+0.5)*sh/float64(newH) - 0.5
		for x := 0; x < newW; x++ {
			sx := (float64(x)+0.5)*sw/float64(newW) - 0.5
			dst.Set(x, y, bilinear(src, sb, sx, sy))
		}
	}
	return dst
}

// bilinear samples src at fractional coordinates (sx, sy) relative to
// the image bounds, interpolating the four nearest pixels.
func bilinear(src image.Image, b image.Rectangle, sx, sy float64) color.Color {
	x0 := int(sx)
	y0 := int(sy)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 > b.Max.X-1 {
		x1 = b.Max.X - 1
	}
	if y1 > b.Max.Y-1 {
		y1 = b.Max.Y - 1
	}
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	if fx < 0 {
		fx = 0
	}
	if fy < 0 {
		fy = 0
	}
	c00 := color.NRGBAModel.Convert(src.At(b.Min.X+x0, b.Min.Y+y0)).(color.NRGBA)
	c10 := color.NRGBAModel.Convert(src.At(b.Min.X+x1, b.Min.Y+y0)).(color.NRGBA)
	c01 := color.NRGBAModel.Convert(src.At(b.Min.X+x0, b.Min.Y+y1)).(color.NRGBA)
	c11 := color.NRGBAModel.Convert(src.At(b.Min.X+x1, b.Min.Y+y1)).(color.NRGBA)
	lerp := func(a, b uint8, t float64) uint8 {
		return uint8(float64(a)*(1-t) + float64(b)*t + 0.5)
	}
	top := color.NRGBA{
		R: lerp(c00.R, c10.R, fx),
		G: lerp(c00.G, c10.G, fx),
		B: lerp(c00.B, c10.B, fx),
		A: lerp(c00.A, c10.A, fx),
	}
	bottom := color.NRGBA{
		R: lerp(c01.R, c11.R, fx),
		G: lerp(c01.G, c11.G, fx),
		B: lerp(c01.B, c11.B, fx),
		A: lerp(c01.A, c11.A, fx),
	}
	return color.NRGBA{
		R: lerp(top.R, bottom.R, fy),
		G: lerp(top.G, bottom.G, fy),
		B: lerp(top.B, bottom.B, fy),
		A: lerp(top.A, bottom.A, fy),
	}
}

// ApplyOrientation returns src transformed according to the EXIF
// orientation value (1-8). Orientation 1 is the identity; anything
// outside 1-8 is treated as 1.
//
//	1 = normal, 2 = flip horizontal, 3 = rotate 180°,
//	4 = flip vertical, 5 = transpose, 6 = rotate 90° CW,
//	7 = transverse, 8 = rotate 90° CCW.
func ApplyOrientation(src image.Image, orientation int) image.Image {
	if orientation < 1 || orientation > 8 || orientation == 1 {
		return src
	}
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	swap := orientation >= 5
	dstW, dstH := w, h
	if swap {
		dstW, dstH = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			sx, sy := orientMap(orientation, x, y, w, h)
			dst.Set(x, y, src.At(sb.Min.X+sx, sb.Min.Y+sy))
		}
	}
	return dst
}

// orientMap maps a destination pixel (x, y) back to the source pixel
// for the given orientation. w and h are the source dimensions.
func orientMap(orientation, x, y, w, h int) (int, int) {
	switch orientation {
	case 2: // flip horizontal
		return w - 1 - x, y
	case 3: // rotate 180
		return w - 1 - x, h - 1 - y
	case 4: // flip vertical
		return x, h - 1 - y
	case 5: // transpose
		return y, x
	case 6: // rotate 90 CW
		return y, h - 1 - x
	case 7: // transverse
		return w - 1 - y, h - 1 - x
	case 8: // rotate 90 CCW
		return w - 1 - y, x
	default:
		return x, y
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
