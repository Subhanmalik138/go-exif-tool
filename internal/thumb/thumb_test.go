package thumb

import (
	"image"
	"image/color"
	"testing"
)

// gradient returns a w×h image whose pixels encode their coordinates,
// so a wrong rotation or flip is visible in the test assertions.
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 50), G: uint8(y * 50), B: 0, A: 255})
		}
	}
	return img
}

func TestResizeDimensions(t *testing.T) {
	img := gradient(800, 400)
	out := Resize(img, 200, 100)
	if b := out.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("Resize = %dx%d, want 200x100", b.Dx(), b.Dy())
	}
}

func TestResizeKeepsCorners(t *testing.T) {
	img := gradient(4, 4)
	out := Resize(img, 2, 2)
	// Corners of a bilinear downscale should stay close to the source
	// corners (within interpolation tolerance).
	got := color.RGBAModel.Convert(out.At(0, 0)).(color.RGBA)
	if got.R > 40 || got.G > 40 {
		t.Errorf("top-left pixel = %+v, want near black", got)
	}
	got = color.RGBAModel.Convert(out.At(1, 1)).(color.RGBA)
	if got.R < 100 || got.G < 100 {
		t.Errorf("bottom-right pixel = %+v, want bright", got)
	}
}

func TestThumbnailFitsBox(t *testing.T) {
	img := gradient(800, 400)
	tn := Thumbnail(img, 200, 200, 1)
	b := tn.Bounds()
	if b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("Thumbnail = %dx%d, want 200x100", b.Dx(), b.Dy())
	}
}

func TestThumbnailNoUpscale(t *testing.T) {
	img := gradient(50, 40)
	tn := Thumbnail(img, 200, 200, 1)
	b := tn.Bounds()
	if b.Dx() != 50 || b.Dy() != 40 {
		t.Errorf("Thumbnail = %dx%d, want 50x40 (no upscale)", b.Dx(), b.Dy())
	}
}

func TestThumbnailAppliesOrientation(t *testing.T) {
	// A 400x200 photo stored sideways (orientation 6) displays as
	// 200x400, so its thumbnail must be portrait, not landscape.
	img := gradient(400, 200)
	tn := Thumbnail(img, 200, 200, 6)
	b := tn.Bounds()
	if b.Dx() != 100 || b.Dy() != 200 {
		t.Errorf("Thumbnail (orientation 6) = %dx%d, want 100x200", b.Dx(), b.Dy())
	}
}

func TestApplyOrientation_Rotate90CW(t *testing.T) {
	// 2 wide, 1 tall: pixels (R=0) and (R=50). Rotating 90° CW turns the
	// row into a column with the left pixel on top and the right pixel
	// on the bottom.
	img := gradient(2, 1)
	out := ApplyOrientation(img, 6)
	b := out.Bounds()
	if b.Dx() != 1 || b.Dy() != 2 {
		t.Fatalf("orientation 6 bounds = %dx%d, want 1x2", b.Dx(), b.Dy())
	}
	top := color.RGBAModel.Convert(out.At(0, 0)).(color.RGBA)
	bottom := color.RGBAModel.Convert(out.At(0, 1)).(color.RGBA)
	if top.R != 0 || bottom.R != 50 {
		t.Errorf("orientation 6: top R=%d bottom R=%d, want 0/50", top.R, bottom.R)
	}
}

func TestApplyOrientation_FlipHorizontal(t *testing.T) {
	img := gradient(2, 1)
	out := ApplyOrientation(img, 2)
	left := color.RGBAModel.Convert(out.At(0, 0)).(color.RGBA)
	right := color.RGBAModel.Convert(out.At(1, 0)).(color.RGBA)
	if left.R != 50 || right.R != 0 {
		t.Errorf("orientation 2: left R=%d right R=%d, want 50/0", left.R, right.R)
	}
}

func TestApplyOrientation_Rotate180(t *testing.T) {
	img := gradient(2, 2)
	out := ApplyOrientation(img, 3)
	// Original (0,0) is black; after 180° it lands at (1,1).
	got := color.RGBAModel.Convert(out.At(1, 1)).(color.RGBA)
	if got.R != 0 || got.G != 0 {
		t.Errorf("orientation 3: (1,1) = %+v, want black", got)
	}
	got = color.RGBAModel.Convert(out.At(0, 0)).(color.RGBA)
	if got.R != 50 || got.G != 50 {
		t.Errorf("orientation 3: (0,0) = %+v, want R=50 G=50", got)
	}
}

func TestApplyOrientation_Identity(t *testing.T) {
	img := gradient(3, 2)
	for _, o := range []int{1, 0, 9} {
		out := ApplyOrientation(img, o)
		if out.Bounds() != img.Bounds() {
			t.Errorf("orientation %d changed bounds", o)
		}
	}
}
