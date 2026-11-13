// This file supplies the generic file-type icons that the thumbnails view
// shows for files without a preview (and while previews are still being
// generated). Total Commander uses the shell's icons there; we cannot rely on
// every platform giving us those, so every file gets a portable fallback: a
// kind derived from its extension (image, video, archive, document, ...), a
// colour per kind and a short label with the extension, and the icon can be
// rendered either as SVG (crisp at any thumbnail size, what the web view
// uses) or as a raster image for code paths that need pixels.

package thumbs

import (
	"bytes"
	"fmt"
	"html"
	"image"
	"image/color"
	"path/filepath"
	"strings"
)

// Kind is a coarse file category used for fallback icons.
type Kind string

// Known kinds.
const (
	KindFolder       Kind = "folder"
	KindParent       Kind = "parent"
	KindImage        Kind = "image"
	KindVideo        Kind = "video"
	KindAudio        Kind = "audio"
	KindArchive      Kind = "archive"
	KindPDF          Kind = "pdf"
	KindDocument     Kind = "document"
	KindSpreadsheet  Kind = "spreadsheet"
	KindPresentation Kind = "presentation"
	KindText         Kind = "text"
	KindCode         Kind = "code"
	KindExecutable   Kind = "executable"
	KindFont         Kind = "font"
	KindDiskImage    Kind = "diskimage"
	KindGeneric      Kind = "generic"
)

// kindByExt maps lower-case extensions (without dot) to kinds.
var kindByExt = map[string]Kind{}

func init() {
	add := func(k Kind, exts string) {
		for _, e := range strings.Fields(exts) {
			kindByExt[e] = k
		}
	}
	add(KindImage, "jpg jpeg jpe jfif png gif bmp dib webp tif tiff heic heif avif ico icns svg psd raw cr2 cr3 nef arw dng orf rw2 tga pcx jxl")
	add(KindVideo, "mp4 m4v mkv mov avi wmv flv webm mpg mpeg m2ts mts ts 3gp ogv vob")
	add(KindAudio, "mp3 wav flac aac m4a ogg oga opus wma aiff aif mid midi ape alac")
	add(KindArchive, "zip 7z rar tar gz tgz bz2 tbz tbz2 xz txz zst lz lzma z cab arj lzh lha ace jar war apk deb rpm cpio")
	add(KindPDF, "pdf")
	add(KindDocument, "doc docx odt rtf pages wpd epub mobi djvu")
	add(KindSpreadsheet, "xls xlsx ods csv tsv numbers")
	add(KindPresentation, "ppt pptx odp key")
	add(KindText, "txt md markdown log ini cfg conf nfo diz rst tex")
	add(KindCode, "go c h cc cpp hpp cs java kt js mjs ts tsx jsx py rb php pl rs swift m mm sh bash zsh ps1 bat cmd lua sql html htm css scss json xml yaml yml toml vue dart r scala")
	add(KindExecutable, "exe com msi app dll so dylib bin run appimage")
	add(KindFont, "ttf otf woff woff2 fon")
	add(KindDiskImage, "iso img dmg vhd vhdx vmdk nrg")
}

// KindOf returns the kind for a file name based on its extension.
func KindOf(name string) Kind {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if k, ok := kindByExt[ext]; ok {
		return k
	}
	// Multi-part extensions such as .tar.gz are already covered by the last
	// part; a name without extension stays generic.
	return KindGeneric
}

// kindColors are muted, readable on both light and dark panels.
var kindColors = map[Kind]string{
	KindFolder:       "#E2B33C",
	KindParent:       "#9AA0A6",
	KindImage:        "#3FA34D",
	KindVideo:        "#D9534F",
	KindAudio:        "#9B59B6",
	KindArchive:      "#B7791F",
	KindPDF:          "#C0392B",
	KindDocument:     "#2B6CB0",
	KindSpreadsheet:  "#217346",
	KindPresentation: "#D35400",
	KindText:         "#718096",
	KindCode:         "#5A67D8",
	KindExecutable:   "#2D3748",
	KindFont:         "#805AD5",
	KindDiskImage:    "#4A5568",
	KindGeneric:      "#A0AEC0",
}

// Icon is the fallback icon description for one file.
type Icon struct {
	Kind  Kind   `json:"kind"`
	Label string `json:"label"` // short upper-case extension, may be empty
	Color string `json:"color"` // "#RRGGBB"
}

// IconFor returns the fallback icon for a file name. name ".." yields the
// parent-directory icon.
func IconFor(name string, isDir bool) Icon {
	base := filepath.Base(name)
	switch {
	case base == "..":
		return Icon{Kind: KindParent, Color: kindColors[KindParent]}
	case isDir:
		return Icon{Kind: KindFolder, Color: kindColors[KindFolder]}
	}
	k := KindOf(base)
	label := strings.ToUpper(strings.TrimPrefix(filepath.Ext(base), "."))
	if len([]rune(label)) > 4 {
		label = string([]rune(label)[:4])
	}
	return Icon{Kind: k, Label: label, Color: kindColors[k]}
}

// SVG renders the icon as a standalone SVG document of size x size pixels.
func (ic Icon) SVG(size int) []byte {
	size = max(16, size)
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 64 64">`, size, size)
	switch ic.Kind {
	case KindFolder:
		fmt.Fprintf(&b, `<path d="M4 14a4 4 0 0 1 4-4h16l6 6h26a4 4 0 0 1 4 4v30a4 4 0 0 1-4 4H8a4 4 0 0 1-4-4z" fill="%s"/>`, ic.Color)
		b.WriteString(`<path d="M4 22h56v28a4 4 0 0 1-4 4H8a4 4 0 0 1-4-4z" fill="#ffffff" fill-opacity="0.18"/>`)
	case KindParent:
		fmt.Fprintf(&b, `<path d="M32 12 14 32h11v18h14V32h11z" fill="%s"/>`, ic.Color)
	default:
		b.WriteString(`<path d="M14 4h26l14 14v40a2 2 0 0 1-2 2H14a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z" fill="#f7f7f7" stroke="#b8b8b8" stroke-width="1.5"/>`)
		b.WriteString(`<path d="M40 4v12a2 2 0 0 0 2 2h12" fill="#e2e2e2" stroke="#b8b8b8" stroke-width="1.5"/>`)
		fmt.Fprintf(&b, `<rect x="6" y="36" width="40" height="16" rx="2" fill="%s"/>`, ic.Color)
		if ic.Label != "" {
			fs := 11
			if len([]rune(ic.Label)) >= 4 {
				fs = 9
			}
			fmt.Fprintf(&b, `<text x="26" y="47.5" font-family="Helvetica,Arial,sans-serif" font-size="%d" font-weight="700" fill="#ffffff" text-anchor="middle">%s</text>`, fs, html.EscapeString(ic.Label))
		}
	}
	b.WriteString(`</svg>`)
	return b.Bytes()
}

// Image renders the icon as size x size pixels without the text label: a
// sheet with a coloured band for files, a coloured folder for directories.
func (ic Icon) Image(size int) *image.RGBA {
	size = max(16, size)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := parseHex(ic.Color)
	s := func(v int) int { return v * size / 64 }
	fill := func(x0, y0, x1, y1 int, col color.RGBA) {
		for y := s(y0); y < s(y1); y++ {
			for x := s(x0); x < s(x1); x++ {
				img.SetRGBA(x, y, col)
			}
		}
	}
	switch ic.Kind {
	case KindFolder, KindParent:
		fill(4, 10, 26, 16, c)
		fill(4, 16, 60, 54, c)
	default:
		edge := color.RGBA{0xb8, 0xb8, 0xb8, 0xff}
		fill(12, 4, 54, 60, edge)
		fill(13, 5, 53, 59, color.RGBA{0xf7, 0xf7, 0xf7, 0xff})
		fill(6, 36, 46, 52, c)
	}
	return img
}

func parseHex(h string) color.RGBA {
	var r, g, b uint8
	if _, err := fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{0xa0, 0xae, 0xc0, 0xff}
	}
	return color.RGBA{r, g, b, 0xff}
}

// HasNativePreview reports whether a file name has an extension this package
// can usually decode itself (the UI may use it to decide which files to
// queue first). The decision in Thumbnail is made from the file contents,
// not the name.
func HasNativePreview(name string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "jpg", "jpeg", "jpe", "jfif", "png", "gif", "bmp", "dib", "webp", "tif", "tiff":
		return true
	}
	return false
}
