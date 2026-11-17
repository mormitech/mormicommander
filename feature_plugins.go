package main

// Options > Plugins (board issue #94): which external programs mormiCommander may use and where they are. With "Use
// configured external tools" off, none is started: packing to 7z, playing films that need converting and the exiftool
// facts are then not there. A path left empty means looking for the program on the PATH (and, for ffmpeg, where
// Homebrew puts it), as before.
//
//   7-Zip     packing to 7z (feature_vfs_pack7z.go)
//   unrar     testing RAR archives (Files > Test archives) when the built-in reader can't read one
//   exiftool  picture sizes and dates for the columns and Multi-rename when the built-in readers know nothing
//   ffprobe   the Lister's media player (feature_lister_stream.go); ffmpeg is looked for next to it

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mormicommander/internal/archive/arcapi"
	"mormicommander/internal/archive/tarfmt"
	"mormicommander/internal/archive/ziparc"
	lst "mormicommander/internal/listing"
)

func init() {
	lst.ExternalImageFacts = exiftoolFacts
}

// pluginTool is the program to start for a tool: the configured path when it is set (and there), else "" with
// configured false so the caller looks for it its own way. off is true when external tools are turned off.
func pluginTool(pick func(path7z, unrar, exiftool, ffprobe string) string) (path string, configured, off bool) {
	s, ok := currentSettings()
	if !ok {
		return "", false, false
	}
	if !s.Plugins.UseExternalTools {
		return "", false, true
	}
	p := strings.TrimSpace(pick(s.Plugins.SevenZipPath, s.Plugins.UnrarPath, s.Plugins.ExifToolPath, s.Plugins.FFprobePath))
	if p == "" {
		return "", false, false
	}
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p, true, false
	}
	if lp, err := exec.LookPath(p); err == nil {
		return lp, true, false
	}
	return "", false, false
}

// exiftoolFacts asks exiftool (Options > Plugins > exiftool program, which must be set) for a picture's size and date.
func exiftoolFacts(path string) (w, h int, date time.Time, ok bool) {
	exe, configured, off := pluginTool(func(_, _, e, _ string) string { return e })
	if off || !configured {
		return 0, 0, time.Time{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "-j", "-n", "-ImageWidth", "-ImageHeight", "-DateTimeOriginal", "-CreateDate", "--", path).Output()
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	var r []map[string]any
	if json.Unmarshal(out, &r) != nil || len(r) == 0 {
		return 0, 0, time.Time{}, false
	}
	num := func(k string) int { f, _ := r[0][k].(float64); return int(f) }
	w, h = num("ImageWidth"), num("ImageHeight")
	for _, k := range []string{"DateTimeOriginal", "CreateDate"} {
		if s, _ := r[0][k].(string); s != "" {
			if t, err := time.ParseInLocation("2006:01:02 15:04:05", s[:min(len(s), 19)], time.Local); err == nil {
				date = t
				break
			}
		}
	}
	return w, h, date, w > 0 || !date.IsZero()
}

// unrarTest tests a RAR archive with unrar (Options > Plugins > unrar program, or unrar on the PATH): ok false when
// there is no unrar to ask; problems lists what it reported.
func unrarTest(ctx context.Context, file, password string) (problems []string, ok bool) {
	exe, _, off := pluginTool(func(_, u, _, _ string) string { return u })
	if off {
		return nil, false
	}
	if exe == "" {
		p, err := exec.LookPath("unrar")
		if err != nil {
			return nil, false
		}
		exe = p
	}
	if password != "" {
		return nil, false // (a password would show in the process list: the built-in reader's answer stands)
	}
	out, err := exec.CommandContext(ctx, exe, "t", "-idq", "-p-", "--", file).CombinedOutput()
	if err == nil {
		return nil, true
	}
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			problems = append(problems, "unrar: "+l)
		}
	}
	if len(problems) == 0 {
		problems = []string{"unrar: " + err.Error()}
	}
	return problems, true
}

// ffmpegConfigured is ffprobe (or ffmpeg next to it) as Options > Plugins names it; found false when nothing is set.
func ffmpegConfigured(name string) (path string, found, off bool) {
	exe, configured, off := pluginTool(func(_, _, _, f string) string { return f })
	if off || !configured {
		return "", false, off
	}
	if name == "ffprobe" {
		return exe, true, false
	}
	c := filepath.Join(filepath.Dir(exe), name+filepath.Ext(exe))
	if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
		return c, true, false
	}
	return "", false, false
}

// archiveOptions hands Options > Zip and Options > Packer to the archive engines before a pack or unpack job starts.
func archiveOptions() {
	s, ok := currentSettings()
	if !ok {
		return
	}
	ziparc.DefaultLevel = s.Zip.Level
	ziparc.NoUTF8Names = !s.Zip.UTF8Names
	ziparc.NoZip64 = !s.Zip.Zip64
	arcapi.NoFileTimes = !s.Zip.SetTimes
	tarfmt.NoOwner = !s.Packer.TarOwner
}

func init() {
	onStartup(func(*App) { archiveOptions() })
}

// externalZip is the zip program of Options > Packer when "Use the internal zip packer" is off and one is named (and
// external tools are allowed); "" means the internal packer.
func externalZip() string {
	s, ok := currentSettings()
	if !ok || s.Packer.InternalZip || !s.Plugins.UseExternalTools {
		return ""
	}
	p := strings.TrimSpace(s.Packer.ExternalZip)
	if p == "" {
		return ""
	}
	if lp, err := exec.LookPath(p); err == nil {
		return lp
	}
	return ""
}

// packExternalZip packs with an Info-ZIP style program ("zip" and its kind): run in each source's folder with the
// names relative to it, so the archive holds the same paths the internal packer would store. Archives with a password
// or in parts stay with the internal packer.
func packExternalZip(ctx context.Context, exe, target string, sources []string, o vfsPackOpts) error {
	args := []string{"-q", "-y"}
	if !o.NoRecurse {
		args = append(args, "-r")
	}
	if o.NoPaths {
		args = append(args, "-j")
	}
	if o.Move {
		args = append(args, "-m")
	}
	if o.Level > 0 && o.Level <= 9 {
		args = append(args, "-"+strconv.Itoa(o.Level))
	}
	byDir := map[string][]string{}
	var order []string
	for _, s := range sources {
		d := filepath.Dir(s)
		if _, ok := byDir[d]; !ok {
			order = append(order, d)
		}
		byDir[d] = append(byDir[d], filepath.Base(s))
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	for _, d := range order {
		cmd := exec.CommandContext(ctx, exe, append(append(append([]string{}, args...), abs, "--"), byDir[d]...)...)
		cmd.Dir = d
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("The zip program %s failed: %s", filepath.Base(exe), msg)
		}
	}
	return nil
}
