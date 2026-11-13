// Package thumbs produces the small preview images that the thumbnails view
// (Total Commander's Ctrl+Shift+F1) shows instead of plain file icons. It
// decodes JPEG, PNG, GIF, BMP, WebP and TIFF files with the Go standard
// library and golang.org/x/image, turns the picture upright according to its
// EXIF orientation tag (phone photos are usually stored sideways), scales it
// to fit a configurable box between 32 and 512 pixels, and encodes the result
// as JPEG (opaque images, small) or PNG (images with transparency). Formats we
// cannot decode ourselves, such as HEIC, PDF or video, can be handed to the
// operating system's own thumbnailer. The package has no UI: the panel code
// asks a Thumbnailer (optionally backed by a disk Cache) or a background Pool
// for thumbnails, and falls back to the generic file-type icons in icons.go
// when a file has no preview.
package thumbs

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
)

// Size limits for the thumbnail box. Total Commander lets the user pick the
// thumbnail width and height freely; we keep the same range as the FEATURES
// checklist (32..512 px) so a single huge setting cannot make the cache explode.
const (
	MinSize     = 32
	MaxSize     = 512
	DefaultSize = 96
)

// Default safety limits. A thumbnail run walks whole folders, so one broken or
// hostile file (a 2 GB TIFF, a "decompression bomb" PNG that claims to be
// 100000x100000) must not stall the view or exhaust memory.
const (
	DefaultMaxSourceBytes = 256 << 20 // 256 MiB of encoded data
	DefaultMaxPixels      = 100 << 20 // ~100 megapixels decoded
	DefaultJPEGQuality    = 85
)

// Errors returned by Thumbnailer.Thumbnail and Generate. The UI treats all of
// them the same way (show the file-type icon), but tests and logs can tell
// them apart with errors.Is.
var (
	// ErrUnsupported means the file is not an image format we can decode and
	// no system thumbnailer produced a preview for it.
	ErrUnsupported = errors.New("thumbs: unsupported file type")
	// ErrDecode means the file looked like an image but could not be decoded.
	ErrDecode = errors.New("thumbs: cannot decode image")
	// ErrTooLarge means the file or its pixel count exceeds the safety limits.
	ErrTooLarge = errors.New("thumbs: image too large")
	// ErrIsDir is returned for directories, which never get thumbnails.
	ErrIsDir = errors.New("thumbs: is a directory")
)

// Options controls how thumbnails are generated. The zero value is usable
// (sizes fall back to DefaultSize and limits to their defaults), but
// DefaultOptions also enables the system thumbnailer.
type Options struct {
	// Width and Height of the box the thumbnail must fit in. Zero means
	// DefaultSize; values are clamped to MinSize..MaxSize. The aspect ratio of
	// the picture is always kept, as in Total Commander.
	Width, Height int
	// Upscale enlarges images that are smaller than the box. Total Commander
	// does not enlarge small images by default, and neither do we.
	Upscale bool
	// UseSystem asks the operating system's thumbnailer (QuickLook on macOS,
	// the freedesktop thumbnail cache on Linux) for files we cannot decode.
	UseSystem bool
	// MaxSourceBytes and MaxPixels are safety limits; zero means default.
	MaxSourceBytes int64
	MaxPixels      int64
	// JPEGQuality for opaque thumbnails (1..100); zero means default.
	JPEGQuality int
}

// DefaultOptions returns the settings a fresh install uses: 96 px square box,
// no upscaling, system thumbnailer enabled.
func DefaultOptions() Options {
	return Options{Width: DefaultSize, Height: DefaultSize, UseSystem: true}
}

// ClampSize forces a requested edge length into MinSize..MaxSize, mapping zero
// or negative values to DefaultSize.
func ClampSize(n int) int {
	switch {
	case n <= 0:
		return DefaultSize
	case n < MinSize:
		return MinSize
	case n > MaxSize:
		return MaxSize
	}
	return n
}

// normalized returns a copy with every field filled in and in range, so the
// rest of the package never has to repeat the defaulting logic.
func (o Options) normalized() Options {
	o.Width = ClampSize(o.Width)
	o.Height = ClampSize(o.Height)
	if o.MaxSourceBytes <= 0 {
		o.MaxSourceBytes = DefaultMaxSourceBytes
	}
	if o.MaxPixels <= 0 {
		o.MaxPixels = DefaultMaxPixels
	}
	if o.JPEGQuality <= 0 || o.JPEGQuality > 100 {
		o.JPEGQuality = DefaultJPEGQuality
	}
	return o
}

// Meta describes the original image, so the thumbnails view can show
// "1920 x 1080" style field lines under the picture (FEATURES F-679).
type Meta struct {
	// Format is the decoder name: "jpeg", "png", "gif", "bmp", "webp",
	// "tiff", or "system" when the OS thumbnailer produced the preview.
	Format string `json:"format"`
	// Width and Height of the original image as displayed, i.e. after the
	// EXIF orientation was applied. Zero when unknown (system thumbnails).
	Width  int `json:"width"`
	Height int `json:"height"`
	// Orientation is the EXIF orientation tag (1..8); 1 means upright.
	Orientation int `json:"orientation"`
}

// Thumb is a finished thumbnail: encoded bytes ready to hand to the web view
// plus the metadata of the source picture.
type Thumb struct {
	Meta Meta `json:"meta"`
	// Width and Height of the thumbnail itself.
	Width  int `json:"width"`
	Height int `json:"height"`
	// MIME is "image/jpeg" or "image/png".
	MIME string `json:"mime"`
	// Data holds the encoded thumbnail.
	Data []byte `json:"-"`
	// FromCache reports whether the thumbnail came from the disk cache.
	FromCache bool `json:"fromCache"`
}

// Image decodes the thumbnail bytes back into an image.
func (t *Thumb) Image() (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(t.Data))
	return img, err
}

// DataURL returns the thumbnail as a data: URL, the simplest way to pass it
// through the Wails bridge into an <img> element.
func (t *Thumb) DataURL() string {
	return "data:" + t.MIME + ";base64," + base64.StdEncoding.EncodeToString(t.Data)
}
