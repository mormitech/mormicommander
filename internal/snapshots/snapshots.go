// Package snapshots finds the earlier versions of a file or folder that the file system's own snapshots keep (F-874):
//
//   - ZFS: every dataset has <root>/.zfs/snapshot/<name>/… (even when the .zfs folder is hidden);
//   - Btrfs with snapper: <subvolume>/.snapshots/<n>/snapshot/…, with the date in <n>/info.xml;
//   - macOS: APFS local snapshots listed by "tmutil listlocalsnapshots <volume>" (opened only when one is already
//     mounted, found in "mount"; mounting one needs root), and the backups of a connected Time Machine disk
//     ("tmutil listbackups");
//   - Windows: the shadow copies "vssadmin list shadows /for=C:\" lists (it needs administrator rights), opened as
//     \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopyN\….
//
// Everything here only reads: it never creates, deletes, mounts or restores a snapshot. The file system and the
// programs are reached through Env so tests use fake folder layouts and fake command output.
package snapshots

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Version is one snapshot's copy of the file or folder asked about.
type Version struct {
	Kind    string `json:"kind"`    // zfs, btrfs, apfs, timemachine, vss
	Name    string `json:"name"`    // the snapshot's name
	When    int64  `json:"when"`    // when it was taken, Unix milliseconds (0 when not known)
	Path    string `json:"path"`    // where the item is inside the snapshot ("" when it can't be opened from here)
	Missing bool   `json:"missing"` // the snapshot is there but holds no such item
	Dir     bool   `json:"dir"`
	Size    int64  `json:"size"`
	Mod     int64  `json:"mod"`           // the item's modified time in the snapshot, Unix milliseconds
	Why     string `json:"why,omitempty"` // why Path is empty
	Info    string `json:"info,omitempty"`
}

// Result is what Find found.
type Result struct {
	Live     string    `json:"live"`     // the item's live path (the path asked about, taken out of a snapshot)
	Versions []Version `json:"versions"` // newest first
	Notes    []string  `json:"notes"`    // plain sentences: what could not be looked at, and why
}

// Env is how Find reaches the file system and programs.
type Env struct {
	GOOS       string
	Stat       func(string) (fs.FileInfo, error)
	ReadDir    func(string) ([]fs.DirEntry, error)
	ReadFile   func(string) ([]byte, error)
	Run        func(ctx context.Context, name string, args ...string) (string, error) // combined output
	VolumeRoot func(string) string                                                    // mount point of the volume holding a path ("" unknown)
}

// AdminWhyAPFS says why an APFS local snapshot that is not mounted can't be opened here.
const AdminWhyAPFS = "Not mounted. Mounting an APFS local snapshot needs administrator rights (mount_apfs as root); open it in Time Machine, or mount it read-only in Terminal with sudo mount_apfs -o rdonly -s <name> <volume> <empty folder>."

var (
	reBtrfsSnap = regexp.MustCompile(`^(.*)/\.snapshots/\d+/snapshot(?:/(.*))?$`)
	reZfsSnap   = regexp.MustCompile(`^(.*)/\.zfs/snapshot/[^/]+(?:/(.*))?$`)
	reVSS       = regexp.MustCompile(`(?i)^\\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy\d+(?:\\(.*))?$`)
)

// InSnapshot reports whether p is inside a snapshot this package knows (a ZFS or snapper snapshot folder, a shadow
// copy, a mounted APFS local snapshot or a Time Machine backup), which must never be written to.
func InSnapshot(p string) bool {
	s := strings.ReplaceAll(p, `\`, "/")
	if reVSS.MatchString(p) {
		return true
	}
	return reBtrfsSnap.MatchString(s) || reZfsSnap.MatchString(s) ||
		strings.HasPrefix(s, "/Volumes/.timemachine/") || strings.HasPrefix(s, "/Volumes/com.apple.TimeMachine.localsnapshots/")
}

// LiveOf takes a path inside a ZFS, snapper or shadow-copy snapshot back to the live path it is a copy of (other
// paths come back unchanged), so asking for versions from inside a snapshot lists the same versions.
func LiveOf(p string, goos string) string {
	if m := reVSS.FindStringSubmatch(p); m != nil && goos == "windows" {
		return "" // the drive letter is not in the path: the caller keeps it as it is
	}
	s := p
	for _, re := range []*regexp.Regexp{reBtrfsSnap, reZfsSnap} {
		if m := re.FindStringSubmatch(s); m != nil {
			base := m[1]
			if base == "" {
				base = "/"
			}
			s = path.Join(base, m[2])
		}
	}
	return s
}

// Find lists the snapshot versions of the file or folder at p (an absolute local path).
func Find(ctx context.Context, env Env, p string) Result {
	live := p
	if l := LiveOf(p, env.GOOS); l != "" {
		live = l
	}
	res := Result{Live: live}
	var vs []Version
	if env.GOOS != "windows" {
		vs = append(vs, folderSnapshots(env, live, &res.Notes)...)
	}
	switch env.GOOS {
	case "darwin":
		vs = append(vs, apfsSnapshots(ctx, env, live, &res.Notes)...)
	case "windows":
		vs = append(vs, vssSnapshots(ctx, env, live, &res.Notes)...)
	}
	for i := range vs {
		v := &vs[i]
		if v.Path == "" {
			continue
		}
		fi, err := env.Stat(v.Path)
		if err != nil {
			v.Missing = true
			continue
		}
		v.Dir = fi.IsDir()
		v.Size = fi.Size()
		v.Mod = fi.ModTime().UnixMilli()
	}
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].When > vs[j].When })
	res.Versions = vs
	if res.Versions == nil {
		res.Versions = []Version{}
	}
	if res.Notes == nil {
		res.Notes = []string{}
	}
	return res
}

// ancestors lists p and the folders above it, innermost first (slash paths).
func ancestors(p string) []string {
	p = path.Clean(p)
	out := []string{p}
	for p != "/" && p != "." {
		p = path.Dir(p)
		out = append(out, p)
	}
	return out
}

func isDir(env Env, p string) bool {
	fi, err := env.Stat(p)
	return err == nil && fi.IsDir()
}

// folderSnapshots looks for ZFS and snapper snapshot folders at the innermost dataset / subvolume above live.
func folderSnapshots(env Env, live string, notes *[]string) []Version {
	for _, anc := range ancestors(live) {
		rel := strings.TrimPrefix(strings.TrimPrefix(live, anc), "/")
		var out []Version
		found := false
		if zdir := path.Join(anc, ".zfs", "snapshot"); isDir(env, zdir) {
			found = true
			ents, err := env.ReadDir(zdir)
			if err != nil {
				*notes = append(*notes, "The ZFS snapshots in "+zdir+" can't be read: "+plainErr(err)+".")
			}
			for _, e := range ents {
				snap := path.Join(zdir, e.Name())
				v := Version{Kind: "zfs", Name: e.Name(), Path: path.Join(snap, rel)}
				if t, ok := dateInName(e.Name()); ok {
					v.When = t.UnixMilli()
				} else if fi, err := env.Stat(snap); err == nil {
					v.When = fi.ModTime().UnixMilli()
				}
				out = append(out, v)
			}
		}
		if sdir := path.Join(anc, ".snapshots"); isDir(env, sdir) {
			ents, err := env.ReadDir(sdir)
			if err != nil {
				found = true
				*notes = append(*notes, "The snapper snapshots in "+sdir+" can't be read: "+plainErr(err)+". snapper keeps them for root unless its config allows your user (ALLOW_USERS, SYNC_ACL).")
			}
			for _, e := range ents {
				if _, err := strconv.Atoi(e.Name()); err != nil {
					continue
				}
				snap := path.Join(sdir, e.Name(), "snapshot")
				if !isDir(env, snap) {
					continue
				}
				found = true
				v := Version{Kind: "btrfs", Name: e.Name(), Path: path.Join(snap, rel)}
				if b, err := env.ReadFile(path.Join(sdir, e.Name(), "info.xml")); err == nil {
					v.When, v.Info = snapperInfo(string(b))
				}
				if v.When == 0 {
					if fi, err := env.Stat(snap); err == nil {
						v.When = fi.ModTime().UnixMilli()
					}
				}
				out = append(out, v)
			}
		}
		if found {
			return out
		}
	}
	return nil
}

var (
	reXMLDate = regexp.MustCompile(`<date>([^<]+)</date>`)
	reXMLDesc = regexp.MustCompile(`<description>([^<]*)</description>`)
)

// snapperInfo reads the date (UTC) and description of a snapper info.xml.
func snapperInfo(x string) (int64, string) {
	var when int64
	if m := reXMLDate.FindStringSubmatch(x); m != nil {
		if t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(m[1])); err == nil {
			when = t.UnixMilli()
		}
	}
	desc := ""
	if m := reXMLDesc.FindStringSubmatch(x); m != nil {
		desc = strings.TrimSpace(m[1])
	}
	return when, desc
}

var reNameDate = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})(?:[-_T ](\d{2})[:.]?(\d{2})(?:[:.]?(\d{2}))?)?`)

// dateInName finds a date in a snapshot name as the usual tools write it: com.apple.TimeMachine.2024-01-15-103000,
// autosnap_2024-01-15_10:30:00_hourly, zfs-auto-snap_daily-2024-01-15-1030. The time is taken as local time.
func dateInName(name string) (time.Time, bool) {
	m := reNameDate.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	t := time.Date(n(m[1]), time.Month(n(m[2])), n(m[3]), n(m[4]), n(m[5]), n(m[6]), 0, time.Local)
	if t.Month() != time.Month(n(m[2])) || n(m[2]) == 0 {
		return time.Time{}, false
	}
	return t, true
}

func plainErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

// ---- macOS ----

var reMountSnap = regexp.MustCompile(`^(\S+)@\S+ on (.+) \([^)]*\)$`)

// mountedSnapshots reads "mount" output: snapshot name → where it is mounted.
func mountedSnapshots(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if r := reMountSnap.FindStringSubmatch(strings.TrimSpace(line)); r != nil {
			m[r[1]] = r[2]
		}
	}
	return m
}

// localSnapshotNames reads "tmutil listlocalsnapshots" output.
func localSnapshotNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ":") || strings.Contains(line, " ") {
			continue
		}
		names = append(names, line)
	}
	return names
}

// macRels lists the paths live may have inside a snapshot or backup of its volume: relative to the volume root, and
// for the startup disk also relative to its Data volume (the user's files live there behind firmlinks).
func macRels(live, root string) []string {
	if root == "/" {
		if r := strings.TrimPrefix(live, "/System/Volumes/Data"); r != live {
			return []string{strings.TrimPrefix(r, "/")}
		}
		return []string{strings.TrimPrefix(live, "/")}
	}
	return []string{strings.TrimPrefix(strings.TrimPrefix(live, root), "/")}
}

// firstExisting is the first of base/rel that exists (base/rels[0] when none does, so it shows as missing).
func firstExisting(env Env, base string, rels []string) string {
	for _, r := range rels {
		if p := path.Join(base, r); exists(env, p) {
			return p
		}
	}
	return path.Join(base, rels[0])
}

func exists(env Env, p string) bool { _, err := env.Stat(p); return err == nil }

func apfsSnapshots(ctx context.Context, env Env, live string, notes *[]string) []Version {
	if strings.HasPrefix(live, "/Volumes/.timemachine/") || strings.HasPrefix(live, "/Volumes/com.apple.TimeMachine.localsnapshots/") {
		return nil
	}
	root := "/"
	if env.VolumeRoot != nil {
		if r := env.VolumeRoot(live); r != "" {
			root = r
		}
	}
	rels := macRels(live, root)
	var out []Version
	if txt, err := env.Run(ctx, "tmutil", "listlocalsnapshots", root); err != nil {
		*notes = append(*notes, "APFS local snapshots could not be listed: "+firstLine(txt, err)+".")
	} else if names := localSnapshotNames(txt); len(names) > 0 {
		mounted := map[string]string{}
		if m, err := env.Run(ctx, "mount"); err == nil {
			mounted = mountedSnapshots(m)
		}
		for _, n := range names {
			v := Version{Kind: "apfs", Name: n}
			if t, ok := dateInName(n); ok {
				v.When = t.UnixMilli()
			}
			if mp, ok := mounted[n]; ok {
				v.Path = firstExisting(env, mp, rels)
			} else {
				v.Why = AdminWhyAPFS
			}
			out = append(out, v)
		}
	}
	// a connected Time Machine disk (tmutil prints the backup folders; nothing is listed when none is connected)
	if txt, err := env.Run(ctx, "tmutil", "listbackups"); err == nil {
		for _, b := range strings.Split(txt, "\n") {
			b = strings.TrimSpace(b)
			if !strings.HasPrefix(b, "/") {
				continue
			}
			v := Version{Kind: "timemachine", Name: path.Base(b)}
			if t, ok := dateInName(path.Base(b)); ok {
				v.When = t.UnixMilli()
			}
			v.Path = backupPath(env, b, rels)
			out = append(out, v)
		}
	}
	return out
}

// backupPath finds the item in a Time Machine backup folder, which holds one folder per backed-up volume.
func backupPath(env Env, backup string, rels []string) string {
	ents, _ := env.ReadDir(backup)
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		for _, r := range rels {
			if p := path.Join(backup, e.Name(), r); exists(env, p) {
				return p
			}
		}
	}
	if len(ents) > 0 {
		return path.Join(backup, ents[0].Name(), rels[0])
	}
	return path.Join(backup, rels[0])
}

func firstLine(out string, err error) string {
	s := strings.TrimSpace(out)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		s = err.Error()
	}
	return strings.TrimSuffix(s, ".")
}

// ---- Windows ----

// Shadow is one shadow copy vssadmin listed.
type Shadow struct {
	Volume string // the original volume's drive, "C:" ("" when not shown)
	Device string // \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopyN
	When   time.Time
	Text   string // the creation time as vssadmin wrote it
}

var (
	reVSSTime   = regexp.MustCompile(`(?i)creation time:\s*(.+)$`)
	reVSSOrig   = regexp.MustCompile(`(?i)original volume:\s*\(([A-Za-z]:)\)`)
	reVSSDevice = regexp.MustCompile(`(?i)shadow copy volume:\s*(\\\\\?\\GLOBALROOT\\Device\\\S+)`)
)

// vssTimeLayouts are the creation time forms of common Windows locales.
var vssTimeLayouts = []string{"1/2/2006 3:04:05 PM", "2006-01-02 15:04:05", "02/01/2006 15:04:05", "02.01.2006 15:04:05", "2006. 01. 02. 15:04:05", "2006.01.02. 15:04:05"}

// ParseVSS reads "vssadmin list shadows" output.
func ParseVSS(out string) []Shadow {
	var list []Shadow
	var when time.Time
	var text, vol string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if m := reVSSTime.FindStringSubmatch(line); m != nil {
			text = strings.TrimSpace(m[1])
			when = time.Time{}
			for _, l := range vssTimeLayouts {
				if t, err := time.ParseInLocation(l, text, time.Local); err == nil {
					when = t
					break
				}
			}
		}
		if m := reVSSOrig.FindStringSubmatch(line); m != nil {
			vol = strings.ToUpper(m[1])
		}
		if m := reVSSDevice.FindStringSubmatch(line); m != nil {
			list = append(list, Shadow{Volume: vol, Device: m[1], When: when, Text: text})
		}
	}
	return list
}

func vssSnapshots(ctx context.Context, env Env, live string, notes *[]string) []Version {
	vol := filepath.VolumeName(live)
	if len(vol) != 2 || vol[1] != ':' {
		vol = ""
		if len(live) >= 2 && live[1] == ':' {
			vol = live[:2]
		}
	}
	if vol == "" {
		return nil // a share or a shadow copy path itself: vssadmin only knows local drives
	}
	vol = strings.ToUpper(vol)
	rel := strings.TrimLeft(live[2:], `\/`)
	txt, err := env.Run(ctx, "vssadmin", "list", "shadows", "/for="+vol+`\`)
	if err != nil {
		low := strings.ToLower(txt)
		if strings.Contains(low, "elevated") || strings.Contains(low, "permission") || strings.Contains(low, "administrator") {
			*notes = append(*notes, "Windows shadow copies (Previous Versions) can only be listed with administrator rights: start mormiCommander with Run as administrator to see them.")
		} else if !strings.Contains(low, "no items found") {
			*notes = append(*notes, "Windows shadow copies could not be listed: "+firstLine(txt, err)+".")
		}
		return nil
	}
	var out []Version
	for _, s := range ParseVSS(txt) {
		if s.Volume != "" && s.Volume != vol {
			continue
		}
		v := Version{Kind: "vss", Name: path.Base(strings.ReplaceAll(s.Device, `\`, "/")), Info: s.Text}
		if !s.When.IsZero() {
			v.When = s.When.UnixMilli()
		}
		v.Path = s.Device + `\` + rel
		if rel == "" {
			v.Path = s.Device + `\`
		}
		out = append(out, v)
	}
	return out
}
