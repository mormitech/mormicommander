package main

// Packing to 7z with an installed 7-Zip (7zz, 7z or 7za).
//
// 7-Zip packs a folder named on its command line completely, even with -r- (checked with 7-Zip 26), so "pack without
// subfolders" and "pack without paths" are done here: the files to pack are listed one by one in a UTF-8 list file.
// -spd turns off wildcards, so a file called "*.txt" is only that file. "Move to archive" deletes a source file only
// when the tested archive lists it under the expected name with the same size, and a folder only when it is empty
// afterwards, so nothing that was not packed is ever deleted.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// vfs7zItem is one entry to pack: the name given to 7-Zip in the list file and the name it is stored under.
type vfs7zItem struct {
	Arg    string // list-file line: absolute, or relative to vfs7zPlan.Dir
	Stored string // the name inside the archive, with "/" separators
}

// vfs7zPlan is what is handed to 7-Zip.
type vfs7zPlan struct {
	Dir   string // working folder for relative list-file lines ("" = none)
	Items []vfs7zItem
}

// vfs7zPlanFor lists what to pack. Without the options each source goes in under its base name with everything below
// it. NoPaths packs every file under its own name (two files with one name are refused, since one would be lost).
// NoRecurse packs only the files directly inside a selected folder, under "folder/name".
func vfs7zPlanFor(sources []string, noPaths, noRecurse bool) (vfs7zPlan, error) {
	var plan vfs7zPlan
	if !noPaths && !noRecurse {
		for _, s := range sources {
			plan.Items = append(plan.Items, vfs7zItem{Arg: s, Stored: filepath.Base(s)})
		}
		return plan, nil
	}
	seen := map[string]string{}
	add := func(abs, stored string) error {
		if other, ok := seen[stored]; ok {
			return fmt.Errorf("%s and %s would both be stored as %s, so one of them would be lost. Pack with path names.",
				other, abs, stored)
		}
		seen[stored] = abs
		plan.Items = append(plan.Items, vfs7zItem{Arg: abs, Stored: stored})
		return nil
	}
	for _, s := range sources {
		fi, err := os.Lstat(s)
		if err != nil {
			return plan, err
		}
		if !fi.IsDir() {
			if err := add(s, filepath.Base(s)); err != nil {
				return plan, err
			}
			continue
		}
		if noRecurse {
			des, err := os.ReadDir(s)
			if err != nil {
				return plan, err
			}
			for _, d := range des {
				if d.IsDir() {
					continue
				}
				stored := filepath.Base(s) + "/" + d.Name()
				if noPaths {
					stored = d.Name()
				}
				if err := add(filepath.Join(s, d.Name()), stored); err != nil {
					return plan, err
				}
			}
			continue
		}
		err = filepath.WalkDir(s, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return add(p, d.Name()) // noPaths here, since noRecurse was handled above
		})
		if err != nil {
			return plan, err
		}
	}
	if len(plan.Items) == 0 {
		return plan, errors.New("There are no files to pack: the selected folders hold only subfolders.")
	}
	if noPaths {
		return plan, nil // absolute list-file lines are stored under their base names
	}
	// with paths but without subfolders: relative lines from the sources' common folder keep "folder/name"
	dir := filepath.Dir(sources[0])
	for _, s := range sources[1:] {
		if filepath.Dir(s) != dir {
			return plan, errors.New("Packing to 7z without subfolders needs all sources in one folder.")
		}
	}
	plan.Dir = dir
	for i := range plan.Items {
		plan.Items[i].Arg = filepath.FromSlash(plan.Items[i].Stored)
	}
	return plan, nil
}

// vfs7zPasswordArgs gives the password to 7-Zip. Elsewhere it is typed on 7-Zip's standard input when 7-Zip asks, so
// it never shows in the process list. On Windows it stays an argument (only the same user can read it there).
func vfs7zPasswordArgs(pw string, asking bool) (args []string, stdin string) {
	if pw == "" {
		return nil, ""
	}
	if runtime.GOOS == "windows" {
		return []string{"-p" + pw}, ""
	}
	if asking {
		return []string{"-p"}, pw + "\n" // "a": a bare -p makes 7-Zip ask
	}
	return nil, pw + "\n" // "t" and "l" ask by themselves for an encrypted archive
}

// vfs7zRun runs 7-Zip and returns its output, or the last line of it as the error.
func vfs7zRun(ctx context.Context, exe, dir, stdin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		for _, l := range strings.Split(msg, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "ERROR:") {
				msg = strings.TrimSpace(l)
			}
		}
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return string(out), fmt.Errorf("7-Zip failed: %s", msg)
	}
	return string(out), nil
}

// vfs7zEntry is a file listed by `7z l -slt`.
type vfs7zEntry struct {
	Size   int64
	Folder bool
}

// vfs7zParseSlt reads the entries of a `7z l -slt` listing (the archive's own block, before "----------", is skipped).
func vfs7zParseSlt(out string) map[string]vfs7zEntry {
	got := map[string]vfs7zEntry{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	inEntries := false
	name := ""
	cur := vfs7zEntry{Size: -1}
	flush := func() {
		if name != "" {
			got[name] = cur
		}
		name, cur = "", vfs7zEntry{Size: -1}
	}
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(l, "----------") {
			inEntries = true
			continue
		}
		if !inEntries {
			continue
		}
		k, v, ok := strings.Cut(l, " = ")
		switch {
		case !ok && strings.TrimSpace(l) == "":
			flush()
		case k == "Path":
			flush()
			name = filepath.ToSlash(v)
		case k == "Size":
			cur.Size, _ = strconv.ParseInt(v, 10, 64)
		case k == "Folder":
			cur.Folder = v == "+"
		case k == "Attributes" && strings.HasPrefix(v, "D"):
			cur.Folder = true
		}
	}
	flush()
	return got
}

// vfsPack7z packs local sources into a 7z with 7-Zip. For "move to archive" the archive is tested and listed before
// anything is deleted, and only what it holds is deleted.
func vfsPack7z(ctx context.Context, target string, sources []string, o vfsPackOpts) error {
	exe := vfsSevenZip()
	if exe == "" {
		return errors.New("Packing to 7z needs 7-Zip (7zz or 7z) installed.")
	}
	plan, err := vfs7zPlanFor(sources, o.NoPaths, o.NoRecurse)
	if err != nil {
		return err
	}
	list, err := os.CreateTemp("", "mc-7z-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(list.Name())
	for _, it := range plan.Items {
		fmt.Fprintln(list, it.Arg)
	}
	if err := list.Close(); err != nil {
		return err
	}
	args := []string{"a", "-t7z", "-y", "-bd", "-spd", "-scsUTF-8"}
	if o.Level > 0 {
		args = append(args, "-mx="+strconv.Itoa(o.Level))
	}
	pwArgs, stdin := vfs7zPasswordArgs(o.Password, true)
	if o.Password != "" {
		args = append(append(args, pwArgs...), "-mhe=on")
	}
	if o.VolumeSize > 0 {
		args = append(args, "-v"+strconv.FormatInt(o.VolumeSize, 10)+"b")
	}
	// the archive path is absolute, so it cannot be read as a switch; a list file is only read before any "--"
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if _, err := vfs7zRun(ctx, exe, plan.Dir, stdin, append(args, abs, "@"+list.Name())...); err != nil {
		return err
	}
	if !o.Move {
		return nil
	}
	targ := abs
	if o.VolumeSize > 0 {
		targ += ".001"
	}
	pwArgs, stdin = vfs7zPasswordArgs(o.Password, false)
	if _, err := vfs7zRun(ctx, exe, "", stdin, append(append([]string{"t", "-bd", "-y"}, pwArgs...), "--", targ)...); err != nil {
		return fmt.Errorf("the new archive failed its test, so nothing was deleted: %v", err)
	}
	out, err := vfs7zRun(ctx, exe, "", stdin, append(append([]string{"l", "-slt", "-y"}, pwArgs...), "--", targ)...)
	if err != nil {
		return fmt.Errorf("the new archive could not be listed, so nothing was deleted: %v", err)
	}
	return vfs7zRemovePacked(sources, plan, o.NoPaths, o.NoRecurse, vfs7zParseSlt(out))
}

// vfs7zRemovePacked deletes the sources that the archive holds: a file when it is listed under its stored name with
// its size, a folder when it is empty afterwards. What is kept is reported.
func vfs7zRemovePacked(sources []string, plan vfs7zPlan, noPaths, noRecurse bool, inArc map[string]vfs7zEntry) error {
	packed := func(stored string, fi fs.FileInfo) bool {
		e, ok := inArc[stored]
		if !ok || e.Folder {
			return false
		}
		if fi.Mode().IsRegular() {
			return e.Size == fi.Size()
		}
		return true // a link: 7-Zip stores the link or its target, either way it is in the archive
	}
	kept := 0
	removeFile := func(abs, stored string) {
		fi, err := os.Lstat(abs)
		if err != nil {
			return
		}
		if !packed(stored, fi) || os.Remove(abs) != nil {
			kept++
		}
	}
	if noPaths || noRecurse {
		for _, it := range plan.Items {
			abs := it.Arg
			if plan.Dir != "" {
				abs = filepath.Join(plan.Dir, it.Arg)
			}
			removeFile(abs, it.Stored)
		}
	} else {
		for _, s := range sources {
			base := filepath.Base(s)
			_ = filepath.WalkDir(s, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(s, p)
				stored := base
				if rel != "." {
					stored = base + "/" + filepath.ToSlash(rel)
				}
				removeFile(p, stored)
				return nil
			})
		}
	}
	if !noRecurse {
		// whole folders were packed: remove the folders that are empty now, deepest first (a folder that still holds
		// something holds a kept file, which is already counted)
		for _, s := range sources {
			var dirs []string
			_ = filepath.WalkDir(s, func(p string, d fs.DirEntry, err error) error {
				if err == nil && d.IsDir() {
					dirs = append(dirs, p)
				}
				return nil
			})
			sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
			for _, d := range dirs {
				_ = os.Remove(d)
			}
		}
	}
	if kept > 0 {
		return fmt.Errorf("%d item(s) were not found in the new archive (or could not be deleted), so they were kept.", kept)
	}
	return nil
}
