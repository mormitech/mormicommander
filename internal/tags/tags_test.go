// Tests of the tag codecs and of reading and writing tags on temporary files (skipped where the temporary folder's
// file system has no extended attributes).
package tags

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"mormicommander/internal/xattr"
)

func TestPlistRoundTrip(t *testing.T) {
	long := strings.Repeat("x", 300)
	many := make([]string, 300)
	for i := range many {
		many[i] = "t" + strings.Repeat("y", i%20)
	}
	for _, list := range [][]string{{}, {"Red\n6"}, {"Work", "Árvíztűrő\n2", "日本"}, {long}, many} {
		got, err := decodeStrings(encodeStrings(list))
		if err != nil || !reflect.DeepEqual(got, list) {
			t.Fatalf("round trip of %d strings: %v, %v", len(list), err, len(got))
		}
	}
}

func TestPlistBroken(t *testing.T) {
	good := encodeStrings([]string{"Red\n6", "Blue\n4"})
	for i := 0; i < len(good); i++ {
		if _, err := decodeStrings(good[:i]); err == nil && i < len(good) {
			// a cut-off list must never read as a valid one (nor panic)
			t.Fatalf("cut at %d read as valid", i)
		}
	}
	for i := range good {
		b := append([]byte(nil), good...)
		b[i] ^= 0xFF
		_, _ = decodeStrings(b) // must not panic
	}
}

// a list as Finder writes it (made by macOS for a file with the tags "Red" and "Projekt")
func TestParseFinder(t *testing.T) {
	got := parseFinder(encodeStrings([]string{"Red\n6", "Projekt\n0", "Plain", "\n3"}))
	want := []Tag{{"Red", 6}, {"Projekt", 0}, {"Plain", 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFinder = %+v", got)
	}
	if parseFinder([]byte("garbage")) != nil {
		t.Fatal("garbage should read as no tags")
	}
}

func TestParseXDG(t *testing.T) {
	got := parseXDG([]byte(" work, home,,Work ,x\x00"))
	if !reflect.DeepEqual(got, []Tag{{Name: "work"}, {Name: "home"}, {Name: "x"}}) {
		t.Fatalf("parseXDG = %+v", got)
	}
}

func TestChangeAndClean(t *testing.T) {
	list := []Tag{{"Red", 6}, {"Work", 0}}
	out, ch := Change(list, []Tag{{"work", 0}, {"Blue", 4}}, []string{"RED"})
	if !ch || !reflect.DeepEqual(out, []Tag{{"Work", 0}, {"Blue", 4}}) {
		t.Fatalf("Change = %+v %v", out, ch)
	}
	if _, ch := Change(list, []Tag{{"Red", 0}}, []string{"none"}); ch {
		t.Fatal("nothing should change")
	}
	if !reflect.DeepEqual(list, []Tag{{"Red", 6}, {"Work", 0}}) {
		t.Fatal("Change changed its input")
	}
	if Clean(XDG, " a,b\nc ") != "a b c" || Clean(Finder, " a,b\nc ") != "a,b c" || Clean(Finder, "  ") != "" {
		t.Fatal("Clean")
	}
	if DefaultColor("red") != 6 || DefaultColor("Gray") != 1 || DefaultColor("Work") != 0 {
		t.Fatal("DefaultColor")
	}
}

func tempFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !xattr.Supported() {
		t.Skip("no extended attributes on this system")
	}
	if err := xattr.Set(p, "user.mc-probe", []byte("1")); err != nil {
		t.Skip("the temporary folder's file system has no extended attributes")
	}
	xattr.Remove(p, "user.mc-probe")
	return p
}

func TestWriteReadFinder(t *testing.T) {
	p := tempFile(t)
	list := []Tag{{"Red", 6}, {"Projekt", 0}, {"Blue", 4}}
	if err := WriteStyle(Finder, p, list); err != nil {
		if errors.Is(err, syscall.ENOTSUP) {
			t.Skip("this file system has no Finder attributes (Linux allows only user.* names)")
		}
		t.Fatal(err)
	}
	got, err := ReadStyle(Finder, p)
	if err != nil || !reflect.DeepEqual(got, list) {
		t.Fatalf("read back %+v, %v", got, err)
	}
	fi, ok, _ := xattr.Get(p, FinderInfo)
	if !ok || len(fi) != 32 || int(fi[9]>>1)&7 != 4 {
		t.Fatalf("FinderInfo label = %v %v", fi, ok)
	}
	if err := WriteStyle(Finder, p, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadStyle(Finder, p); len(got) != 0 {
		t.Fatalf("after removing: %+v", got)
	}
	if xattr.Has(p, FinderAttr) || xattr.Has(p, FinderInfo) {
		t.Fatal("empty tags should leave no attributes")
	}
}

func TestFinderInfoKeepsOtherBytes(t *testing.T) {
	p := tempFile(t)
	fi := make([]byte, 32)
	fi[0], fi[9] = 'T', 0x01|6<<1 // a type code and the red label (plus another flag bit)
	if err := xattr.Set(p, FinderInfo, fi); err != nil {
		t.Skip("FinderInfo can't be set here:", err)
	}
	if got, _ := ReadStyle(Finder, p); !reflect.DeepEqual(got, []Tag{{"Red", 6}}) {
		t.Fatalf("label read as %+v", got)
	}
	if err := WriteStyle(Finder, p, []Tag{{"Work", 0}}); err != nil {
		t.Fatal(err)
	}
	fi2, ok, _ := xattr.Get(p, FinderInfo)
	if !ok || fi2[0] != 'T' || fi2[9] != 0x01 {
		t.Fatalf("FinderInfo = %v", fi2)
	}
}

func TestWriteReadXDG(t *testing.T) {
	p := tempFile(t)
	if err := WriteStyle(XDG, p, []Tag{{Name: "work"}, {Name: "home"}}); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := xattr.Get(p, XDGAttr); string(v) != "work,home" {
		t.Fatalf("value %q", v)
	}
	got, _ := ReadStyle(XDG, p)
	if !reflect.DeepEqual(got, []Tag{{Name: "work"}, {Name: "home"}}) {
		t.Fatalf("read %+v", got)
	}
	if err := WriteStyle(XDG, p, nil); err != nil || xattr.Has(p, XDGAttr) {
		t.Fatalf("remove: %v", err)
	}
}
