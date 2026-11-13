// On Linux the "system thumbnailer" is the freedesktop.org thumbnail cache
// that the desktop environment maintains (see freedesktop.go). We read it
// but never run the desktop's thumbnailer programs ourselves, because their
// set and behaviour differ between distributions.

package thumbs

import (
	"context"
	"image"
)

func systemThumbnailer() SystemFunc {
	return func(ctx context.Context, path string, w, h int) (image.Image, error) {
		return FreedesktopLookup(ctx, freedesktopRoot(), path, max(w, h))
	}
}
