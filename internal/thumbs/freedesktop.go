// This file reads thumbnails that Linux desktops have already made. GNOME,
// KDE, Xfce and friends follow the freedesktop.org Thumbnail Managing
// Standard: previews live in ~/.cache/thumbnails/{normal,large,x-large,
// xx-large}/ as PNG files named after the MD5 of the file's URI, and each
// PNG carries the source's modification time in a "Thumb::MTime" text chunk.
// Reusing them gives us previews of videos, PDFs and other formats we cannot
// decode, without shipping any decoders. We only read the cache, never write
// it, and a thumbnail whose recorded mtime differs from the file is ignored.
// The code is platform neutral so it can be tested anywhere; only the Linux
// build wires it in as the system thumbnailer.

package thumbs

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// freedesktopDirs lists the standard size folders with their pixel size.
var freedesktopDirs = []struct {
	name string
	size int
}{{"normal", 128}, {"large", 256}, {"x-large", 512}, {"xx-large", 1024}}

// FreedesktopThumbPath returns the path where a freedesktop thumbnail of
// absPath would be stored in the given size folder under cacheRoot (normally
// $XDG_CACHE_HOME/thumbnails).
func FreedesktopThumbPath(cacheRoot, folder, absPath string) string {
	uri := (&url.URL{Scheme: "file", Path: absPath}).String()
	sum := md5.Sum([]byte(uri))
	return filepath.Join(cacheRoot, folder, hex.EncodeToString(sum[:])+".png")
}

// freedesktopRoot returns $XDG_CACHE_HOME/thumbnails or ~/.cache/thumbnails.
func freedesktopRoot() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "thumbnails")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "thumbnails")
}

// FreedesktopLookup returns a still-valid freedesktop thumbnail of absPath
// from cacheRoot, preferring the smallest folder at least as large as want.
func FreedesktopLookup(ctx context.Context, cacheRoot, absPath string, want int) (image.Image, error) {
	if cacheRoot == "" {
		return nil, ErrUnsupported
	}
	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	mtime := fi.ModTime().Unix()
	// Order: folders big enough for want (smallest first), then smaller ones
	// (largest first) as a last resort.
	var order []string
	for _, d := range freedesktopDirs {
		if d.size >= want {
			order = append(order, d.name)
		}
	}
	for i := len(freedesktopDirs) - 1; i >= 0; i-- {
		if freedesktopDirs[i].size < want {
			order = append(order, freedesktopDirs[i].name)
		}
	}
	for _, folder := range order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(FreedesktopThumbPath(cacheRoot, folder, absPath))
		if err != nil {
			continue
		}
		if v, ok := pngTextChunks(data)["Thumb::MTime"]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err != nil || n != mtime {
				continue // stale
			}
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		return img, nil
	}
	return nil, fmt.Errorf("%w: no freedesktop thumbnail", ErrUnsupported)
}

// pngTextChunks returns the keyword/value pairs of all tEXt chunks.
func pngTextChunks(b []byte) map[string]string {
	out := map[string]string{}
	const sig = "\x89PNG\r\n\x1a\n"
	if !bytes.HasPrefix(b, []byte(sig)) {
		return out
	}
	i := len(sig)
	for i+12 <= len(b) {
		n := int(binary.BigEndian.Uint32(b[i:]))
		typ := string(b[i+4 : i+8])
		if n < 0 || i+12+n > len(b) {
			break
		}
		if typ == "tEXt" {
			body := b[i+8 : i+8+n]
			if k := bytes.IndexByte(body, 0); k > 0 {
				out[string(body[:k])] = string(body[k+1:])
			}
		}
		if typ == "IEND" {
			break
		}
		i += 12 + n
	}
	return out
}
