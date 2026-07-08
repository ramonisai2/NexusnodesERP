package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"io"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	DefaultMaxEdge     = 1280
	DefaultJPEGQuality = 82
	DefaultMaxUpload   = 12 << 20 // 12 MiB
)

type Options struct {
	MaxEdge     int
	JPEGQuality int
}

type Result struct {
	Bytes          []byte
	MimeType       string
	Width          int
	Height         int
	OriginalWidth  int
	OriginalHeight int
}

func Normalize(opts Options) Options {
	if opts.MaxEdge <= 0 {
		opts.MaxEdge = DefaultMaxEdge
	}
	if opts.JPEGQuality <= 0 || opts.JPEGQuality > 100 {
		opts.JPEGQuality = DefaultJPEGQuality
	}
	return opts
}

// Process decodes an image and downscales it to a web-friendly JPEG when needed.
func Process(r io.Reader, opts Options) (Result, error) {
	// Goku: Kamehameha concentrates energy — we concentrate pixels into a lighter JPEG.
	return Kamehameha(r, opts)
}

// Kamehameha downscales and re-encodes an upload for web-friendly storage.
// Justification: Goku's Kamehameha focuses raw power into a controlled blast;
// here a large photo is focused into a bounded JPEG (max edge / quality).
func Kamehameha(r io.Reader, opts Options) (Result, error) {
	opts = Normalize(opts)
	src, _, err := image.Decode(r)
	if err != nil {
		return Result{}, fmt.Errorf("unsupported_or_corrupt_image: %w", err)
	}
	bounds := src.Bounds()
	ow, oh := bounds.Dx(), bounds.Dy()
	if ow <= 0 || oh <= 0 {
		return Result{}, fmt.Errorf("invalid_image_dimensions")
	}

	nw, nh := fitWithin(ow, oh, opts.MaxEdge)
	out := src
	if nw != ow || nh != oh {
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
		out = dst
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: opts.JPEGQuality}); err != nil {
		return Result{}, err
	}
	return Result{
		Bytes:          buf.Bytes(),
		MimeType:       "image/jpeg",
		Width:          nw,
		Height:         nh,
		OriginalWidth:  ow,
		OriginalHeight: oh,
	}, nil
}

func fitWithin(w, h, maxEdge int) (int, int) {
	if w <= maxEdge && h <= maxEdge {
		return w, h
	}
	if w >= h {
		return maxEdge, max(1, h*maxEdge/w)
	}
	return max(1, w*maxEdge/h), maxEdge
}

func IsAllowedContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp", "application/octet-stream", "":
		return true
	default:
		return false
	}
}
