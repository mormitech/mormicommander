// Tests for the comment- and order-preserving INI reader/writer and the
// encodings it reads and writes.

package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTripUnchanged(t *testing.T) {
	cases := map[string]string{
		"empty":        "",
		"lf":           "; top comment\n[Configuration]\nShowHiddenSystem=1\n\n# another\n[Layout]\nButtonBar = 0\n",
		"crlf":         "[A]\r\nx=1\r\n; c\r\n\r\n[B]\r\ny=2\r\n",
		"no final nl":  "[A]\nx=1",
		"odd spacing":  "  [ Spaced ]  ; trailing\n  key   =   value with spaces   \nnot a kv line\n",
		"preamble":     "orphan=1\n\n[S]\nk=v\n",
		"blank only":   "\n",
		"dup sections": "[A]\nx=1\n[B]\n[a]\ny=2\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			f := ParseString(in)
			if got := f.String(); got != in {
				t.Fatalf("round trip changed text:\n got %q\nwant %q", got, in)
			}
		})
	}
}

func TestGetCaseInsensitiveFirstWins(t *testing.T) {
	f := ParseString("[Conf]\nKey=1\nkey=2\n[CONF]\nOther=3\nKEY=4\n")
	tests := []struct {
		sec, key, want string
		ok             bool
	}{
		{"conf", "KEY", "1", true},
		{"Conf", "other", "3", true},
		{"conf", "missing", "", false},
		{"nosuch", "key", "", false},
	}
	for _, tt := range tests {
		got, ok := f.Get(tt.sec, tt.key)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Get(%q,%q) = %q,%v want %q,%v", tt.sec, tt.key, got, ok, tt.want, tt.ok)
		}
	}
	if !reflect.DeepEqual(f.Keys("conf"), []string{"Key", "Other"}) {
		t.Errorf("Keys = %v", f.Keys("conf"))
	}
	if !reflect.DeepEqual(f.Sections(), []string{"Conf"}) {
		t.Errorf("Sections = %v", f.Sections())
	}
}

func TestTypedGetters(t *testing.T) {
	f := ParseString("[s]\na=12abc\nb=-3\nc=yes\nd=0\ne=x\nf=on\n")
	tests := []struct {
		key  string
		i    int
		b    bool
		defI int
		defB bool
	}{
		{"a", 12, true, 7, false},
		{"b", -3, true, 7, false},
		{"c", 7, true, 7, false},
		{"d", 0, false, 7, true},
		{"e", 7, true, 7, true},
		{"f", 7, true, 7, false},
		{"zz", 7, true, 7, true},
	}
	for _, tt := range tests {
		if got := f.GetInt("s", tt.key, tt.defI); got != tt.i {
			t.Errorf("GetInt(%s) = %d want %d", tt.key, got, tt.i)
		}
		if got := f.GetBool("s", tt.key, tt.defB); got != tt.b {
			t.Errorf("GetBool(%s) = %v want %v", tt.key, got, tt.b)
		}
	}
}

func TestSetPreservesLayout(t *testing.T) {
	in := "; header\n[A]\n; about x\nx=1\n\n; section B follows\n[B]\ny=2\n"
	tests := []struct {
		name string
		edit func(*File)
		want string
	}{
		{"change in place", func(f *File) { f.Set("a", "X", "9") },
			"; header\n[A]\n; about x\nx=9\n\n; section B follows\n[B]\ny=2\n"},
		{"same value no change", func(f *File) { f.Set("A", "x", "1") }, in},
		{"append before blank", func(f *File) { f.Set("A", "z", "3") },
			"; header\n[A]\n; about x\nx=1\nz=3\n\n; section B follows\n[B]\ny=2\n"},
		{"new section", func(f *File) { f.SetBool("C", "on", true) },
			in + "\n[C]\non=1\n"},
		{"delete", func(f *File) { f.Delete("B", "y") },
			"; header\n[A]\n; about x\nx=1\n\n; section B follows\n[B]\n"},
		{"delete section", func(f *File) { f.DeleteSection("b") },
			"; header\n[A]\n; about x\nx=1\n\n; section B follows\n"},
		{"rename section", func(f *File) { f.RenameSection("B", "Bee") },
			"; header\n[A]\n; about x\nx=1\n\n; section B follows\n[Bee]\ny=2\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ParseString(in)
			tt.edit(f)
			if got := f.String(); got != tt.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestReplaceSection(t *testing.T) {
	tests := []struct {
		name, in string
		kvs      []KV
		want     string
	}{
		{"replace keeps comments", "[H]\n; history\n0=a\n1=b\n\n[N]\nk=v\n", []KV{{"0", "c"}},
			"[H]\n; history\n0=c\n\n[N]\nk=v\n"},
		{"empty section gets keys before blank", "[H]\n\n[N]\n", []KV{{"0", "x"}, {"1", "y"}},
			"[H]\n0=x\n1=y\n\n[N]\n"},
		{"new section", "[N]\nk=v\n", []KV{{"a", "1"}},
			"[N]\nk=v\n\n[H]\na=1\n"},
		{"clear", "[H]\n0=a\n[N]\n", nil, "[H]\n[N]\n"},
		{"duplicate sections emptied", "[H]\n0=a\n[X]\n[h]\n1=b\n", []KV{{"0", "z"}},
			"[H]\n0=z\n[X]\n[h]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ParseString(tt.in)
			f.ReplaceSection("H", tt.kvs)
			if got := f.String(); got != tt.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestNewFileAndClone(t *testing.T) {
	f := NewFile()
	f.Set("A", "k", "v")
	f.SetInt("A", "n", 5)
	if got := f.String(); got != "[A]\nk=v\nn=5\n" {
		t.Fatalf("got %q", got)
	}
	c := f.Clone()
	c.Set("A", "k", "changed")
	if v, _ := f.Get("A", "k"); v != "v" {
		t.Fatal("clone shares lines with original")
	}
	m := f.Map("a")
	if m["k"] != "v" || m["n"] != "5" {
		t.Fatalf("Map = %v", m)
	}
}

func TestEncodings(t *testing.T) {
	text := "[DirMenu]\nmenu1=Árvíztűrő\n"
	for _, enc := range []Encoding{EncUTF8, EncUTF8BOM, EncUTF16LE, EncUTF16BE, EncCP1250} {
		t.Run(enc.String(), func(t *testing.T) {
			data, err := Encode(text, enc)
			if err != nil {
				t.Fatal(err)
			}
			got, gotEnc := Decode(data, EncCP1250)
			if got != text || gotEnc != enc {
				t.Fatalf("Decode = %q,%v want %q,%v", got, gotEnc, text, enc)
			}
		})
	}
	// cp1252 can't hold ű.
	if _, err := Encode("ű", EncCP1252); err != ErrUnencodable {
		t.Fatalf("want ErrUnencodable, got %v", err)
	}
	// Non-UTF-8 bytes decode with the fallback.
	if got, enc := Decode([]byte{'a', 0xE9}, EncCP1252); got != "aé" || enc != EncCP1252 {
		t.Fatalf("cp1252 fallback = %q %v", got, enc)
	}
	if got, _ := Decode([]byte{0xF5}, EncCP1250); got != "ő" {
		t.Fatalf("cp1250 fallback = %q", got)
	}
}

func TestSaveKeepsEncodingAndUpgrades(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wincmd.ini")
	orig, _ := Encode("[A]\r\nk=café\r\n", EncCP1252)
	if err := os.WriteFile(p, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Encoding != EncCP1252 || f.Newline != "\r\n" {
		t.Fatalf("detected %v %q", f.Encoding, f.Newline)
	}
	f.Set("A", "n", "1")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	want, _ := Encode("[A]\r\nk=café\r\nn=1\r\n", EncCP1252)
	if !bytes.Equal(data, want) {
		t.Fatalf("saved %q want %q", data, want)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %v", st.Mode().Perm())
	}
	// A character outside the code page switches to UTF-16LE like TC.
	f.Set("A", "h", "ő")
	if err := f.Save(p); err != nil {
		t.Fatal(err)
	}
	g, _ := LoadFile(p)
	if g.Encoding != EncUTF16LE {
		t.Fatalf("encoding after upgrade = %v", g.Encoding)
	}
	if v, _ := g.Get("A", "h"); v != "ő" {
		t.Fatalf("value = %q", v)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "x.ini")
	if err := WriteFileAtomic(p, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomicBackup(p, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "two" {
		t.Fatalf("content %q", b)
	}
	if b, _ := os.ReadFile(p + ".bak"); string(b) != "one" {
		t.Fatalf("backup %q", b)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
	if err := WriteFileAtomic(dir, []byte("x"), 0o644); err == nil {
		t.Error("writing over a folder should fail")
	}
}
