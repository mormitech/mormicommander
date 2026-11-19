package snapshots

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// fakeEnv serves a fake folder layout (absolute slash paths) and fake program output; it never touches the disk or
// runs a program.
func fakeEnv(goos string, files fstest.MapFS, progs map[string]string) (Env, *[]string) {
	var ran []string
	rel := func(p string) string {
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			return "."
		}
		return p
	}
	return Env{
		GOOS:     goos,
		Stat:     func(p string) (fs.FileInfo, error) { return fs.Stat(files, rel(p)) },
		ReadDir:  func(p string) ([]fs.DirEntry, error) { return fs.ReadDir(files, rel(p)) },
		ReadFile: func(p string) ([]byte, error) { return fs.ReadFile(files, rel(p)) },
		Run: func(_ context.Context, name string, args ...string) (string, error) {
			k := strings.Join(append([]string{name}, args...), " ")
			ran = append(ran, k)
			out, ok := progs[k]
			if !ok {
				return "", errors.New("not found")
			}
			if strings.HasPrefix(out, "ERR:") {
				return strings.TrimPrefix(out, "ERR:"), errors.New("exit status 1")
			}
			return out, nil
		},
	}, &ran
}

func names(vs []Version) string {
	var s []string
	for _, v := range vs {
		n := v.Kind + ":" + v.Name
		if v.Missing {
			n += "(missing)"
		}
		s = append(s, n)
	}
	return strings.Join(s, ",")
}

func TestZFS(t *testing.T) {
	mt := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	files := fstest.MapFS{
		"tank/home/me/doc.txt": {Data: []byte("now")},
		"tank/home/.zfs/snapshot/autosnap_2024-01-15_10:30:00_hourly/me/doc.txt": {Data: []byte("old")},
		"tank/home/.zfs/snapshot/zfs-auto-snap_daily-2024-02-01-0000/me/doc.txt": {Data: []byte("older")},
		"tank/home/.zfs/snapshot/manual/me/other.txt":                            {Data: []byte("x")},
		"tank/home/.zfs/snapshot/manual":                                         {Mode: fs.ModeDir, ModTime: mt},
		"tank/.zfs/snapshot/outer/home/me/doc.txt":                               {Data: []byte("outer")},
	}
	env, _ := fakeEnv("linux", files, nil)
	r := Find(context.Background(), env, "/tank/home/me/doc.txt")
	if got := names(r.Versions); got != "zfs:zfs-auto-snap_daily-2024-02-01-0000,zfs:autosnap_2024-01-15_10:30:00_hourly,zfs:manual(missing)" {
		t.Fatalf("versions = %s", got)
	}
	v := r.Versions[0]
	if v.Path != "/tank/home/.zfs/snapshot/zfs-auto-snap_daily-2024-02-01-0000/me/doc.txt" || v.Size != 5 || v.Dir {
		t.Fatalf("first = %+v", v)
	}
	if r.Versions[2].When != mt.UnixMilli() {
		t.Fatalf("a name without a date takes the folder's time: %+v", r.Versions[2])
	}
	// from inside a snapshot: the same versions of the live file
	r2 := Find(context.Background(), env, "/tank/home/.zfs/snapshot/manual/me/doc.txt")
	if r2.Live != "/tank/home/me/doc.txt" || names(r2.Versions) != names(r.Versions) {
		t.Fatalf("from a snapshot: live %q versions %s", r2.Live, names(r2.Versions))
	}
}

func TestSnapper(t *testing.T) {
	files := fstest.MapFS{
		"home/me/notes":                         {Mode: fs.ModeDir},
		"home/.snapshots/1/info.xml":            {Data: []byte("<snapshot><num>1</num><date>2024-03-01 08:00:00</date><description>timeline</description></snapshot>")},
		"home/.snapshots/1/snapshot/me/notes/a": {Data: []byte("a")},
		"home/.snapshots/2/info.xml":            {Data: []byte("<snapshot><date>2024-03-02 08:00:00</date><description>before update</description></snapshot>")},
		"home/.snapshots/2/snapshot/me/notes/a": {Data: []byte("a")},
		"home/.snapshots/junk/snapshot/x":       {Data: []byte("not a snapper number")},
		"home/.snapshots/3/info.xml":            {Data: []byte("<date>2024-03-03 08:00:00</date>")},
	}
	env, ran := fakeEnv("linux", files, nil)
	r := Find(context.Background(), env, "/home/me/notes")
	if got := names(r.Versions); got != "btrfs:2,btrfs:1" {
		t.Fatalf("versions = %s", got)
	}
	v := r.Versions[0]
	want := time.Date(2024, 3, 2, 8, 0, 0, 0, time.UTC).UnixMilli()
	if v.Path != "/home/.snapshots/2/snapshot/me/notes" || !v.Dir || v.When != want || v.Info != "before update" {
		t.Fatalf("first = %+v", v)
	}
	if len(*ran) != 0 {
		t.Fatalf("Linux runs no program: %v", *ran)
	}
	if LiveOf("/home/.snapshots/2/snapshot/me/notes", "linux") != "/home/me/notes" || LiveOf("/.snapshots/7/snapshot", "linux") != "/" {
		t.Fatal("LiveOf snapper")
	}
}

func TestNoSnapshots(t *testing.T) {
	env, _ := fakeEnv("linux", fstest.MapFS{"data/a.txt": {Data: []byte("a")}}, nil)
	r := Find(context.Background(), env, "/data/a.txt")
	if len(r.Versions) != 0 || len(r.Notes) != 0 || r.Versions == nil || r.Notes == nil {
		t.Fatalf("got %+v", r)
	}
}

func TestAPFS(t *testing.T) {
	files := fstest.MapFS{
		"Users/me/a.txt": {Data: []byte("now")},
		"Volumes/com.apple.TimeMachine.localsnapshots/Backups.backupdb/Mac/2024-01-15-103000/Macintosh HD - Data/Users/me/a.txt": {Data: []byte("then")},
		"Volumes/.timemachine/ABC/2024-01-10-090000.backup/2024-01-10-090000.backup/Macintosh HD - Data/Users/me/a.txt":          {Data: []byte("backup")},
	}
	progs := map[string]string{
		"tmutil listlocalsnapshots /": "Snapshots for disk /:\ncom.apple.TimeMachine.2024-01-15-103000.local\ncom.apple.TimeMachine.2024-01-16-103000.local\n",
		"mount": "/dev/disk3s1s1 on / (apfs, sealed, local, read-only, journaled)\n" +
			"com.apple.TimeMachine.2024-01-15-103000.local@/dev/disk3s5 on /Volumes/com.apple.TimeMachine.localsnapshots/Backups.backupdb/Mac/2024-01-15-103000/Macintosh HD - Data (apfs, local, read-only, journaled, nobrowse)\n",
		"tmutil listbackups": "/Volumes/.timemachine/ABC/2024-01-10-090000.backup/2024-01-10-090000.backup\n",
	}
	env, _ := fakeEnv("darwin", files, progs)
	r := Find(context.Background(), env, "/Users/me/a.txt")
	if got := names(r.Versions); got != "apfs:com.apple.TimeMachine.2024-01-16-103000.local,apfs:com.apple.TimeMachine.2024-01-15-103000.local,timemachine:2024-01-10-090000.backup" {
		t.Fatalf("versions = %s", got)
	}
	if v := r.Versions[0]; v.Path != "" || v.Why != AdminWhyAPFS {
		t.Fatalf("a snapshot that is not mounted can't be opened: %+v", v)
	}
	if v := r.Versions[1]; v.Path != "/Volumes/com.apple.TimeMachine.localsnapshots/Backups.backupdb/Mac/2024-01-15-103000/Macintosh HD - Data/Users/me/a.txt" || v.Missing || v.Size != 4 {
		t.Fatalf("mounted snapshot = %+v", v)
	}
	if v := r.Versions[2]; v.Path != "/Volumes/.timemachine/ABC/2024-01-10-090000.backup/2024-01-10-090000.backup/Macintosh HD - Data/Users/me/a.txt" || v.Missing {
		t.Fatalf("backup = %+v", v)
	}
	if want := time.Date(2024, 1, 16, 10, 30, 0, 0, time.Local).UnixMilli(); r.Versions[0].When != want {
		t.Fatal("date from the snapshot name")
	}
	// tmutil failing is a note, not an error
	env2, _ := fakeEnv("darwin", files, map[string]string{"tmutil listlocalsnapshots /": "ERR:Operation not permitted\n"})
	r2 := Find(context.Background(), env2, "/Users/me/a.txt")
	if len(r2.Versions) != 0 || len(r2.Notes) != 1 || !strings.Contains(r2.Notes[0], "Operation not permitted") {
		t.Fatalf("got %+v", r2)
	}
}

const vssOut = `vssadmin 1.1 - Volume Shadow Copy Service administrative command-line tool
(C) Copyright 2001-2013 Microsoft Corp.

Contents of shadow copy set ID: {11111111-0000-0000-0000-000000000000}
   Contained 1 shadow copies at creation time: 1/15/2024 10:30:00 AM
      Shadow Copy ID: {22222222-0000-0000-0000-000000000000}
         Original Volume: (C:)\\?\Volume{33333333-0000-0000-0000-000000000000}\
         Shadow Copy Volume: \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1
         Originating Machine: PC
         Type: ClientAccessibleWriters

Contents of shadow copy set ID: {44444444-0000-0000-0000-000000000000}
   Contained 1 shadow copies at creation time: 2/20/2024 4:05:06 PM
      Shadow Copy ID: {55555555-0000-0000-0000-000000000000}
         Original Volume: (C:)\\?\Volume{33333333-0000-0000-0000-000000000000}\
         Shadow Copy Volume: \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy4
`

func TestVSS(t *testing.T) {
	sh := ParseVSS(vssOut)
	if len(sh) != 2 || sh[0].Volume != "C:" || sh[1].Device != `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy4` {
		t.Fatalf("parsed %+v", sh)
	}
	if want := time.Date(2024, 2, 20, 16, 5, 6, 0, time.Local); !sh[1].When.Equal(want) {
		t.Fatalf("time %v", sh[1].When)
	}
	env, ran := fakeEnv("windows", fstest.MapFS{}, map[string]string{`vssadmin list shadows /for=C:\`: vssOut})
	r := Find(context.Background(), env, `C:\Users\me\a.txt`)
	if names(r.Versions) != "vss:HarddiskVolumeShadowCopy4(missing),vss:HarddiskVolumeShadowCopy1(missing)" {
		t.Fatalf("versions = %s (ran %v)", names(r.Versions), *ran)
	}
	if r.Versions[0].Path != `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy4\Users\me\a.txt` {
		t.Fatalf("path %q", r.Versions[0].Path)
	}
	// without administrator rights: a note saying so
	deny := "ERR:Error: You don't have the correct permissions to run this command.  Please run this utility from a command\nwindow that has elevated administrator privileges.\n"
	env2, _ := fakeEnv("windows", fstest.MapFS{}, map[string]string{`vssadmin list shadows /for=D:\`: deny})
	r2 := Find(context.Background(), env2, `d:\x`)
	if len(r2.Versions) != 0 || len(r2.Notes) != 1 || !strings.Contains(r2.Notes[0], "administrator") {
		t.Fatalf("got %+v", r2)
	}
	// none at all: no note
	env3, _ := fakeEnv("windows", fstest.MapFS{}, map[string]string{`vssadmin list shadows /for=C:\`: "ERR:No items found that satisfy the query.\n"})
	if r3 := Find(context.Background(), env3, `C:\x`); len(r3.Notes) != 0 {
		t.Fatalf("got %+v", r3)
	}
	// a share: vssadmin is not asked
	env4, ran4 := fakeEnv("windows", fstest.MapFS{}, nil)
	Find(context.Background(), env4, `\\server\share\a`)
	if len(*ran4) != 0 {
		t.Fatalf("ran %v", *ran4)
	}
}

func TestInSnapshot(t *testing.T) {
	for p, want := range map[string]bool{
		"/tank/.zfs/snapshot/a":                                          true,
		"/tank/.zfs/snapshot/a/b/c":                                      true,
		"/tank/.zfs/snapshot":                                            false,
		"/.snapshots/12/snapshot/etc":                                    true,
		"/.snapshots/12/info.xml":                                        false,
		`\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy3\Users`:          true,
		"/Volumes/.timemachine/X/2024.backup":                            true,
		"/Volumes/com.apple.TimeMachine.localsnapshots/Backups.backupdb": true,
		"/home/me/snapshot":                                              false,
		"C:\\Users\\me":                                                  false,
	} {
		if InSnapshot(p) != want {
			t.Errorf("InSnapshot(%q) = %v", p, !want)
		}
	}
}

func TestDateInName(t *testing.T) {
	for n, want := range map[string]string{
		"com.apple.TimeMachine.2024-01-15-103000.local": "2024-01-15 10:30:00",
		"autosnap_2024-01-15_10:30:00_hourly":           "2024-01-15 10:30:00",
		"zfs-auto-snap_daily-2024-02-01-0000":           "2024-02-01 00:00:00",
		"before-upgrade":                                "",
		"2024-13-01":                                    "",
	} {
		tm, ok := dateInName(n)
		got := ""
		if ok {
			got = tm.Format("2006-01-02 15:04:05")
		}
		if got != want {
			t.Errorf("dateInName(%q) = %q, want %q", n, got, want)
		}
	}
}
