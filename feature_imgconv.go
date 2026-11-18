package main

// HEIC, AVIF and JPEG XL where the web view or Go can't read them (issue #25): the thumbnails view, the Lister and
// Quick View hand such a picture to a program the computer already has (internal/imgconv: sips on macOS, the WIC
// codecs on Windows, or libheif/libavif/libjxl/ImageMagick/vips/ffmpeg when installed), or to the one named in
// Options > Plugins > "Program for HEIC, AVIF and JPEG XL pictures". Turning external tools off in Options > Plugins
// turns this off too. Nothing is downloaded and nothing goes online.

import (
	"context"
	"encoding/base64"
	"errors"
	"image"
	"os"

	"mormicommander/internal/imgconv"
	"mormicommander/internal/thumbs"
)

// imgconvOptions are the converter settings from Options > Plugins.
func imgconvOptions() imgconv.Options {
	s, ok := currentSettings()
	if !ok {
		return imgconv.Options{}
	}
	return imgconv.Options{Program: s.Plugins.ImageDecoder, Off: !s.Plugins.UseExternalTools}
}

// imgconvThumb is the thumbnailer's last resort for HEIC, AVIF and JPEG XL files that neither Go nor the system
// thumbnailer (QuickLook, the freedesktop cache) gave a picture for. Other files are left alone at once.
func imgconvThumb(ctx context.Context, path string, w, h int) (image.Image, error) {
	if imgconv.ExtFormat(path) == "" && imgconv.SniffFile(path) == "" {
		return nil, thumbs.ErrUnsupported
	}
	img, _, err := imgconv.Decode(ctx, path, imgconvOptions())
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.Join(thumbs.ErrUnsupported, err)
	}
	return img, nil
}

// ListerDecoded is a HEIC, AVIF or JPEG XL picture converted for the web view.
type ListerDecoded struct {
	URL   string `json:"url"`   // data: URL of the converted picture, "" when it could not be converted
	Tool  string `json:"tool"`  // the program that converted it
	Error string `json:"error"` // why not, for the page to show
}

// Decoded converts the Lister's HEIC, AVIF or JPEG XL picture for a web view that can't draw it itself (the page asks
// only after its <img> failed, so on macOS, where WebKit reads these formats, it is never needed).
func (l *Lister) Decoded(id int) (ListerDecoded, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerDecoded{}, err
	}
	if fi, err := os.Stat(s.path); err != nil || !fi.Mode().IsRegular() {
		return ListerDecoded{Error: "This picture can't be converted."}, nil
	}
	o := imgconvOptions()
	mime, data, tool, err := imgconv.DecodeForView(context.Background(), s.path, o)
	switch {
	case errors.Is(err, imgconv.ErrNoTool):
		if o.Off {
			return ListerDecoded{Error: "Showing this kind of picture needs an external program, and external tools are turned off (Options > Plugins)."}, nil
		}
		return ListerDecoded{Error: "Showing this kind of picture needs a program that reads it, for example libheif (heif-dec), libavif (avifdec), libjxl (djxl), ImageMagick or ffmpeg; or name one in Options > Plugins."}, nil
	case errors.Is(err, imgconv.ErrNotSupported):
		return ListerDecoded{Error: "This is not a HEIC, AVIF or JPEG XL picture."}, nil
	case errors.Is(err, imgconv.ErrTooLarge):
		return ListerDecoded{Tool: tool, Error: "This picture is too large to show here."}, nil
	case err != nil:
		return ListerDecoded{Tool: tool, Error: "The picture could not be converted (" + tool + ")."}, nil
	}
	return ListerDecoded{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), Tool: tool}, nil
}
