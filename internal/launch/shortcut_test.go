// Tests for reading .url, .webloc, .desktop and .lnk shortcut files.
package launch

import (
	"encoding/binary"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// buildLnk writes a shell link with a LinkInfo local base path and optional Unicode string data.
func buildLnk(localPath, workDir, args, relPath string) []byte {
	b := make([]byte, 0x4C)
	binary.LittleEndian.PutUint32(b, 0x4C)
	flags := uint32(lnkIsUnicode)
	if localPath != "" {
		flags |= lnkHasLinkInfo
	}
	if relPath != "" {
		flags |= lnkHasRelPath
	}
	if workDir != "" {
		flags |= lnkHasWorkingDir
	}
	if args != "" {
		flags |= lnkHasArguments
	}
	flags |= lnkHasIDList
	binary.LittleEndian.PutUint32(b[0x14:], flags)
	// An ID list we skip over: size 4, then 4 bytes.
	b = append(b, 4, 0, 1, 2, 3, 4)
	if localPath != "" {
		hdr := 0x1C
		base := append([]byte(localPath), 0)
		suffix := []byte{0}
		size := hdr + len(base) + len(suffix)
		li := make([]byte, hdr)
		binary.LittleEndian.PutUint32(li[0:], uint32(size))
		binary.LittleEndian.PutUint32(li[4:], uint32(hdr))
		binary.LittleEndian.PutUint32(li[8:], 1) // VolumeIDAndLocalBasePath
		binary.LittleEndian.PutUint32(li[16:], uint32(hdr))
		binary.LittleEndian.PutUint32(li[24:], uint32(hdr+len(base)))
		li = append(append(li, base...), suffix...)
		b = append(b, li...)
	}
	str := func(s string) {
		u := utf16.Encode([]rune(s))
		b = binary.LittleEndian.AppendUint16(b, uint16(len(u)))
		for _, c := range u {
			b = binary.LittleEndian.AppendUint16(b, c)
		}
	}
	if relPath != "" {
		str(relPath)
	}
	if workDir != "" {
		str(workDir)
	}
	if args != "" {
		str(args)
	}
	return b
}

func TestReadShortcut(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"site.url":     "[InternetShortcut]\r\nURL=https://example.com/\r\n",
		"local.url":    "[InternetShortcut]\nURL=file:///C:/My%20Docs/a.txt\n",
		"page.webloc":  `<?xml version="1.0"?><plist version="1.0"><dict><key>URL</key><string>https://ex.org/x</string></dict></plist>`,
		"link.desktop": "[Desktop Entry]\nType=Link\nURL=file:///home/u/My%20Dir\n",
		"web.desktop":  "[Desktop Entry]\nType=Link\nURL=https://ex.net\n",
		"app.desktop":  "[Desktop Entry]\nType=Application\nExec=gimp --new \"a b\" %U\n",
		"dir.desktop":  "[Desktop Entry]\nType=Directory\n",
		"empty.url":    "[InternetShortcut]\n",
		"bin.webloc":   string(encodeBplist(map[string]any{"URL": "https://bin.example/"})),
		"tool.lnk":     string(buildLnk(`C:\Tools\tool.exe`, `C:\Work`, `-v "x y"`, "")),
		"doc.lnk":      string(buildLnk(`C:\Docs\a.txt`, "", "", "")),
		"rel.lnk":      string(buildLnk("", "", "", `..\target.txt`)),
		"bad.lnk":      "not a link at all, too short",
	}
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content, 0o644)
	}
	tests := []struct {
		name string
		want Shortcut
		err  bool
	}{
		{"site.url", Shortcut{Kind: ShortcutURL, Target: "https://example.com/"}, false},
		{"local.url", Shortcut{Kind: ShortcutPath, Target: `C:\My Docs\a.txt`}, false},
		{"page.webloc", Shortcut{Kind: ShortcutURL, Target: "https://ex.org/x"}, false},
		{"bin.webloc", Shortcut{Kind: ShortcutURL, Target: "https://bin.example/"}, false},
		{"link.desktop", Shortcut{Kind: ShortcutPath, Target: "/home/u/My Dir"}, false},
		{"web.desktop", Shortcut{Kind: ShortcutURL, Target: "https://ex.net"}, false},
		{"app.desktop", Shortcut{Kind: ShortcutCommand, Target: "gimp", Args: `--new a\ b`}, false},
		{"tool.lnk", Shortcut{Kind: ShortcutCommand, Target: `C:\Tools\tool.exe`, Args: `-v "x y"`, Dir: `C:\Work`}, false},
		{"doc.lnk", Shortcut{Kind: ShortcutPath, Target: `C:\Docs\a.txt`}, false},
		{"rel.lnk", Shortcut{Kind: ShortcutPath, Target: filepath.Join(filepath.Dir(dir), "target.txt")}, false},
		{"dir.desktop", Shortcut{}, true},
		{"empty.url", Shortcut{}, true},
		{"bad.lnk", Shortcut{}, true},
		{"missing.url", Shortcut{}, true},
	}
	for _, tt := range tests {
		got, err := ReadShortcut(filepath.Join(dir, tt.name))
		if (err != nil) != tt.err {
			t.Errorf("%s: error %v, want error %v", tt.name, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.name, got, tt.want)
		}
	}
	if !IsShortcutName("A.LNK") || IsShortcutName("a.txt") {
		t.Error("IsShortcutName is wrong")
	}
	writeFile(t, filepath.Join(dir, "x.txt"), "", 0o644)
	if _, err := ReadShortcut(filepath.Join(dir, "x.txt")); err == nil {
		t.Error("a .txt is no shortcut")
	}
	// Corrupted links never panic.
	good := buildLnk(`C:\a\b.exe`, `C:\w`, "-x", "")
	for i := 0; i < len(good); i++ {
		_, _ = ParseLnk(good[:i], dir)
		bad := append([]byte(nil), good...)
		bad[i] ^= 0xFF
		_, _ = ParseLnk(bad, dir)
	}
}

func TestLinkInfoNetworkPath(t *testing.T) {
	// LinkInfo with only CommonNetworkRelativeLink: \\server\share + suffix.
	hdr := 0x1C
	cnr := make([]byte, 0x14)
	net := append([]byte(`\\srv\share`), 0)
	binary.LittleEndian.PutUint32(cnr[0:], uint32(0x14+len(net)))
	binary.LittleEndian.PutUint32(cnr[8:], 0x14)
	cnr = append(cnr, net...)
	suffix := append([]byte(`dir\f.txt`), 0)
	li := make([]byte, hdr)
	binary.LittleEndian.PutUint32(li[4:], uint32(hdr))
	binary.LittleEndian.PutUint32(li[8:], 2)
	binary.LittleEndian.PutUint32(li[20:], uint32(hdr))
	binary.LittleEndian.PutUint32(li[24:], uint32(hdr+len(cnr)))
	li = append(append(li, cnr...), suffix...)
	binary.LittleEndian.PutUint32(li[0:], uint32(len(li)))
	if got := linkInfoPath(li); got != `\\srv\share\dir\f.txt` {
		t.Errorf("network path = %q", got)
	}
}
