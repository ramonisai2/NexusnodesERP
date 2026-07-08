package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestProcessDownscalesLargeJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3200, 1800))
	for y := 0; y < 1800; y++ {
		for x := 0; x < 3200; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 90, A: 255})
		}
	}
	var in bytes.Buffer
	if err := jpeg.Encode(&in, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	inLen := in.Len()
	out, err := Process(&in, Options{MaxEdge: 1280, JPEGQuality: 82})
	if err != nil {
		t.Fatal(err)
	}
	if out.Width != 1280 || out.Height != 720 {
		t.Fatalf("got %dx%d want 1280x720", out.Width, out.Height)
	}
	if out.OriginalWidth != 3200 || out.OriginalHeight != 1800 {
		t.Fatalf("original dims %dx%d", out.OriginalWidth, out.OriginalHeight)
	}
	if out.MimeType != "image/jpeg" {
		t.Fatalf("mime %s", out.MimeType)
	}
	if len(out.Bytes) == 0 || len(out.Bytes) >= inLen {
		t.Fatalf("expected smaller jpeg, in=%d out=%d", inLen, len(out.Bytes))
	}
}

func TestProcessKeepsSmallPNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 300))
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	out, err := Process(&in, Options{MaxEdge: 1280})
	if err != nil {
		t.Fatal(err)
	}
	if out.Width != 400 || out.Height != 300 {
		t.Fatalf("got %dx%d", out.Width, out.Height)
	}
}

func TestFitWithin(t *testing.T) {
	w, h := fitWithin(100, 50, 1280)
	if w != 100 || h != 50 {
		t.Fatalf("no-op failed: %dx%d", w, h)
	}
	w, h = fitWithin(2000, 1000, 1280)
	if w != 1280 || h != 640 {
		t.Fatalf("landscape: %dx%d", w, h)
	}
	w, h = fitWithin(1000, 2000, 1280)
	if w != 640 || h != 1280 {
		t.Fatalf("portrait: %dx%d", w, h)
	}
}
