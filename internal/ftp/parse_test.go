// This file tests the listing parsers line by line with real-world samples
// of every supported dialect (UNIX ls in its many variants, DOS/IIS, MLSD,
// VMS, EPLF, OS/400) and with lines that must be rejected, plus the
// whole-listing behaviour of ParseList: noise lines, "." and "..", VMS
// continuation lines and the bare-name fallback.
package ftp

import (
	"io/fs"
	"testing"
	"time"
)

var parseNow = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

func popt() ParseOptions { return ParseOptions{Location: time.UTC, Now: parseNow} }

func date(y int, mo time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, mo, d, h, mi, s, 0, time.UTC)
}

func TestParseLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		ok     bool
		want   Entry
		noTime bool
	}{
		// UNIX ls
		{"unix file with time", "-rw-r--r--   1 owner    group        1234 Mar  1 10:30 file.txt", true,
			Entry{Name: "file.txt", Type: TypeFile, Size: 1234, ModTime: date(2024, 3, 1, 10, 30, 0), Mode: 0o644, Owner: "owner", Group: "group", Format: FormatUnix}, false},
		{"unix dir with year", "drwxr-xr-x   2 ftp ftp 4096 Dec 31  2019 pub", true,
			Entry{Name: "pub", Type: TypeDir, Size: 4096, ModTime: date(2019, 12, 31, 0, 0, 0), Mode: fs.ModeDir | 0o755, Owner: "ftp", Group: "ftp", Format: FormatUnix}, false},
		{"unix date in the future goes to last year", "-rw-r--r-- 1 u g 1 Dec 24 18:00 xmas", true,
			Entry{Name: "xmas", Type: TypeFile, Size: 1, ModTime: date(2023, 12, 24, 18, 0, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},
		{"unix name with spaces", "-rw-r--r-- 1 u g 7 Jan  2  2020 my  file  name.doc", true,
			Entry{Name: "my  file  name.doc", Type: TypeFile, Size: 7, ModTime: date(2020, 1, 2, 0, 0, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},
		{"unix symlink", "lrwxrwxrwx 1 root root 11 Feb  3 04:05 latest -> release-1.2", true,
			Entry{Name: "latest", Type: TypeLink, Size: 11, ModTime: date(2024, 2, 3, 4, 5, 0), Mode: fs.ModeSymlink | 0o777, Owner: "root", Group: "root", Target: "release-1.2", Format: FormatUnix}, false},
		{"unix without group column", "-rw-r--r-- 1 owner 99 Jan  1  2021 nogroup", true,
			Entry{Name: "nogroup", Type: TypeFile, Size: 99, ModTime: date(2021, 1, 1, 0, 0, 0), Mode: 0o644, Owner: "owner", Format: FormatUnix}, false},
		{"unix without link count", "-rw-r--r-- owner group 5 Jan  1  2021 nolinks", true,
			Entry{Name: "nolinks", Type: TypeFile, Size: 5, ModTime: date(2021, 1, 1, 0, 0, 0), Mode: 0o644, Owner: "owner", Group: "group", Format: FormatUnix}, false},
		{"unix numeric owner", "-rw-r--r-- 1 1000 1000 5 Jan  1  2021 num", true,
			Entry{Name: "num", Type: TypeFile, Size: 5, ModTime: date(2021, 1, 1, 0, 0, 0), Mode: 0o644, Owner: "1000", Group: "1000", Format: FormatUnix}, false},
		{"unix setuid sticky and ACL marker", "drwsr-sr-t+ 3 a b 0 Jan  1  2021 special", true,
			Entry{Name: "special", Type: TypeDir, Size: 0, ModTime: date(2021, 1, 1, 0, 0, 0), Mode: fs.ModeDir | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | 0o755, Owner: "a", Group: "b", Format: FormatUnix}, false},
		{"unix device", "crw-rw---- 1 root tty 4, 64 Mar  2 09:00 ttyS0", true,
			Entry{Name: "ttyS0", Type: TypeOther, Size: 0, ModTime: date(2024, 3, 2, 9, 0, 0), Mode: 0o660, Owner: "root", Group: "tty", Format: FormatUnix}, false},
		{"unix iso time style", "-rw-r--r-- 1 u g 42 2023-07-08 09:10 iso.txt", true,
			Entry{Name: "iso.txt", Type: TypeFile, Size: 42, ModTime: date(2023, 7, 8, 9, 10, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},
		{"unix day before month", "-rw-r--r-- 1 u g 42 8 Jul 2023 dm.txt", true,
			Entry{Name: "dm.txt", Type: TypeFile, Size: 42, ModTime: date(2023, 7, 8, 0, 0, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},
		{"unix german month", "-rw-r--r-- 1 u g 3 Okt  5  2022 de.txt", true,
			Entry{Name: "de.txt", Type: TypeFile, Size: 3, ModTime: date(2022, 10, 5, 0, 0, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},
		{"unix owner named like a month", "-rw-r--r-- 1 may dec 3 Jan  5  2022 m", true,
			Entry{Name: "m", Type: TypeFile, Size: 3, ModTime: date(2022, 1, 5, 0, 0, 0), Mode: 0o644, Owner: "may", Group: "dec", Format: FormatUnix}, false},
		{"unix size looks like a day", "-rw-r--r-- 1 u g 5 Jan  1  2020 tiny", true,
			Entry{Name: "tiny", Type: TypeFile, Size: 5, ModTime: date(2020, 1, 1, 0, 0, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},
		{"unix big file", "-rw-r--r-- 1 u g 123456789012 Jan  1  2020 huge.iso", true,
			Entry{Name: "huge.iso", Type: TypeFile, Size: 123456789012, ModTime: date(2020, 1, 1, 0, 0, 0), Mode: 0o644, Owner: "u", Group: "g", Format: FormatUnix}, false},

		// DOS / IIS
		{"dos dir 12h", "01-02-06  03:04PM       <DIR>          Program Files", true,
			Entry{Name: "Program Files", Type: TypeDir, Size: -1, ModTime: date(2006, 1, 2, 15, 4, 0), Mode: fs.ModeDir, Format: FormatDOS}, false},
		{"dos file 24h 4-digit year", "12-31-2020  23:59             1,234 report final.xlsx", true,
			Entry{Name: "report final.xlsx", Type: TypeFile, Size: 1234, ModTime: date(2020, 12, 31, 23, 59, 0), Format: FormatDOS}, false},
		{"dos midnight AM", "07-04-21  12:15AM                10 a.txt", true,
			Entry{Name: "a.txt", Type: TypeFile, Size: 10, ModTime: date(2021, 7, 4, 0, 15, 0), Format: FormatDOS}, false},
		{"dos separate PM token", "07-04-21  01:15 PM               10 b.txt", true,
			Entry{Name: "b.txt", Type: TypeFile, Size: 10, ModTime: date(2021, 7, 4, 13, 15, 0), Format: FormatDOS}, false},
		{"dos iso date", "2021-07-04  13:15       <DIR>          iso dir", true,
			Entry{Name: "iso dir", Type: TypeDir, Size: -1, ModTime: date(2021, 7, 4, 13, 15, 0), Mode: fs.ModeDir, Format: FormatDOS}, false},
		{"dos european date", "04.07.2021  13:15               99 eu.txt", true,
			Entry{Name: "eu.txt", Type: TypeFile, Size: 99, ModTime: date(2021, 7, 4, 13, 15, 0), Format: FormatDOS}, false},
		{"dos junction", "07-04-21  01:15PM    <JUNCTION>     Docs [C:\\Users\\Docs]", true,
			Entry{Name: "Docs", Type: TypeLink, Size: -1, ModTime: date(2021, 7, 4, 13, 15, 0), Mode: fs.ModeSymlink, Target: `C:\Users\Docs`, Format: FormatDOS}, false},

		// MLSD
		{"mlsd file", "type=file;size=1024;modify=20230102030405;perm=adfrw;unix.mode=0640;UNIX.owner=peter;UNIX.group=staff; a file.txt", true,
			Entry{Name: "a file.txt", Type: TypeFile, Size: 1024, ModTime: date(2023, 1, 2, 3, 4, 5), Mode: 0o640, Owner: "peter", Group: "staff", Perm: "adfrw", Format: FormatMLSD}, false},
		{"mlsd dir with fraction", "Type=dir;Modify=20230102030405.123;Perm=el; folder", true,
			Entry{Name: "folder", Type: TypeDir, Size: -1, ModTime: date(2023, 1, 2, 3, 4, 5).Add(123 * time.Millisecond), Mode: fs.ModeDir, Perm: "el", Format: FormatMLSD}, false},
		{"mlsd symlink with target", "type=OS.unix=slink:/real/target;modify=20230102030405; link", true,
			Entry{Name: "link", Type: TypeLink, Size: -1, ModTime: date(2023, 1, 2, 3, 4, 5), Mode: fs.ModeSymlink, Target: "/real/target", Format: FormatMLSD}, false},
		{"mlsd cdir", "type=cdir;modify=20230102030405; /home/x", true,
			Entry{Name: ".", Type: TypeDir, Size: -1, ModTime: date(2023, 1, 2, 3, 4, 5), Mode: fs.ModeDir, Format: FormatMLSD}, false},
		{"mlst indented line", " type=file;size=3; /path/x", true,
			Entry{Name: "/path/x", Type: TypeFile, Size: 3, Format: FormatMLSD}, true},

		// VMS
		{"vms file", "LOGIN.COM;3          2/3      12-JAN-2020 10:15:22  [GROUP,SMITH]  (RWED,RWED,RE,)", true,
			Entry{Name: "LOGIN.COM", Type: TypeFile, Size: 1024, ModTime: date(2020, 1, 12, 10, 15, 22), Owner: "SMITH", Group: "GROUP", Perm: "(RWED,RWED,RE,)", Format: FormatVMS}, false},
		{"vms dir", "PROJECTS.DIR;1  1  2-FEB-2021 08:00 [SMITH] (RWE,RWE,RE,E)", true,
			Entry{Name: "PROJECTS", Type: TypeDir, Size: 512, ModTime: date(2021, 2, 2, 8, 0, 0), Mode: fs.ModeDir, Owner: "SMITH", Perm: "(RWE,RWE,RE,E)", Format: FormatVMS}, false},

		// EPLF
		{"eplf file", "+i8388621.48594,m825718503,r,s280,\tdjb.html", true,
			Entry{Name: "djb.html", Type: TypeFile, Size: 280, ModTime: time.Unix(825718503, 0).UTC(), Format: FormatEPLF}, false},
		{"eplf dir with mode", "+i8388621.29609,m824255902,/,up755,\tdev", true,
			Entry{Name: "dev", Type: TypeDir, Size: -1, ModTime: time.Unix(824255902, 0).UTC(), Mode: fs.ModeDir | 0o755, Format: FormatEPLF}, false},

		// OS/400
		{"os400 dir", "QSYS            77824 02/23/00 15:09:55 *DIR       QOpenSys/", true,
			Entry{Name: "QOpenSys", Type: TypeDir, Size: 77824, ModTime: date(2000, 2, 23, 15, 9, 55), Mode: fs.ModeDir, Owner: "QSYS", Perm: "*DIR", Format: FormatOS400}, false},
		{"os400 file", "USER1            1234 12/31/19 23:59:59 *STMF      data file.csv", true,
			Entry{Name: "data file.csv", Type: TypeFile, Size: 1234, ModTime: date(2019, 12, 31, 23, 59, 59), Owner: "USER1", Perm: "*STMF", Format: FormatOS400}, false},

		// Rejected
		{"garbage", "hello world", false, Entry{}, false},
		{"empty", "   ", false, Entry{}, false},
		{"unix without name", "-rw-r--r-- 1 u g 5 Jan  1  2020", false, Entry{}, false},
		{"bad perms", "-rw-r--r-X 1 u g 5 Jan  1  2020 x", false, Entry{}, false},
		{"bad dos date", "13-45-20  10:00AM 5 x", false, Entry{}, false},
		{"vms name alone", "LOGIN.COM;3", false, Entry{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseLine(tt.line, popt())
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (entry %+v)", ok, tt.ok, got)
			}
			if !ok {
				return
			}
			w := tt.want
			if got.Name != w.Name || got.Type != w.Type || got.Size != w.Size || got.Format != w.Format {
				t.Errorf("got name=%q type=%v size=%d format=%v; want name=%q type=%v size=%d format=%v",
					got.Name, got.Type, got.Size, got.Format, w.Name, w.Type, w.Size, w.Format)
			}
			if !tt.noTime && !got.ModTime.Equal(w.ModTime) {
				t.Errorf("time = %v, want %v", got.ModTime, w.ModTime)
			}
			if got.Mode != w.Mode {
				t.Errorf("mode = %v, want %v", got.Mode, w.Mode)
			}
			if got.Owner != w.Owner || got.Group != w.Group || got.Target != w.Target {
				t.Errorf("owner/group/target = %q/%q/%q, want %q/%q/%q", got.Owner, got.Group, got.Target, w.Owner, w.Group, w.Target)
			}
			if w.Perm != "" && got.Perm != w.Perm {
				t.Errorf("perm = %q, want %q", got.Perm, w.Perm)
			}
			if got.Raw != tt.line {
				t.Errorf("raw = %q", got.Raw)
			}
		})
	}
}

func TestParseListBehaviour(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		names   []string
		bad     int
		formats []ListFormat
	}{
		{"unix with total and dots", []string{
			"total 8",
			"drwxr-xr-x 2 u g 4096 Jan  1  2020 .",
			"drwxr-xr-x 2 u g 4096 Jan  1  2020 ..",
			"-rw-r--r-- 1 u g 1 Jan  1  2020 a",
		}, []string{"a"}, 0, []ListFormat{FormatUnix}},
		{"mlsd with cdir and pdir", []string{
			"type=cdir;modify=20200101000000; .",
			"type=pdir;modify=20200101000000; ..",
			"type=file;size=1; x",
		}, []string{"x"}, 0, []ListFormat{FormatMLSD}},
		{"unknown lines are reported", []string{
			"-rw-r--r-- 1 u g 1 Jan  1  2020 a",
			"this is not a listing line",
		}, []string{"a"}, 1, []ListFormat{FormatUnix}},
		{"names only fallback", []string{"alpha", "beta gamma"}, []string{"alpha", "beta gamma"}, 0, []ListFormat{FormatNameOnly, FormatNameOnly}},
		{"vms continuation", []string{
			"Directory DISK$X:[Y]",
			"",
			"VERY_LONG_NAME_THAT_WRAPS.TXT;12",
			"                    3/4   1-JAN-2020 00:00:00 [Y] (RWED,,,)",
			"Total of 1 file, 3/4 blocks.",
		}, []string{"VERY_LONG_NAME_THAT_WRAPS.TXT"}, 0, []ListFormat{FormatVMS}},
		{"mixed dialects", []string{
			"-rw-r--r-- 1 u g 1 Jan  1  2020 unixfile",
			"01-02-06  03:04PM       <DIR>          dosdir",
		}, []string{"unixfile", "dosdir"}, 0, []ListFormat{FormatUnix, FormatDOS}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es, bad := ParseList(tt.lines, popt())
			if len(es) != len(tt.names) {
				t.Fatalf("got %d entries %+v, want %v", len(es), es, tt.names)
			}
			for i, e := range es {
				if e.Name != tt.names[i] || e.Format != tt.formats[i] {
					t.Errorf("entry %d = %q/%v, want %q/%v", i, e.Name, e.Format, tt.names[i], tt.formats[i])
				}
			}
			if len(bad) != tt.bad {
				t.Errorf("bad = %q, want %d lines", bad, tt.bad)
			}
		})
	}
}

func TestParseOffsetAndLocation(t *testing.T) {
	berlin := time.FixedZone("CET", 3600)
	opt := ParseOptions{Location: berlin, Offset: -30 * time.Minute, Now: parseNow}
	e, ok := ParseLine("-rw-r--r-- 1 u g 1 Jan  1  2020 a", opt)
	if !ok {
		t.Fatal("not parsed")
	}
	want := time.Date(2020, 1, 1, 0, 0, 0, 0, berlin).Add(-30 * time.Minute)
	if !e.ModTime.Equal(want) {
		t.Fatalf("time = %v, want %v", e.ModTime, want)
	}
	// MLSD is UTC and ignores the offset.
	e, _ = ParseLine("type=file;modify=20200101000000; m", opt)
	if !e.ModTime.Equal(date(2020, 1, 1, 0, 0, 0)) {
		t.Fatalf("MLSD time = %v", e.ModTime)
	}
}

func TestParseHelpers(t *testing.T) {
	if got := parseQuoted(`"/a ""b"" c" is current directory`); got != `/a "b" c` {
		t.Errorf("parseQuoted = %q", got)
	}
	if got := parseQuoted(`/plain is cwd`); got != "/plain" {
		t.Errorf("parseQuoted unquoted = %q", got)
	}
	if h, p, err := parsePASV("Entering Passive Mode (192,168,1,2,195,80)."); err != nil || h != "192.168.1.2" || p != 195*256+80 {
		t.Errorf("parsePASV = %s %d %v", h, p, err)
	}
	if _, _, err := parsePASV("Entering Passive Mode (1,2,3)"); err == nil {
		t.Error("bad PASV accepted")
	}
	if _, _, err := parsePASV("(300,1,1,1,1,1)"); err == nil {
		t.Error("PASV octet > 255 accepted")
	}
	f := parseFeatures("Features:\n MDTM\n MLST type*;size*;\n UTF8\n REST STREAM\nEnd")
	if _, ok := f["MDTM"]; !ok || f["MLST"] != "type*;size*;" || f["REST"] != "STREAM" {
		t.Errorf("features = %v", f)
	}
	if ft, ok := parseFactTime("20240229235959"); !ok || !ft.Equal(date(2024, 2, 29, 23, 59, 59)) {
		t.Errorf("parseFactTime = %v %v", ft, ok)
	}
	if _, ok := parseFactTime("2024"); ok {
		t.Error("short fact time accepted")
	}
	for _, tc := range []struct {
		name string
		want bool
	}{{"x.TXT", true}, {"dir/y.html", true}, {"z.bin", false}, {`C:\a\b.css`, true}} {
		if got := IsASCIIName(tc.name, DefaultASCIIMasks); got != tc.want {
			t.Errorf("IsASCIIName(%q) = %v", tc.name, got)
		}
	}
	if toLatin1("café€") != "caf\xe9?" || fromLatin1("caf\xe9") != "café" {
		t.Error("Latin-1 conversion wrong")
	}
	if maskCmd("PASS hunter2") != "PASS ****" || maskCmd("pass x") != "pass ****" || maskCmd("USER x") != "USER x" {
		t.Error("maskCmd wrong")
	}
}
