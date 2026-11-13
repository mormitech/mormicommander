// These tests run the Linux backend against a fake clipboard that understands the same command lines as wl-copy,
// wl-paste, xclip and xsel, so the exact commands, the format chosen per desktop, the cut marks of GNOME and KDE, and
// the error messages for missing tools are all checked on any OS without touching a real clipboard.

package clipfiles

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fakeTools emulates the clipboard tools over an in-memory clipboard of MIME type -> data.
type fakeTools struct {
	tools []string
	clip  map[string][]byte
	order []string // types in the order they were offered
	calls []string
}

func (f *fakeTools) has(name string) bool { return slices.Contains(f.tools, name) }

func (f *fakeTools) set(types map[string][]byte, order ...string) {
	f.clip, f.order = types, order
}

func (f *fakeTools) serve(stdin []byte, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	data := append([]byte(nil), stdin...)
	mime := ""
	for i, a := range args {
		if (a == "--type" || a == "-t") && i+1 < len(args) {
			mime = args[i+1]
		}
	}
	switch name {
	case "wl-copy":
		if mime == MimeText {
			f.set(map[string][]byte{MimeText: data, "text/plain": data, "UTF8_STRING": data}, MimeText, "text/plain", "UTF8_STRING")
		} else {
			f.set(map[string][]byte{mime: data}, mime)
		}
	case "xclip":
		if mime == "" {
			f.set(map[string][]byte{"UTF8_STRING": data, "STRING": data}, "TARGETS", "UTF8_STRING", "STRING")
		} else {
			f.set(map[string][]byte{mime: data}, "TARGETS", mime)
		}
	case "xsel":
		f.set(map[string][]byte{"UTF8_STRING": data}, "UTF8_STRING")
	default:
		return errors.New("unknown tool " + name)
	}
	return nil
}

func (f *fakeTools) output(stdin []byte, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	last := args[len(args)-1]
	switch {
	case name == "wl-paste" && last == "--list-types", name == "xclip" && last == "TARGETS":
		if len(f.clip) == 0 {
			return nil, errors.New("nothing is copied")
		}
		return []byte(strings.Join(f.order, "\n") + "\n"), nil
	case name == "wl-paste", name == "xclip" && slices.Contains(args, "-o"):
		if d, ok := f.clip[last]; ok {
			return d, nil
		}
		return nil, errors.New("no suitable type")
	case name == "xsel" && last == "--output":
		return f.clip["UTF8_STRING"], nil
	case name == "wl-copy" && last == "--clear", name == "xsel" && last == "--clear":
		f.set(nil)
		return nil, nil
	}
	return nil, errors.New("unexpected command " + name + " " + strings.Join(args, " "))
}

func newFake(env map[string]string, tools ...string) (*xdgBackend, *fakeTools) {
	f := &fakeTools{tools: tools}
	return &xdgBackend{r: f, getenv: func(k string) string { return env[k] }}, f
}

var wayland = map[string]string{"WAYLAND_DISPLAY": "wayland-0", "XDG_CURRENT_DESKTOP": "GNOME"}

func TestXDGToolChoice(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		tools   []string
		want    xdgTool
		wantErr string
	}{
		{"wayland", wayland, []string{"wl-copy", "wl-paste", "xclip"}, toolWayland, ""},
		{"wayland without wl-clipboard uses xwayland", map[string]string{"WAYLAND_DISPLAY": "w", "DISPLAY": ":0"},
			[]string{"xclip"}, toolXclip, ""},
		{"x11", map[string]string{"DISPLAY": ":0"}, []string{"xclip", "xsel"}, toolXclip, ""},
		{"x11 xsel only", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, toolXsel, ""},
		{"no session", map[string]string{}, []string{"xclip"}, 0, "no graphical session"},
		{"wayland no tools", map[string]string{"WAYLAND_DISPLAY": "w"}, nil, 0, "wl-clipboard"},
		{"x11 no tools", map[string]string{"DISPLAY": ":0"}, nil, 0, "install xclip"},
	}
	for _, tt := range tests {
		x, _ := newFake(tt.env, tt.tools...)
		got, err := x.tool()
		if tt.wantErr != "" {
			if !errors.Is(err, ErrNoTool) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v", tt.name, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: got %v, %v", tt.name, got, err)
		}
	}
}

func TestXDGFilesFormat(t *testing.T) {
	tests := []struct {
		desktop string
		op      Op
		want    string
	}{
		{"GNOME", Copy, MimeGnomeCopied},
		{"ubuntu:GNOME", Copy, MimeGnomeCopied},
		{"X-Cinnamon", Copy, MimeGnomeCopied},
		{"XFCE", Copy, MimeGnomeCopied},
		{"KDE", Copy, MimeURIList},
		{"sway", Copy, MimeURIList},
		{"", Copy, MimeURIList},
		{"KDE", Cut, MimeGnomeCopied},
	}
	for _, tt := range tests {
		x, _ := newFake(map[string]string{"XDG_CURRENT_DESKTOP": tt.desktop})
		if got := x.filesFormat(tt.op); got != tt.want {
			t.Errorf("%q %v: got %s, want %s", tt.desktop, tt.op, got, tt.want)
		}
	}
	LinuxFileFormat = MimeURIList
	defer func() { LinuxFileFormat = "" }()
	x, _ := newFake(map[string]string{"XDG_CURRENT_DESKTOP": "GNOME"})
	if got := x.filesFormat(Cut); got != MimeURIList {
		t.Errorf("override ignored: %s", got)
	}
}

func TestXDGFilesRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	paths := []string{"/home/u/a b.txt", "/home/u/ünï"}
	for _, env := range []map[string]string{
		wayland,
		{"WAYLAND_DISPLAY": "w", "XDG_CURRENT_DESKTOP": "KDE"},
		{"DISPLAY": ":0", "XDG_CURRENT_DESKTOP": "XFCE"},
		{"DISPLAY": ":0", "XDG_CURRENT_DESKTOP": "KDE"},
	} {
		for _, op := range []Op{Copy, Cut} {
			x, f := newFake(env, "wl-copy", "wl-paste", "xclip")
			if err := x.writeFiles(paths, op); err != nil {
				t.Fatal(err)
			}
			got, gotOp, err := x.readFiles()
			if err != nil || gotOp != op || !reflect.DeepEqual(got, paths) {
				t.Errorf("%v %v: got %q %v %v (calls %q)", env, op, got, gotOp, err, f.calls)
			}
		}
	}
}

func TestXDGExactCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	x, f := newFake(wayland, "wl-copy", "wl-paste")
	if err := x.writeFiles([]string{"/a"}, Cut); err != nil {
		t.Fatal(err)
	}
	if want := "wl-copy --type " + MimeGnomeCopied; f.calls[0] != want {
		t.Errorf("call %q, want %q", f.calls[0], want)
	}
	if got := string(f.clip[MimeGnomeCopied]); got != "cut\nfile:///a" {
		t.Errorf("payload %q", got)
	}

	x, f = newFake(map[string]string{"DISPLAY": ":0"}, "xclip")
	if err := x.writeText("héllo"); err != nil {
		t.Fatal(err)
	}
	if f.calls[0] != "xclip -selection clipboard -in" {
		t.Errorf("text call %q", f.calls[0])
	}
	if err := x.writeFiles([]string{"/a"}, Copy); err != nil {
		t.Fatal(err)
	}
	if want := "xclip -selection clipboard -in -t " + MimeURIList; f.calls[1] != want {
		t.Errorf("files call %q, want %q", f.calls[1], want)
	}
	if got := string(f.clip[MimeURIList]); got != "file:///a\r\n" {
		t.Errorf("uri-list payload %q", got)
	}
}

func TestXDGKDECut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	x, f := newFake(wayland, "wl-copy", "wl-paste")
	f.set(map[string][]byte{MimeURIList: []byte("file:///a\nfile:///b\n"), MimeKDECut: []byte("1")},
		MimeURIList, MimeKDECut)
	got, op, err := x.readFiles()
	if err != nil || op != Cut || !reflect.DeepEqual(got, []string{"/a", "/b"}) {
		t.Fatalf("got %q %v %v", got, op, err)
	}
	f.clip[MimeKDECut] = []byte("0")
	if _, op, _ := x.readFiles(); op != Copy {
		t.Fatal("x-kde-cutselection 0 must mean copy")
	}
}

func TestXDGEmptyAndText(t *testing.T) {
	x, f := newFake(wayland, "wl-copy", "wl-paste")
	if _, _, err := x.readFiles(); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty clipboard files: %v", err)
	}
	if _, err := x.readText(); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty clipboard text: %v", err)
	}
	if err := x.writeText("line1\nline2\n"); err != nil {
		t.Fatal(err)
	}
	if s, err := x.readText(); err != nil || s != "line1\nline2\n" {
		t.Errorf("text round trip %q %v", s, err)
	}
	if _, _, err := x.readFiles(); !errors.Is(err, ErrEmpty) {
		t.Errorf("text is not files: %v", err)
	}
	c, err := x.content()
	if err != nil || !c.Text || c.Files || c.Image {
		t.Errorf("content %+v %v", c, err)
	}
	if err := x.clear(); err != nil || len(f.clip) != 0 {
		t.Errorf("clear: %v %v", err, f.clip)
	}
}

func TestXDGXsel(t *testing.T) {
	x, _ := newFake(map[string]string{"DISPLAY": ":0"}, "xsel")
	if err := x.writeText("abc"); err != nil {
		t.Fatal(err)
	}
	if s, err := x.readText(); err != nil || s != "abc" {
		t.Errorf("xsel text %q %v", s, err)
	}
	if err := x.writeFiles([]string{"/a"}, Copy); !errors.Is(err, ErrNoTool) {
		t.Errorf("xsel files write: %v", err)
	}
	if _, _, err := x.readFiles(); !errors.Is(err, ErrNoTool) {
		t.Errorf("xsel files read: %v", err)
	}
	if c, _ := x.content(); !c.Text {
		t.Errorf("xsel content %+v", c)
	}
}

func TestXDGImages(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(1, 1, color.NRGBA{255, 0, 0, 255})
	var pngBuf, jpgBuf bytes.Buffer
	png.Encode(&pngBuf, img)
	jpeg.Encode(&jpgBuf, img, nil)
	bmp := append([]byte("BM"), make([]byte, 12)...)
	bmp = append(bmp, EncodeDIB(img)...)

	x, f := newFake(wayland, "wl-copy", "wl-paste")
	if err := x.writeImage(pngBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if got, err := x.readImage(); err != nil || !bytes.Equal(got, pngBuf.Bytes()) {
		t.Errorf("png round trip: %v", err)
	}
	for _, tt := range []struct {
		mime string
		data []byte
	}{{"image/jpeg", jpgBuf.Bytes()}, {"image/bmp", bmp}} {
		f.set(map[string][]byte{tt.mime: tt.data}, tt.mime)
		got, err := x.readImage()
		if err != nil {
			t.Errorf("%s: %v", tt.mime, err)
			continue
		}
		dec, err := png.Decode(bytes.NewReader(got))
		if err != nil || dec.Bounds().Dx() != 2 {
			t.Errorf("%s: not converted to PNG: %v", tt.mime, err)
		}
	}
	f.set(map[string][]byte{"image/bmp": []byte("XX")}, "image/bmp")
	if _, err := x.readImage(); err == nil {
		t.Error("bad BMP accepted")
	}
	if c, _ := x.content(); !c.Image {
		t.Errorf("content %+v", c)
	}
}
