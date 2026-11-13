// This file is the Linux clipboard backend. Linux has no single clipboard API without linking a toolkit, so it drives
// the standard command-line tools: wl-copy and wl-paste (wl-clipboard) on Wayland, xclip on X11, and xsel as a last,
// text-only fallback. Each tool offers one data type per copy, so a file list goes on the clipboard in the one format
// the running desktop's file manager reads best: x-special/gnome-copied-files on GNOME, Cinnamon, MATE, Xfce, LXDE and
// their relatives (that format is also the only one that tells a cut from a copy there), and text/uri-list on KDE and
// everywhere else. Reading accepts all of them, plus KDE's application/x-kde-cutselection marker. The code has no
// build tag so its command lines are tested on every OS with a fake runner; clip_linux.go selects it on Linux.

package clipfiles

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"slices"
	"strings"
)

// LinuxFileFormat overrides the format a file list is written in on Linux: MimeGnomeCopied or MimeURIList. Empty (the
// default) picks by desktop as described above. A cut always uses MimeGnomeCopied unless this is set, since
// text/uri-list cannot carry the cut mark.
var LinuxFileFormat = ""

type xdgTool int

const (
	toolWayland xdgTool = iota
	toolXclip
	toolXsel
)

// xdgBackend talks to the clipboard through command-line tools.
type xdgBackend struct {
	r      runner
	getenv func(string) string
}

// tool picks the clipboard tool for the current session.
func (x *xdgBackend) tool() (xdgTool, error) {
	wayland, display := x.getenv("WAYLAND_DISPLAY") != "", x.getenv("DISPLAY") != ""
	if wayland && x.r.has("wl-copy") && x.r.has("wl-paste") {
		return toolWayland, nil
	}
	if display && x.r.has("xclip") {
		return toolXclip, nil // also covers Wayland sessions with XWayland when wl-clipboard is missing
	}
	if display && x.r.has("xsel") {
		return toolXsel, nil
	}
	if !wayland && !display {
		return 0, fmt.Errorf("%w: no graphical session (DISPLAY and WAYLAND_DISPLAY are not set)", ErrNoTool)
	}
	if wayland {
		return 0, fmt.Errorf("%w: install wl-clipboard (wl-copy, wl-paste)", ErrNoTool)
	}
	return 0, fmt.Errorf("%w: install xclip", ErrNoTool)
}

// filesFormat picks the MIME type a file list is written in.
func (x *xdgBackend) filesFormat(op Op) string {
	if LinuxFileFormat != "" {
		return LinuxFileFormat
	}
	if op == Cut {
		return MimeGnomeCopied
	}
	desk := strings.ToUpper(x.getenv("XDG_CURRENT_DESKTOP") + ":" + x.getenv("DESKTOP_SESSION"))
	for _, d := range []string{"GNOME", "UNITY", "CINNAMON", "X-CINNAMON", "MATE", "XFCE", "LXDE", "BUDGIE", "PANTHEON",
		"DEEPIN", "UBUNTU", "POP", "COSMIC"} {
		if strings.Contains(desk, d) {
			return MimeGnomeCopied
		}
	}
	return MimeURIList
}

// write puts data of one MIME type on the clipboard.
func (x *xdgBackend) write(mime string, data []byte) error {
	t, err := x.tool()
	if err != nil {
		return err
	}
	switch t {
	case toolWayland:
		return x.r.serve(data, "wl-copy", "--type", mime)
	case toolXclip:
		if mime == MimeText {
			// Without -t xclip serves UTF8_STRING, STRING and TEXT, which is what X11 programs ask for.
			return x.r.serve(data, "xclip", "-selection", "clipboard", "-in")
		}
		return x.r.serve(data, "xclip", "-selection", "clipboard", "-in", "-t", mime)
	default:
		if mime != MimeText {
			return fmt.Errorf("%w: xsel can only copy text; install xclip to copy files or images", ErrNoTool)
		}
		return x.r.serve(data, "xsel", "--clipboard", "--input")
	}
}

// types lists the clipboard's data types. An empty clipboard gives an empty list, not an error.
func (x *xdgBackend) types() ([]string, xdgTool, error) {
	t, err := x.tool()
	if err != nil {
		return nil, t, err
	}
	var out []byte
	switch t {
	case toolWayland:
		out, err = x.r.output(nil, "wl-paste", "--list-types")
	case toolXclip:
		out, err = x.r.output(nil, "xclip", "-selection", "clipboard", "-o", "-t", "TARGETS")
	default:
		return nil, t, nil // xsel cannot list types
	}
	if err != nil {
		return nil, t, nil // both tools exit non-zero when nothing owns the clipboard
	}
	var list []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			list = append(list, l)
		}
	}
	return list, t, nil
}

// read returns the clipboard's data of one type.
func (x *xdgBackend) read(t xdgTool, mime string) ([]byte, error) {
	switch t {
	case toolWayland:
		return x.r.output(nil, "wl-paste", "--no-newline", "--type", mime)
	case toolXclip:
		return x.r.output(nil, "xclip", "-selection", "clipboard", "-o", "-t", mime)
	default:
		return x.r.output(nil, "xsel", "--clipboard", "--output")
	}
}

func (x *xdgBackend) writeFiles(paths []string, op Op) error {
	switch mime := x.filesFormat(op); mime {
	case MimeGnomeCopied:
		return x.write(mime, []byte(EncodeGnomeCopied(paths, op)))
	default:
		return x.write(MimeURIList, []byte(EncodeURIList(paths)))
	}
}

func (x *xdgBackend) readFiles() ([]string, Op, error) {
	types, t, err := x.types()
	if err != nil {
		return nil, Copy, err
	}
	if t == toolXsel {
		return nil, Copy, fmt.Errorf("%w: xsel cannot read file lists; install xclip", ErrNoTool)
	}
	if slices.Contains(types, MimeGnomeCopied) {
		data, err := x.read(t, MimeGnomeCopied)
		if err == nil {
			if paths, op, ok := ParseGnomeCopied(string(data)); ok && len(paths) > 0 {
				return paths, op, nil
			}
		}
	}
	if slices.Contains(types, MimeURIList) {
		data, err := x.read(t, MimeURIList)
		if err != nil {
			return nil, Copy, err
		}
		paths := ParseURIList(string(data))
		if len(paths) == 0 {
			return nil, Copy, ErrEmpty
		}
		op := Copy
		if slices.Contains(types, MimeKDECut) {
			if mark, err := x.read(t, MimeKDECut); err == nil && strings.TrimSpace(string(mark)) == "1" {
				op = Cut
			}
		}
		return paths, op, nil
	}
	return nil, Copy, ErrEmpty
}

// textTypes lists text targets in order of preference.
var textTypes = []string{MimeText, "UTF8_STRING", "text/plain;charset=UTF-8", "text/plain", "STRING", "TEXT"}

func (x *xdgBackend) writeText(s string) error { return x.write(MimeText, []byte(s)) }

func (x *xdgBackend) readText() (string, error) {
	types, t, err := x.types()
	if err != nil {
		return "", err
	}
	if t == toolXsel {
		out, err := x.read(t, "")
		if err != nil || len(out) == 0 {
			return "", ErrEmpty
		}
		return string(out), nil
	}
	for _, tt := range textTypes {
		if slices.Contains(types, tt) {
			out, err := x.read(t, tt)
			if err != nil {
				return "", err
			}
			return string(out), nil
		}
	}
	return "", ErrEmpty
}

func (x *xdgBackend) writeImage(data []byte) error { return x.write(MimePNG, data) }

// imageTypes lists image targets in order of preference; all but PNG are converted.
var imageTypes = []string{MimePNG, "image/bmp", "image/x-bmp", "image/jpeg", "image/gif"}

func (x *xdgBackend) readImage() ([]byte, error) {
	types, t, err := x.types()
	if err != nil {
		return nil, err
	}
	for _, it := range imageTypes {
		if !slices.Contains(types, it) {
			continue
		}
		data, err := x.read(t, it)
		if err != nil {
			return nil, err
		}
		return toPNG(it, data)
	}
	return nil, ErrEmpty
}

// toPNG converts image data of the given MIME type to PNG.
func toPNG(mime string, data []byte) ([]byte, error) {
	var img image.Image
	var err error
	switch mime {
	case MimePNG:
		return data, nil
	case "image/bmp", "image/x-bmp":
		if len(data) < 14 || data[0] != 'B' || data[1] != 'M' {
			return nil, errors.New("clipboard BMP image has no file header")
		}
		img, err = DecodeDIB(data[14:])
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "image/gif":
		img, err = gif.Decode(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("unsupported image type %s", mime)
	}
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (x *xdgBackend) content() (Content, error) {
	types, t, err := x.types()
	if err != nil {
		return Content{}, err
	}
	var c Content
	if t == toolXsel {
		if s, err := x.readText(); err == nil && s != "" {
			c.Text = true
		}
		return c, nil
	}
	c.Types = types
	c.Files = slices.Contains(types, MimeGnomeCopied) || slices.Contains(types, MimeURIList)
	for _, tt := range textTypes {
		c.Text = c.Text || slices.Contains(types, tt)
	}
	for _, it := range imageTypes {
		c.Image = c.Image || slices.Contains(types, it)
	}
	return c, nil
}

func (x *xdgBackend) clear() error {
	t, err := x.tool()
	if err != nil {
		return err
	}
	switch t {
	case toolWayland:
		_, err = x.r.output(nil, "wl-copy", "--clear")
	case toolXclip:
		err = x.r.serve(nil, "xclip", "-selection", "clipboard", "-in") // X11 has no "empty"; own it with no text
	default:
		_, err = x.r.output(nil, "xsel", "--clipboard", "--clear")
	}
	return err
}
