package main

// One small set of archive functions over the three engines, for Pack (Alt+F5), Unpack (Alt+F9), Test
// (Alt+Shift+F9) and copying out of archives: internal/archive/ziparc for zip, internal/archive/tarfmt for the tar
// family and the single-file packers (.gz .bz2 .xz .zst .lzma), internal/archive/sevenrar for 7z and RAR (read
// only). Creating 7z archives uses an installed 7-Zip (7zz, 7z or 7za) when there is one (FEATURES.md F-607). The
// engines do the real work and their safety rules (zip-slip, links, atomic rewrites) hold; this file only maps the
// options of the dialogs onto each engine.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/arcapi"
	"mormicommander/internal/archive/sevenrar"
	"mormicommander/internal/archive/tarfmt"
	"mormicommander/internal/archive/ziparc"
)

// vfsEngineOf names the engine that reads a local file: "zip", "tar", "7z" or "" (not an archive). The content
// decides first (a renamed archive still opens, a .jar is a zip), then the name — the same rule as the panels.
func vfsEngineOf(file string) string {
	if d, err := sevenrar.DetectFile(file); err == nil {
		switch d.Format {
		case sevenrar.Format7z, sevenrar.FormatRar:
			return "7z"
		case sevenrar.FormatZip:
			return "zip"
		case sevenrar.FormatTar, sevenrar.FormatGzip, sevenrar.FormatBzip2, sevenrar.FormatXz, sevenrar.FormatZstd:
			return "tar"
		}
	}
	name := filepath.Base(file)
	switch {
	case tarfmt.DetectFormat(name) != tarfmt.FormatUnknown:
		return "tar"
	case archive.IsArchiveName(name):
		return "zip"
	}
	return ""
}

// vfsLaterVolume says whether file is the second or later part of a multi-volume set (Unpack and Test skip those:
// the first part brings the whole set).
func vfsLaterVolume(file string) bool {
	d, err := sevenrar.DetectFile(file)
	return err == nil && d.Volume.Multi && !d.Volume.First && d.Volume.Number > 0
}

// vfsArchiveStem is the folder name for "unpack each archive to a separate subfolder": "a.tar.gz" → "a",
// "x.part1.rar" → "x".
func vfsArchiveStem(name string) string {
	if tarfmt.DetectFormat(name) != tarfmt.FormatUnknown {
		if b := tarfmt.BaseName(name); b != "" {
			return b
		}
	}
	if s := sevenrar.ArchiveStem(name); s != "" && s != name {
		return s
	}
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name + ".files"
}

// vfsPasswordFunc asks for an archive's password (nil: encrypted data fails with "a password is needed").
type vfsPasswordFunc func(arc, entry string, attempt int) (string, bool)

// vfsProgressFunc gets the current entry and the bytes done of the whole job (total 0 when unknown).
type vfsProgressFunc func(name string, done, total int64, files int)

// vfsExtractOpts are the Unpack dialog's options for one archive.
type vfsExtractOpts struct {
	Paths    []string // selected entries (archive paths from the root); empty = everything below Base
	Base     string   // the archive folder the panel shows; entries are written relative to it
	Mask     string   // TC file mask ("*.txt *.doc | old*"); "" or "*.*" = all
	NoPaths  bool     // "unpack path names" switched off
	Password vfsPasswordFunc
	Progress vfsProgressFunc
}

// vfsMask normalises the mask field: TC's "*.*" and "*" take every file.
func vfsMask(m string) string {
	m = strings.TrimSpace(m)
	if m == "*.*" || m == "*" {
		return ""
	}
	return m
}

// vfsExtract unpacks a local archive file into dest (made when missing), which should be an empty folder: existing
// files there are overwritten (callers unpack into a staging folder and merge it with the overwrite question). Problems
// with single entries don't stop the run; they come back as sentences.
//
// Without paths two members with one name (dir1/x.txt, dir2/x.txt) land on one name in dest. They are kept both, the
// later one under a free name, so the merge question sees both (#149); tar, which can't rename, skips the later one and
// says so.
func vfsExtract(ctx context.Context, file, dest string, o vfsExtractOpts) (files int, problems []string, err error) {
	mask := vfsMask(o.Mask)
	zipPolicy, sevenMode, tarPolicy := archive.PolicyOverwrite, sevenrar.OverwriteAll, arcapi.ConflictOverwrite
	if o.NoPaths {
		zipPolicy, sevenMode, tarPolicy = archive.PolicyAutoRename, sevenrar.OverwriteRename, arcapi.ConflictAsk
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, nil, err
	}
	onErr := func(p string, e error) bool {
		problems = append(problems, fmt.Sprintf("%s: %s", p, vfsErrText(e)))
		return true
	}
	switch vfsEngineOf(file) {
	case "zip":
		r, err := ziparc.Open(file, archive.OpenOptions{Password: vfsArchivePassword(o.Password)})
		if err != nil {
			return 0, nil, err
		}
		defer r.Close()
		res, err := archive.Extract(ctx, r, archive.ExtractOptions{
			Dest: dest, Paths: o.Paths, Base: o.Base, NoPaths: o.NoPaths, Mask: mask, Policy: zipPolicy,
			OnError: onErr,
			Progress: func(p archive.Progress) {
				if o.Progress != nil {
					o.Progress(p.Path, p.Done, p.Total, p.Files)
				}
			},
		})
		for _, s := range res.Skipped {
			if s.Reason != "" && !strings.Contains(s.Reason, "exist") {
				problems = append(problems, fmt.Sprintf("%s: %s", s.Path, s.Reason))
			}
		}
		return res.Files, problems, err
	case "tar":
		r, err := tarfmt.Open(file)
		if err != nil {
			return 0, nil, err
		}
		defer r.Close()
		if _, err := r.Entries(ctx); err != nil { // (also gives the progress a total)
			return 0, nil, err
		}
		var m archive.Mask
		if mask != "" {
			m = archive.ParseMask(mask)
		}
		names := o.Paths
		if len(names) == 0 && o.Base != "" {
			names = []string{o.Base}
		}
		n := 0
		err = r.Extract(ctx, names, dest, arcapi.ExtractOptions{
			Flatten: o.NoPaths, StripPrefix: o.Base, Conflict: tarPolicy,
			OnConflict: func(c arcapi.Conflict) arcapi.ConflictPolicy {
				problems = append(problems, fmt.Sprintf("%s: another file named %s was unpacked already, so this one was skipped. Unpack with path names to get both.", c.Entry.Path, path.Base(c.Entry.Path)))
				return arcapi.ConflictSkip
			},
			Filter: func(e arcapi.Entry) bool { return mask == "" || e.IsDir || m.Match(path.Base(e.Path)) },
			Progress: func(p arcapi.Progress) {
				n = p.Files
				if o.Progress != nil {
					o.Progress(p.Name, p.Bytes, p.Total, p.Files)
				}
			},
		})
		var ee *arcapi.EntryError
		if err != nil && errors.As(err, &ee) && ctx.Err() == nil {
			for _, e := range vfsUnjoin(err) {
				problems = append(problems, vfsErrText(e))
			}
			err = nil
		}
		return n, problems, err
	case "7z":
		a, err := sevenrar.Open(file, sevenrar.Options{PasswordFunc: vfsSevenPassword(o.Password)})
		if err != nil {
			return 0, nil, err
		}
		defer a.Close()
		res, err := a.Extract(ctx, dest, sevenrar.ExtractOptions{
			Paths: o.Paths, BaseDir: o.Base, Mask: mask, FlattenPaths: o.NoPaths, Overwrite: sevenMode,
			OnError: func(e *sevenrar.EntryError) bool { return onErr(e.Path, e.Err) },
			Progress: func(p sevenrar.Progress) {
				if o.Progress != nil {
					o.Progress(p.Entry, p.Bytes, p.TotalBytes, p.Files)
				}
			},
		})
		if err != nil && ctx.Err() == nil && len(res.Errors) > 0 && !errors.Is(err, sevenrar.ErrPasswordCancelled) && !errors.Is(err, sevenrar.ErrMissingVolume) {
			err = nil // the per-entry errors are in problems already
		}
		return res.Files, problems, err
	}
	return 0, nil, fmt.Errorf("%s is not an archive of a known type", filepath.Base(file))
}

// vfsUnjoin splits an errors.Join error into its parts.
func vfsUnjoin(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}

func vfsArchivePassword(f vfsPasswordFunc) archive.PasswordFunc {
	if f == nil {
		return nil
	}
	return func(arc, entry string, attempt int) (string, error) {
		pw, ok := f(arc, entry, attempt)
		if !ok {
			return "", archive.ErrCanceled
		}
		return pw, nil
	}
}

func vfsSevenPassword(f vfsPasswordFunc) func(sevenrar.PasswordRequest) (string, bool) {
	if f == nil {
		return nil
	}
	return func(r sevenrar.PasswordRequest) (string, bool) { return f(r.Archive, r.Entry, r.Attempt) }
}

// vfsTest tests one local archive (Alt+Shift+F9): the number of files checked and the damaged ones as sentences. err
// is set when the test could not run at all (not an archive, cancelled, no password).
func vfsTest(ctx context.Context, file string, pw vfsPasswordFunc, progress vfsProgressFunc) (files int, problems []string, err error) {
	switch vfsEngineOf(file) {
	case "zip":
		r, err := ziparc.Open(file, archive.OpenOptions{Password: vfsArchivePassword(pw)})
		if err != nil {
			return 0, nil, err
		}
		defer r.Close()
		rep, err := archive.Verify(ctx, r, archive.VerifyOptions{Progress: func(p archive.Progress) {
			if progress != nil {
				progress(p.Path, p.Done, p.Total, p.Files)
			}
		}})
		for _, f := range rep.Failed {
			problems = append(problems, fmt.Sprintf("%s: %s", f.Path, vfsErrText(f.Err)))
		}
		return rep.Files, problems, err
	case "tar":
		n := 0
		err := tarfmt.Test(ctx, file, func(p arcapi.Progress) {
			n = p.Files
			if progress != nil {
				progress(p.Name, p.ArchiveRead, p.ArchiveSize, p.Files)
			}
		})
		if err != nil && ctx.Err() == nil {
			return n, []string{vfsErrText(err)}, nil
		}
		return n, nil, err
	case "7z":
		a, err := sevenrar.Open(file, sevenrar.Options{PasswordFunc: vfsSevenPassword(pw)})
		if err != nil {
			return 0, nil, err
		}
		defer a.Close()
		tr, err := a.Test(ctx, func(p sevenrar.Progress) {
			if progress != nil {
				progress(p.Entry, p.Bytes, p.TotalBytes, p.Files)
			}
		})
		for _, e := range tr.Errors {
			problems = append(problems, fmt.Sprintf("%s: %s", e.Path, vfsErrText(e.Err)))
		}
		return tr.Files, problems, err
	}
	return 0, nil, fmt.Errorf("%s is not an archive of a known type", filepath.Base(file))
}

// ---- packing ----

// vfsPacker is one entry of the Pack dialog's packer list.
type vfsPacker struct {
	Name      string `json:"name"`      // what the page sends back: "zip", "tar", "tar.gz", ..., "7z"
	Label     string `json:"label"`     // shown in the list
	Ext       string `json:"ext"`       // the archive name's extension
	Single    bool   `json:"single"`    // packs exactly one file (gz, bz2, xz, zst)
	Levels    bool   `json:"levels"`    // has a compression level
	Encrypt   bool   `json:"encrypt"`   // can encrypt
	Volumes   bool   `json:"volumes"`   // can split into volumes
	Available bool   `json:"available"` // can be used on this computer
	Note      string `json:"note"`      // why not, when it can't
}

// vfsPackers lists the packers of the Pack dialog in TC's order, zip first.
func vfsPackers() []vfsPacker {
	out := []vfsPacker{{Name: "zip", Label: "ZIP", Ext: "zip", Levels: true, Encrypt: true, Volumes: true, Available: true}}
	for _, fi := range tarfmt.Formats() {
		if fi.Format == tarfmt.FormatTarLzma || fi.Format == tarfmt.FormatLzma {
			continue // (readable, but nobody makes these any more; TC has no packer for them)
		}
		label := map[string]string{"tar": "TAR", "tar.gz": "TGZ (tar.gz)", "tar.bz2": "TBZ (tar.bz2)", "tar.xz": "TXZ (tar.xz)",
			"tar.zst": "TZST (tar.zst)", "gz": "GZ (one file)", "bz2": "BZ2 (one file)", "xz": "XZ (one file)", "zst": "ZST (one file)"}[fi.Name]
		if label == "" {
			label = strings.ToUpper(fi.Name)
		}
		out = append(out, vfsPacker{Name: fi.Name, Label: label, Ext: strings.TrimPrefix(fi.Ext, "."), Single: fi.Single,
			Levels: fi.Format != tarfmt.FormatTar, Available: fi.CanWrite})
	}
	seven := vfsPacker{Name: "7z", Label: "7Z", Ext: "7z", Levels: true, Encrypt: true, Volumes: true}
	if vfsSevenZip() != "" {
		seven.Available = true
	} else {
		seven.Note = "Packing to 7z needs 7-Zip (7zz or 7z) installed; 7z archives can always be opened and unpacked."
	}
	return append(out, seven)
}

// vfsSevenZip is the installed 7-Zip program, or "".
func vfsSevenZip() string {
	if p, configured, off := pluginTool(func(z, _, _, _ string) string { return z }); off {
		return "" // (Options > Plugins > Use configured external tools off)
	} else if configured {
		return p // (Options > Plugins > 7-Zip program)
	}
	for _, n := range []string{"7zz", "7z", "7za"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// vfsPackerFor finds a packer by name.
func vfsPackerFor(name string) (vfsPacker, bool) {
	for _, p := range vfsPackers() {
		if p.Name == name {
			return p, true
		}
	}
	return vfsPacker{}, false
}

// vfsPackOpts are the Pack dialog's options for one archive.
type vfsPackOpts struct {
	Format     string // a vfsPacker name
	Level      int    // 1..9, 0 = the packer's default
	Password   string
	Encryption string // zip: "aes256" (default), "aes192", "aes128", "zipcrypto"
	NoPaths    bool   // "also pack path names" switched off
	NoRecurse  bool   // "recursively pack subfolders" switched off
	VolumeSize int64  // > 0: "multiple disk archive" with parts of this many bytes
	Move       bool   // "move to archive": the sources are deleted after the archive was written and tested
	Progress   vfsProgressFunc
}

// vfsPack packs local sources into the local archive target, adding to it when it exists (zip, tar). It returns the
// local paths that were moved (Move).
func vfsPack(ctx context.Context, target string, sources []string, o vfsPackOpts) error {
	pk, ok := vfsPackerFor(o.Format)
	if !ok {
		return fmt.Errorf("mormiCommander has no packer called %q", o.Format)
	}
	if !pk.Available {
		return errors.New(pk.Note)
	}
	if pk.Single {
		if len(sources) != 1 {
			return fmt.Errorf("The %s packer packs one file only; choose ZIP or TAR for several files.", pk.Label)
		}
		if fi, err := os.Stat(sources[0]); err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("The %s packer packs one file only, not a folder; choose ZIP or TAR.", pk.Label)
		}
	}
	if o.Password != "" && !pk.Encrypt {
		return fmt.Errorf("%s archives can't be encrypted; choose ZIP or 7Z.", pk.Label)
	}
	if o.VolumeSize > 0 && !pk.Volumes {
		return fmt.Errorf("%s archives can't be split into parts; choose ZIP or 7Z.", pk.Label)
	}
	if fi, err := os.Stat(target); err == nil {
		if fi.IsDir() {
			return fmt.Errorf("%s is a folder.", filepath.Base(target))
		}
		eng := vfsEngineOf(target)
		want := map[string]string{"zip": "zip", "7z": "7z"}[pk.Name]
		if want == "" {
			want = "tar"
		}
		if eng != want {
			return fmt.Errorf("%s is already there and is not a %s archive, so nothing can be added to it.", filepath.Base(target), pk.Label)
		}
	}
	if pk.Name == "zip" && o.Password == "" && o.VolumeSize == 0 {
		if exe := externalZip(); exe != "" {
			return packExternalZip(ctx, exe, target, sources, o)
		}
	}
	progress := func(p archive.Progress) {
		if o.Progress != nil {
			o.Progress(p.Path, p.Done, p.Total, p.Files)
		}
	}
	switch {
	case pk.Name == "zip":
		items := make([]archive.AddItem, len(sources))
		for i, s := range sources {
			items[i] = archive.AddItem{Source: s}
		}
		enc := map[string]archive.Encryption{"zipcrypto": archive.EncryptZipCrypto, "aes128": archive.EncryptAES128,
			"aes192": archive.EncryptAES192}[o.Encryption]
		if o.Password != "" && enc == archive.EncryptNone {
			enc = archive.EncryptAES256
		}
		_, err := ziparc.Packer{}.Update(ctx, target, archive.Changes{Add: items}, archive.PackOptions{
			Level: o.Level, Password: o.Password, Encryption: enc, FlattenPaths: o.NoPaths, NoRecurse: o.NoRecurse,
			VolumeSize: o.VolumeSize, MoveSources: o.Move, Policy: archive.PolicyOverwrite, Progress: progress,
		})
		return err
	case pk.Name == "7z":
		return vfsPack7z(ctx, target, sources, o)
	default:
		tp := func(p arcapi.Progress) {
			if o.Progress != nil {
				o.Progress(p.Name, p.Bytes, p.Total, p.Files)
			}
		}
		co := tarfmt.CreateOptions{Format: tarfmt.DetectFormat("x." + pk.Ext), Level: o.Level, NoPaths: o.NoPaths,
			NoRecurse: o.NoRecurse, RemoveSources: o.Move, Progress: tp}
		if _, err := os.Stat(target); err == nil && !pk.Single {
			return tarfmt.Modify(ctx, target, tarfmt.Changes{Add: sources, AddOptions: co, Level: o.Level, Progress: tp})
		}
		if err := tarfmt.Create(ctx, target, sources, co); err != nil {
			return err
		}
		vfsNewFilePerm(target)
		return nil
	}
}

// vfsNewFilePerm gives a newly packed archive the permissions any new file gets in its folder (rw-r--r-- with the
// usual umask), like a ZIP gets: tarfmt writes through a private temporary file (rw-------) and renames it.
func vfsNewFilePerm(file string) {
	if runtime.GOOS == "windows" {
		return
	}
	probe := filepath.Join(filepath.Dir(file), ".mc-perm-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return
	}
	fi, err := f.Stat()
	f.Close()
	os.Remove(probe)
	if err == nil {
		_ = os.Chmod(file, fi.Mode().Perm())
	}
}

// vfsTopNames lists the names directly inside a local folder, sorted.
func vfsTopNames(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(des))
	for _, d := range des {
		out = append(out, d.Name())
	}
	sort.Strings(out)
	return out, nil
}
