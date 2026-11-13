// Resolving a link means following it, and every link it points to, until a
// real file or folder (or nothing) is reached. TC needs this for Ctrl+PgDn /
// cm_GoToDir on a link (jump to the target), for the Properties dialog, and to
// show broken links differently. We walk the chain one hop at a time instead
// of calling filepath.EvalSymlinks directly, because the UI wants to know the
// difference between "target missing" (ErrBroken, the link can be fixed or
// deleted) and "the links point at each other" (ErrLoop), and wants the list
// of hops to display. Each hop is keyed by its real parent folder plus name,
// so a cycle is detected on the first repeat rather than after MaxHops; the
// final answer, when one exists, comes from EvalSymlinks so that ".." inside
// link texts is handled exactly as the OS does.

package links

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileID identifies a file or folder on one machine regardless of the name
// used to reach it (device and inode on Unix, volume serial and file index on
// Windows). Two names with the same FileID are the same object.
type FileID struct {
	Dev uint64
	Ino uint64
}

// IdentityOf returns the FileID of path, following links. ok is false when
// the platform or file system gives no stable identity.
func IdentityOf(path string) (id FileID, ok bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return FileID{}, false
	}
	return fileIdentity(path, fi)
}

// IsLink reports whether path itself is a file-system link (symlink or
// junction), without following it. Shortcut files are not links here.
func IsLink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && isFSLink(path, fi)
}

func isFSLink(path string, fi fs.FileInfo) bool {
	return fi.Mode()&fs.ModeSymlink != 0 || isJunction(path, fi)
}

// ReadLink returns the raw target text of a symlink or junction, exactly as
// stored (it may be relative). Junction targets lose the NT "\??\" prefix.
func ReadLink(path string) (string, error) {
	t, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	return cleanNTPrefix(t), nil
}

func cleanNTPrefix(t string) string {
	if strings.HasPrefix(t, `\??\UNC\`) {
		return `\\` + t[len(`\??\UNC\`):]
	}
	return strings.TrimPrefix(t, `\??\`)
}

// Resolution is the result of following a link chain.
type Resolution struct {
	// Chain lists the absolute path of every hop, starting with the input.
	Chain []string
	// Final is the real absolute path at the end of the chain (for a broken
	// link, the missing path it points to).
	Final string
	// Exists and IsDir describe Final.
	Exists bool
	IsDir  bool
}

// LoopError is returned (wrapping ErrLoop) when a chain cycles.
type LoopError struct {
	Path  string   // the link that was being resolved
	Chain []string // the hops visited before the repeat
}

func (e *LoopError) Error() string {
	return fmt.Sprintf("%s: link loop after %d hops", e.Path, len(e.Chain))
}

func (e *LoopError) Unwrap() error { return ErrLoop }

// Resolve follows path through any number of symlinks and junctions. A path
// that is not a link resolves to its own real path. A broken chain returns
// the Resolution (Exists false) together with an error wrapping ErrBroken; a
// cycle returns an error wrapping ErrLoop (as *LoopError).
func Resolve(path string) (*Resolution, error) {
	cur, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	res := &Resolution{}
	seen := map[string]bool{}
	for hop := 0; ; hop++ {
		key, perr := realParentKey(cur)
		if perr != nil {
			if errors.Is(perr, ErrLoop) {
				return res, &LoopError{Path: path, Chain: append(res.Chain, cur)}
			}
			res.Chain = append(res.Chain, cur)
			res.Final = cur
			if hop == 0 {
				return res, &fs.PathError{Op: "resolve", Path: path, Err: perr}
			}
			return res, fmt.Errorf("%s: %w", path, ErrBroken)
		}
		res.Chain = append(res.Chain, key)
		if seen[key] || hop > MaxHops {
			return res, &LoopError{Path: path, Chain: res.Chain}
		}
		seen[key] = true
		fi, err := os.Lstat(key)
		if err != nil {
			if isLoopErr(err) {
				return res, &LoopError{Path: path, Chain: res.Chain}
			}
			res.Final = key
			if hop == 0 {
				return res, err // the path itself does not exist
			}
			return res, fmt.Errorf("%s: %w", path, ErrBroken)
		}
		if !isFSLink(key, fi) {
			res.Final, res.Exists, res.IsDir = key, true, fi.IsDir()
			if r, err := filepath.EvalSymlinks(path); err == nil {
				if a, err := filepath.Abs(r); err == nil {
					res.Final = a
				}
			}
			return res, nil
		}
		t, err := ReadLink(key)
		if err != nil {
			return res, err
		}
		cur = targetFromLink(t, key)
	}
}

// realParentKey returns p with its parent folder fully resolved, which is
// where a relative link text is interpreted. It fails with ErrLoop when the
// parent itself loops, and with a not-exist error when it is missing.
func realParentKey(p string) (string, error) {
	dir, base := filepath.Split(filepath.Clean(p))
	if base == "" { // a root
		return filepath.Clean(p), nil
	}
	rd, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if isLoopErr(err) || strings.Contains(err.Error(), "too many links") {
			return "", ErrLoop
		}
		var pe *fs.PathError
		if errors.As(err, &pe) && isLoopErr(pe.Err) {
			return "", ErrLoop
		}
		return "", err
	}
	if !filepath.IsAbs(rd) {
		if a, err := filepath.Abs(rd); err == nil {
			rd = a
		}
	}
	return filepath.Join(rd, base), nil
}

// HasLoop reports whether following path can never end because of a cycle.
func HasLoop(path string) bool {
	_, err := Resolve(path)
	return errors.Is(err, ErrLoop)
}

// PointsToAncestor reports whether the link at path resolves to a folder
// that contains the link itself (for example "sub/up -> .."). Descending into
// such a link while following links recurses forever, so recursive copy and
// size calculation must not enter it (F-722).
func PointsToAncestor(path string) bool {
	if !IsLink(path) {
		return false
	}
	res, err := Resolve(path)
	if err != nil || !res.IsDir {
		return false
	}
	key, err := realParentKey(path)
	if err != nil {
		return false
	}
	return isWithin(filepath.Dir(key), res.Final)
}

// isWithin reports whether p is dir or lies inside it (lexically, on paths
// that are already real and absolute).
func isWithin(p, dir string) bool {
	if sameName(p, dir) {
		return true
	}
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
