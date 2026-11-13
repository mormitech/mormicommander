// Tests for line splitting with mixed endings, document encoding round trips
// and atomic saving. Files live in t.TempDir().
package compare

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		in   string
		want []Line
	}{
		{"", nil},
		{"a", []Line{{"a", ""}}},
		{"a\n", []Line{{"a", "\n"}}},
		{"a\r\nb\nc\rd", []Line{{"a", "\r\n"}, {"b", "\n"}, {"c", "\r"}, {"d", ""}}},
		{"\n\n", []Line{{"", "\n"}, {"", "\n"}}},
		{"x\r", []Line{{"x", "\r"}}},
	}
	for _, tc := range tests {
		got := SplitLines(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitLines(%q) = %+v want %+v", tc.in, got, tc.want)
		}
		d := &Document{Lines: got, Encoding: EncUTF8}
		if d.String() != tc.in {
			t.Errorf("String() round trip of %q = %q", tc.in, d.String())
		}
	}
}

func TestDefaultEOL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "\n"},
		{"a", "\n"},
		{"a\r\nb\r\nc\n", "\r\n"},
		{"a\nb\r\n", "\n"},
		{"a\rb\rc\n", "\r"},
	}
	for _, tc := range tests {
		if got := NewDocument(tc.in).DefaultEOL(); got != tc.want {
			t.Errorf("DefaultEOL(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	text := "első sor\r\nmásodik ő ű\r\nharmadik"
	cases := []struct {
		name string
		enc  Encoding
		bom  bool
	}{
		{"utf8", EncUTF8, false},
		{"utf8bom", EncUTF8, true},
		{"utf16le", EncUTF16LE, true},
		{"utf16be", EncUTF16BE, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := Encode(text, c.enc, c.bom)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, c.name+".txt")
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
			d, det, err := LoadFile(p, EncAuto, DetectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if d.Encoding != c.enc || d.BOM != c.bom || det.Binary || d.Path != p {
				t.Fatalf("loaded as %v bom=%v det=%+v", d.Encoding, d.BOM, det)
			}
			if len(d.Lines) != 3 || d.Lines[1].Text != "második ő ű" {
				t.Fatalf("lines %+v", d.Lines)
			}
			if err := d.SaveFile(p); err != nil {
				t.Fatal(err)
			}
			back, _ := os.ReadFile(p)
			if string(back) != string(data) {
				t.Fatalf("save changed bytes")
			}
			if runtime.GOOS != "windows" {
				st, _ := os.Stat(p)
				if st.Mode().Perm() != 0o600 {
					t.Fatalf("mode not preserved: %v", st.Mode())
				}
			}
		})
	}
	// Explicit encoding overrides detection.
	p := filepath.Join(dir, "latin2.txt")
	data, _ := Encode("őű", EncISO8859_2, false)
	os.WriteFile(p, data, 0o644)
	d, _, err := LoadFile(p, EncISO8859_2, DetectOptions{})
	if err != nil || d.Lines[0].Text != "őű" {
		t.Fatalf("explicit encoding: %+v %v", d, err)
	}
	if _, _, err := LoadFile(filepath.Join(dir, "missing"), EncAuto, DetectOptions{}); err == nil {
		t.Fatal("missing file must fail")
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".txt" {
			t.Errorf("stray file %s", e.Name())
		}
	}
}

func TestSaveUnencodableKeepsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("orig"), 0o644)
	d := &Document{Lines: []Line{{"ő", ""}}, Encoding: EncWindows1252}
	if err := d.SaveFile(p); err == nil {
		t.Fatal("expected encoding error")
	}
	if b, _ := os.ReadFile(p); string(b) != "orig" {
		t.Fatal("file was modified")
	}
	if err := WriteFileAtomic(dir, []byte("x")); err == nil {
		t.Fatal("writing over a directory must fail")
	}
}
