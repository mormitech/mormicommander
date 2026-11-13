// Tests for the .lnk reader. There is no .lnk writer in the package, so the
// tests build MS-SHLLINK files byte by byte with a small helper that follows
// the specification: local and network targets, Unicode and ANSI strings, an
// environment-variable block, and every truncation of a valid file (which
// must fail cleanly, never panic).

package links

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"
)

type lnkSpec struct {
	localBase  string
	suffix     string
	netName    string
	unicodeLI  bool
	name, rel  string
	workDir    string
	args, icon string
	ansi       bool
	envTarget  string
	idList     bool
	attrs      uint32
}

func u16z(s string) []byte {
	var b bytes.Buffer
	for _, c := range utf16.Encode([]rune(s)) {
		binary.Write(&b, binary.LittleEndian, c)
	}
	b.Write([]byte{0, 0})
	return b.Bytes()
}

func buildLnk(s lnkSpec) []byte {
	le := binary.LittleEndian
	var flags uint32
	if s.idList {
		flags |= lnkHasIDList
	}
	if s.localBase != "" || s.netName != "" {
		flags |= lnkHasLinkInfo
	}
	strs := []struct {
		bit uint32
		v   string
	}{{lnkHasName, s.name}, {lnkHasRelPath, s.rel}, {lnkHasWorkingDir, s.workDir}, {lnkHasArguments, s.args}, {lnkHasIcon, s.icon}}
	for _, x := range strs {
		if x.v != "" {
			flags |= x.bit
		}
	}
	if !s.ansi {
		flags |= lnkIsUnicode
	}
	if s.envTarget != "" {
		flags |= lnkHasExpString
	}
	hdr := make([]byte, 76)
	le.PutUint32(hdr[0:], 0x4C)
	copy(hdr[4:], lnkCLSID)
	le.PutUint32(hdr[20:], flags)
	le.PutUint32(hdr[24:], s.attrs)
	le.PutUint64(hdr[44:], 116444736000000000+uint64(1_600_000_000)*10_000_000)
	le.PutUint32(hdr[52:], 1234)
	le.PutUint32(hdr[60:], 1)
	var out bytes.Buffer
	out.Write(hdr)
	if s.idList {
		out.Write([]byte{4, 0, 0xAA, 0xBB, 0xCC, 0xDD})
	}
	if flags&lnkHasLinkInfo != 0 {
		hsize := 0x1C
		if s.unicodeLI {
			hsize = 0x24
		}
		var body bytes.Buffer
		offs := map[string]int{}
		add := func(key string, b []byte) {
			offs[key] = hsize + body.Len()
			body.Write(b)
		}
		var liFlags uint32
		if s.localBase != "" {
			liFlags |= 1
			add("vol", []byte{0x10, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0x10, 0, 0, 0}) // VolumeID, empty label
			add("base", append([]byte(s.localBase), 0))
			if s.unicodeLI {
				add("baseU", u16z(s.localBase))
			}
		} else {
			liFlags |= 2
			cn := make([]byte, 0x14)
			le.PutUint32(cn[0:], uint32(0x14+len(s.netName)+1))
			le.PutUint32(cn[8:], 0x14)
			cn = append(cn, append([]byte(s.netName), 0)...)
			add("net", cn)
		}
		add("suffix", append([]byte(s.suffix), 0))
		if s.unicodeLI {
			add("suffixU", u16z(s.suffix))
		}
		li := make([]byte, hsize)
		le.PutUint32(li[0:], uint32(hsize+body.Len()))
		le.PutUint32(li[4:], uint32(hsize))
		le.PutUint32(li[8:], liFlags)
		le.PutUint32(li[0x0C:], uint32(offs["vol"]))
		le.PutUint32(li[0x10:], uint32(offs["base"]))
		le.PutUint32(li[0x14:], uint32(offs["net"]))
		le.PutUint32(li[0x18:], uint32(offs["suffix"]))
		if s.unicodeLI {
			le.PutUint32(li[0x1C:], uint32(offs["baseU"]))
			le.PutUint32(li[0x20:], uint32(offs["suffixU"]))
		}
		out.Write(li)
		out.Write(body.Bytes())
	}
	for _, x := range strs {
		if x.v == "" {
			continue
		}
		if s.ansi {
			binary.Write(&out, le, uint16(len(x.v)))
			out.WriteString(x.v)
		} else {
			u := utf16.Encode([]rune(x.v))
			binary.Write(&out, le, uint16(len(u)))
			for _, c := range u {
				binary.Write(&out, le, c)
			}
		}
	}
	if s.envTarget != "" {
		blk := make([]byte, 0x314)
		le.PutUint32(blk[0:], 0x314)
		le.PutUint32(blk[4:], lnkEnvBlockSig)
		copy(blk[8:], s.envTarget)
		copy(blk[8+260:], u16z(s.envTarget))
		out.Write(blk)
	}
	out.Write([]byte{0, 0, 0, 0}) // terminal block
	return out.Bytes()
}

func TestParseLnk(t *testing.T) {
	tests := []struct {
		name string
		spec lnkSpec
		want Lnk
	}{
		{
			name: "local unicode",
			spec: lnkSpec{localBase: `C:\Program Files\App`, suffix: "", unicodeLI: true, name: "My app ✓", workDir: `C:\Work`,
				args: "--fast", icon: `C:\icons\a.ico`, idList: true, attrs: 0x20},
			want: Lnk{Target: `C:\Program Files\App`, Name: "My app ✓", WorkingDir: `C:\Work`, Arguments: "--fast", IconLocation: `C:\icons\a.ico`},
		},
		{
			name: "local with suffix ansi",
			spec: lnkSpec{localBase: `D:\`, suffix: `Docs\report.doc`, ansi: true, rel: `..\Docs\report.doc`},
			want: Lnk{Target: `D:\Docs\report.doc`, RelativePath: `..\Docs\report.doc`},
		},
		{
			name: "network share",
			spec: lnkSpec{netName: `\\server\share`, suffix: `folder\file.txt`, attrs: 0x10},
			want: Lnk{Target: `\\server\share\folder\file.txt`},
		},
		{
			name: "environment block only",
			spec: lnkSpec{envTarget: `%USERPROFILE%\Desktop`},
			want: Lnk{Target: `%USERPROFILE%\Desktop`},
		},
		{
			name: "relative only",
			spec: lnkSpec{rel: `.\sub\x.txt`},
			want: Lnk{RelativePath: `.\sub\x.txt`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := buildLnk(tc.spec)
			if !IsLnkData(data) {
				t.Fatal("IsLnkData = false")
			}
			l, err := ParseLnk(data)
			if err != nil {
				t.Fatal(err)
			}
			if l.Target != tc.want.Target || l.Name != tc.want.Name || l.WorkingDir != tc.want.WorkingDir ||
				l.Arguments != tc.want.Arguments || l.IconLocation != tc.want.IconLocation || l.RelativePath != tc.want.RelativePath {
				t.Errorf("got %+v\nwant %+v", *l, tc.want)
			}
			if l.FileSize != 1234 || l.ShowCommand != 1 || !l.WriteTime.Equal(time.Unix(1_600_000_000, 0)) {
				t.Errorf("header fields: %+v", *l)
			}
			if l.IsDir() != (tc.spec.attrs&0x10 != 0) {
				t.Error("IsDir")
			}
		})
	}
}

func TestParseLnkMalformed(t *testing.T) {
	data := buildLnk(lnkSpec{localBase: `C:\x`, suffix: "y", unicodeLI: true, name: "n", args: "a", envTarget: `%A%`})
	for n := 0; n < len(data)-4; n++ {
		// Every truncation must return cleanly (an error or a partial result).
		_, _ = ParseLnk(data[:n])
	}
	if _, err := ParseLnk([]byte("not a shortcut at all, really not")); !errors.Is(err, ErrBadShortcut) {
		t.Errorf("garbage: %v", err)
	}
	if _, err := ParseLnk(data[:60]); !errors.Is(err, ErrBadShortcut) {
		t.Errorf("short header: %v", err)
	}
	bad := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(bad[76:], 0xFFFFFF) // LinkInfo size past the end
	if _, err := ParseLnk(bad); !errors.Is(err, ErrBadShortcut) {
		t.Errorf("bad LinkInfo size: %v", err)
	}
}

func TestReadLnkFile(t *testing.T) {
	dir := realTemp(t)
	p := filepath.Join(dir, "x.lnk")
	writeFile(t, p, string(buildLnk(lnkSpec{localBase: `C:\x`})))
	l, err := ReadLnk(p)
	if err != nil || l.Target != `C:\x` {
		t.Errorf("ReadLnk = %+v, %v", l, err)
	}
	writeFile(t, p, "junk")
	if _, err := ReadLnk(p); !errors.Is(err, ErrBadShortcut) {
		t.Errorf("junk file: %v", err)
	}
	if _, err := ReadLnk(filepath.Join(dir, "missing.lnk")); err == nil {
		t.Error("missing file read")
	}
}

func TestExpandWindowsEnv(t *testing.T) {
	env := map[string]string{"USERPROFILE": `C:\Users\me`, "X": "1"}
	look := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	tests := map[string]string{
		`%USERPROFILE%\Desktop`: `C:\Users\me\Desktop`,
		`%X%%X%`:                "11",
		`%UNKNOWN%\a`:           `%UNKNOWN%\a`,
		`100%`:                  "100%",
		`a%%b`:                  "a%%b",
		`plain`:                 "plain",
	}
	for in, want := range tests {
		if got := ExpandWindowsEnv(in, look); got != want {
			t.Errorf("ExpandWindowsEnv(%q) = %q, want %q", in, got, want)
		}
	}
}
