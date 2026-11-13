// Tests for XML and binary property lists, and for the macOS "Open with" logic built on them.
package launch

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"unicode/utf16"
)

// encodeBplist writes a small binary plist (one-byte object references, two-byte offsets) for tests. Map keys are
// written in sorted order so the output is stable.
func encodeBplist(v any) []byte {
	var objs [][]byte
	var add func(v any) int
	add = func(v any) int {
		idx := len(objs)
		objs = append(objs, nil)
		var b []byte
		lenMarker := func(hi byte, n int) []byte {
			if n < 15 {
				return []byte{hi<<4 | byte(n)}
			}
			return []byte{hi<<4 | 0x0F, 0x11, byte(n >> 8), byte(n)}
		}
		switch x := v.(type) {
		case bool:
			if x {
				b = []byte{0x09}
			} else {
				b = []byte{0x08}
			}
		case int:
			b = []byte{0x13, 0, 0, 0, 0, 0, 0, 0, 0}
			binary.BigEndian.PutUint64(b[1:], uint64(x))
		case string:
			ascii := true
			for _, r := range x {
				if r > 127 {
					ascii = false
				}
			}
			if ascii {
				b = append(lenMarker(0x5, len(x)), x...)
			} else {
				u := utf16.Encode([]rune(x))
				b = lenMarker(0x6, len(u))
				for _, c := range u {
					b = append(b, byte(c>>8), byte(c))
				}
			}
		case []byte:
			b = append(lenMarker(0x4, len(x)), x...)
		case []any:
			refs := make([]byte, 0, len(x))
			for _, e := range x {
				refs = append(refs, byte(add(e)))
			}
			b = append(lenMarker(0xA, len(x)), refs...)
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var kr, vr []byte
			for _, k := range keys {
				kr = append(kr, byte(add(k)))
			}
			for _, k := range keys {
				vr = append(vr, byte(add(x[k])))
			}
			b = append(append(lenMarker(0xD, len(keys)), kr...), vr...)
		}
		objs[idx] = b
		return idx
	}
	add(v)
	out := []byte("bplist00")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(out)
		out = append(out, o...)
	}
	table := len(out)
	for _, off := range offsets {
		out = append(out, byte(off>>8), byte(off))
	}
	tr := make([]byte, 32)
	tr[6], tr[7] = 2, 1
	binary.BigEndian.PutUint64(tr[8:], uint64(len(objs)))
	binary.BigEndian.PutUint64(tr[16:], 0)
	binary.BigEndian.PutUint64(tr[24:], uint64(table))
	return append(out, tr...)
}

func TestParsePlist(t *testing.T) {
	xmlText := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Name</key><string>Text &amp; more</string>
  <key>N</key><integer>42</integer>
  <key>R</key><real>1.5</real>
  <key>Yes</key><true/>
  <key>No</key><false/>
  <key>D</key><data>aGk=</data>
  <key>A</key><array><string>x</string><dict><key>k</key><string>v</string></dict></array>
</dict></plist>`
	want := map[string]any{
		"Name": "Text & more", "N": int64(42), "R": 1.5, "Yes": true, "No": false, "D": []byte("hi"),
		"A": []any{"x", map[string]any{"k": "v"}},
	}
	got, err := ParsePlist([]byte(xmlText))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("XML plist = %#v", got)
	}

	src := map[string]any{"URL": "https://example.com/ä", "Count": 7, "Flag": true, "Off": false,
		"List": []any{"a", "b"}, "Blob": []byte{1, 2}, "Long": "a string that is longer than fifteen characters"}
	got, err = ParsePlist(encodeBplist(src))
	if err != nil {
		t.Fatal(err)
	}
	wantB := map[string]any{"URL": "https://example.com/ä", "Count": int64(7), "Flag": true, "Off": false,
		"List": []any{"a", "b"}, "Blob": []byte{1, 2}, "Long": "a string that is longer than fifteen characters"}
	if !reflect.DeepEqual(got, wantB) {
		t.Errorf("binary plist = %#v", got)
	}
}

func TestParsePlistMalformed(t *testing.T) {
	good := encodeBplist(map[string]any{"a": []any{"b", "c"}, "d": "e"})
	// Every truncation and every single-byte corruption must fail cleanly or parse, never panic or hang.
	for i := 0; i < len(good); i++ {
		_, _ = ParsePlist(good[:i])
		bad := append([]byte(nil), good...)
		bad[i] ^= 0xFF
		_, _ = ParsePlist(bad)
	}
	// An array that contains itself.
	cyc := []byte("bplist00")
	cyc = append(cyc, 0xA1, 0x00) // array of one element: object 0, itself
	tr := make([]byte, 32)
	tr[6], tr[7] = 1, 1
	binary.BigEndian.PutUint64(tr[8:], 1)
	binary.BigEndian.PutUint64(tr[24:], 10)
	cyc = append(cyc, 8) // offset table: object 0 at offset 8
	cyc = append(cyc, tr...)
	if _, err := ParsePlist(cyc); err == nil {
		t.Error("a self-containing array should be rejected")
	}
	if _, err := ParsePlist([]byte("<plist><dict><key>x</key>")); err == nil {
		t.Error("truncated XML should fail")
	}
	if _, err := ParsePlist([]byte("<plist><integer>x</integer></plist>")); err == nil {
		t.Error("bad integer should fail")
	}
}

func infoPlist(id, name string, docTypes []any, exported []any) []byte {
	m := map[string]any{"CFBundleIdentifier": id, "CFBundleName": name, "CFBundleDocumentTypes": docTypes}
	if exported != nil {
		m["UTExportedTypeDeclarations"] = exported
	}
	return encodeBplist(m)
}

func TestMacApps(t *testing.T) {
	apps := t.TempDir()
	mk := func(bundle string, data []byte) {
		writeFile(t, filepath.Join(apps, bundle, "Contents", "Info.plist"), string(data), 0o644)
	}
	mk("TextEdit.app", infoPlist("com.apple.TextEdit", "TextEdit", []any{
		map[string]any{"CFBundleTypeRole": "Editor", "LSItemContentTypes": []any{"public.plain-text", "public.rtf"}},
	}, nil))
	mk("Code.app", infoPlist("com.microsoft.VSCode", "Code", []any{
		map[string]any{"CFBundleTypeRole": "Editor", "CFBundleTypeExtensions": []any{"go", "MD"}},
	}, nil))
	mk("Hex.app", infoPlist("com.ex.hex", "Hex", []any{
		map[string]any{"CFBundleTypeRole": "Viewer", "LSItemContentTypes": []any{"public.data"}},
	}, nil))
	mk("Quiet.app", infoPlist("com.ex.quiet", "Quiet", []any{
		map[string]any{"CFBundleTypeRole": "None", "LSItemContentTypes": []any{"public.plain-text"}},
	}, nil))
	mk("Pages.app", infoPlist("com.apple.iWork.Pages", "Pages", []any{
		map[string]any{"CFBundleTypeRole": "Editor", "LSItemContentTypes": []any{"com.apple.iwork.pages.sffpages"}},
	}, []any{map[string]any{
		"UTTypeIdentifier": "com.apple.iwork.pages.sffpages", "UTTypeConformsTo": []any{"public.composite-content"},
		"UTTypeTagSpecification": map[string]any{"public.filename-extension": []any{"pages"}},
	}}))
	writeFile(t, filepath.Join(apps, "Broken.app", "Contents", "Info.plist"), "not a plist", 0o644)
	writeFile(t, filepath.Join(apps, "NotAnApp", "x"), "", 0o644)

	list, extUTIs, conforms := scanMacApps([]string{apps, filepath.Join(apps, "missing")})
	if len(list) != 5 {
		t.Fatalf("scanned %d apps, want 5", len(list))
	}
	names := func(a []App) []string {
		sortApps(a)
		var n []string
		for _, x := range a {
			n = append(n, x.Name)
		}
		return n
	}
	if got := names(macAppsFor("txt", list, extUTIs, conforms, "")); !reflect.DeepEqual(got, []string{"TextEdit", "Hex"}) {
		t.Errorf("txt apps = %q", got)
	}
	// markdown conforms to plain text, and Code claims "md" by extension.
	if got := names(macAppsFor("md", list, extUTIs, conforms, "com.microsoft.VSCode")); !reflect.DeepEqual(got, []string{"Code", "TextEdit", "Hex"}) {
		t.Errorf("md apps = %q", got)
	}
	// The type Pages declares itself is learned from its own Info.plist.
	if got := names(macAppsFor("pages", list, extUTIs, conforms, "")); !reflect.DeepEqual(got, []string{"Pages"}) {
		t.Errorf("pages apps = %q", got)
	}
	if got := names(macAppsFor("unknownext", list, extUTIs, conforms, "")); !reflect.DeepEqual(got, []string{"Hex"}) {
		t.Errorf("unknown extension apps = %q", got)
	}

	ls := encodeBplist(map[string]any{"LSHandlers": []any{
		map[string]any{"LSHandlerContentType": "public.plain-text", "LSHandlerRoleAll": "com.microsoft.vscode"},
		map[string]any{"LSHandlerContentTag": "log", "LSHandlerContentTagClass": "public.filename-extension", "LSHandlerRoleViewer": "com.apple.Console"},
		map[string]any{"LSHandlerContentType": "public.data", "LSHandlerRoleAll": "com.ex.hex"},
	}})
	if got := macDefaultHandler(ls, "txt", utisForExt("txt", extUTIs, conforms)); got != "com.microsoft.vscode" {
		t.Errorf("default for txt = %q", got)
	}
	if got := macDefaultHandler(ls, "LOG", utisForExt("log", extUTIs, conforms)); got != "com.apple.Console" {
		t.Errorf("default for log = %q", got)
	}
	if got := macDefaultHandler(ls, "png", utisForExt("png", extUTIs, conforms)); got != "" {
		t.Errorf("broad types must not count as a default, got %q", got)
	}
	if got := macDefaultHandler([]byte("junk"), "txt", nil); got != "" {
		t.Errorf("junk preferences = %q", got)
	}
	if len(macAppDirs("/Users/x")) != 5 || len(macAppDirs("")) != 4 {
		t.Error("macAppDirs is wrong")
	}
}
