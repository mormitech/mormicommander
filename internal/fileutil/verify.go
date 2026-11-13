// Verifying checksums is TC's cm_CRCCheck: with one or more checksum files
// selected (.sfv, .md5, .sha, .sha256, ...), every file they list is hashed
// again and compared. The result is a report the dialog shows as a list with
// OK / FAILED / missing / unreadable for each file and a summary line, plus a
// plain-text version for "copy report". Names are looked up relative to the
// folder holding the checksum file, the way md5sum -c does it; a file written
// on Windows with backslashes is still found on macOS and Linux. Lines the
// parser could not understand are listed too, so a damaged checksum file is
// never mistaken for a clean pass.

package fileutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// VerifyStatus is the outcome for one listed file.
type VerifyStatus int

const (
	// StatusOK means the checksum matched.
	StatusOK VerifyStatus = iota
	// StatusFailed means the file was read but the checksum differs.
	StatusFailed
	// StatusMissing means the listed file does not exist.
	StatusMissing
	// StatusError means the file exists but could not be read.
	StatusError
)

func (s VerifyStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusFailed:
		return "FAILED"
	case StatusMissing:
		return "missing"
	case StatusError:
		return "read error"
	}
	return fmt.Sprintf("VerifyStatus(%d)", int(s))
}

// VerifyEntry is the result for one line of a checksum file.
type VerifyEntry struct {
	ChecksumEntry
	Path   string // resolved path that was checked
	Status VerifyStatus
	Actual string    // digest computed from the file (empty if not read)
	Used   Algorithm // algorithm that matched or, on failure, the expected one
	Err    error     // read error for StatusError / StatusMissing
}

// VerifyReport is the result for one checksum file.
type VerifyReport struct {
	ChecksumFile string
	Entries      []VerifyEntry
	BadLines     []LineError
	OK           int
	Failed       int
	Missing      int
	Errors       int
}

// AllOK reports whether every listed file matched and no line was damaged.
func (r *VerifyReport) AllOK() bool {
	return r.Failed == 0 && r.Missing == 0 && r.Errors == 0 && len(r.BadLines) == 0 && len(r.Entries) > 0
}

// Summary returns a one-line summary such as "12 OK, 1 FAILED, 2 missing".
func (r *VerifyReport) Summary() string {
	parts := []string{fmt.Sprintf("%d OK", r.OK)}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d FAILED", r.Failed))
	}
	if r.Missing > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", r.Missing))
	}
	if r.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d unreadable", r.Errors))
	}
	if n := len(r.BadLines); n > 0 {
		parts = append(parts, fmt.Sprintf("%d unrecognised line%s", n, plural(n)))
	}
	if len(r.Entries) == 0 && len(r.BadLines) == 0 {
		return "no files listed"
	}
	return strings.Join(parts, ", ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Text returns a plain-text report, one line per file, for copying or saving.
func (r *VerifyReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Checksum file: %s\n", r.ChecksumFile)
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "%-10s %s", e.Status.String(), e.Name)
		if e.Err != nil && e.Status == StatusError {
			fmt.Fprintf(&b, " (%v)", e.Err)
		}
		b.WriteByte('\n')
	}
	for _, l := range r.BadLines {
		fmt.Fprintf(&b, "%-10s line %d: %s\n", "unknown", l.Line, l.Text)
	}
	fmt.Fprintf(&b, "Result: %s\n", r.Summary())
	return b.String()
}

// VerifyOptions configures verification.
type VerifyOptions struct {
	// BaseDir overrides the folder relative names are resolved against. By
	// default it is the folder holding the checksum file.
	BaseDir  string
	Progress ProgressFunc
}

// VerifyChecksumFile verifies one checksum file.
func VerifyChecksumFile(ctx context.Context, path string, opts VerifyOptions) (*VerifyReport, error) {
	reps, err := VerifyChecksumFiles(ctx, []string{path}, opts)
	if len(reps) == 1 {
		return reps[0], err
	}
	return nil, err
}

// VerifyChecksumFiles verifies several checksum files with one shared
// progress total. It stops early only when the context is cancelled or a
// checksum file itself cannot be read.
func VerifyChecksumFiles(ctx context.Context, paths []string, opts VerifyOptions) ([]*VerifyReport, error) {
	type job struct {
		list *ChecksumList
		base string
	}
	var jobs []job
	var total int64
	for _, p := range paths {
		l, err := ParseChecksumFile(p)
		if err != nil {
			return nil, err
		}
		base := opts.BaseDir
		if base == "" {
			base = filepath.Dir(p)
		}
		for _, e := range l.Entries {
			if s, err := fileSize(resolveListed(base, e.Name)); err == nil {
				total += s
			}
		}
		jobs = append(jobs, job{l, base})
	}
	t := newTracker(ctx, opts.Progress, total)
	var reps []*VerifyReport
	for _, j := range jobs {
		r := &VerifyReport{ChecksumFile: j.list.Path, BadLines: j.list.BadLines}
		reps = append(reps, r)
		for _, e := range j.list.Entries {
			if err := t.check(); err != nil {
				return reps, err
			}
			ve := VerifyEntry{ChecksumEntry: e, Used: e.Algorithm}
			ve.Path = resolveListed(j.base, e.Name)
			size, err := fileSize(ve.Path)
			if err != nil {
				if os.IsNotExist(err) {
					ve.Status = StatusMissing
					r.Missing++
				} else {
					ve.Status = StatusError
					r.Errors++
				}
				ve.Err = err
				r.Entries = append(r.Entries, ve)
				continue
			}
			algs := []Algorithm{e.Algorithm}
			if len(e.Candidates) > 0 {
				algs = e.Candidates
			}
			d, err := t.hashFile(ve.Path, algs, size)
			if err != nil {
				if cerr := t.check(); cerr != nil {
					return reps, cerr
				}
				ve.Status = StatusError
				ve.Err = err
				r.Errors++
				r.Entries = append(r.Entries, ve)
				continue
			}
			ve.Status = StatusFailed
			ve.Actual = d[e.Algorithm]
			for _, a := range algs {
				if d[a] == e.Digest {
					ve.Status = StatusOK
					ve.Used = a
					ve.Actual = d[a]
					break
				}
			}
			if ve.Status == StatusOK {
				r.OK++
			} else {
				r.Failed++
			}
			r.Entries = append(r.Entries, ve)
		}
	}
	return reps, nil
}

// resolveListed turns a name from a checksum file into a path. Absolute
// names are used as they are. On macOS and Linux a name with backslashes
// (written by TC on Windows) is tried with forward slashes when the literal
// name does not exist, because a backslash is a legal file name character
// there.
func resolveListed(base, name string) string {
	try := func(n string) string {
		if filepath.IsAbs(n) {
			return filepath.Clean(n)
		}
		return filepath.Join(base, n)
	}
	p := try(name)
	// SFV names keep their leading blanks; an indented line from another
	// tool names the file without them.
	if trimmed := strings.TrimLeft(name, " \t"); trimmed != name && trimmed != "" {
		if _, err := os.Lstat(p); err != nil {
			if _, err := os.Lstat(try(trimmed)); err == nil {
				return resolveListed(base, trimmed)
			}
		}
	}
	if runtime.GOOS != "windows" && strings.Contains(name, `\`) {
		if _, err := os.Lstat(p); err != nil {
			return try(strings.ReplaceAll(name, `\`, "/"))
		}
	}
	return p
}
