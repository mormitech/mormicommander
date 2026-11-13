// Style-aware path helpers. The command-line box and the placeholders must
// join and clean paths the way the target system does, but filepath only
// knows the rules of the machine the code was built on. These helpers
// implement both rule sets (drive letters and UNC shares for Windows, a
// single root for POSIX) plus virtual paths such as ftp://host/dir, which
// always use "/" and keep their scheme and host untouched. That way
// "cd .." inside an FTP connection or a Windows path typed on a Mac test
// machine behaves the same as on the real system.
package params

import (
	"path"
	"strings"
)

// IsVirtual reports whether p is a URL-like virtual path (ftp://, sftp://,
// archive://, ...), which always uses "/" separators.
func IsVirtual(p string) bool { return strings.Contains(p, "://") }

// splitVirtual splits "scheme://host/rest" into "scheme://host" and "/rest".
func splitVirtual(p string) (prefix, rest string) {
	i := strings.Index(p, "://")
	j := strings.IndexByte(p[i+3:], '/')
	if j < 0 {
		return p, "/"
	}
	return p[:i+3+j], p[i+3+j:]
}

// VolumeName returns the drive ("C:") or UNC share ("\\server\share") at the
// start of a Windows path, or "" for POSIX and virtual paths.
func VolumeName(p string, st Style) string {
	if st.orDefault() != StyleWindows || IsVirtual(p) {
		return ""
	}
	p = strings.ReplaceAll(p, "/", `\`)
	if len(p) >= 2 && p[1] == ':' && isLetter(p[0]) {
		return strings.ToUpper(p[:1]) + ":"
	}
	if strings.HasPrefix(p, `\\`) {
		parts := strings.SplitN(p[2:], `\`, 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return `\\` + parts[0] + `\` + parts[1]
		}
	}
	return ""
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// IsAbs reports whether p is absolute for the style. Virtual paths count as
// absolute; on Windows a path needs a volume and a root ("C:\x", UNC).
func IsAbs(p string, st Style) bool {
	if IsVirtual(p) {
		return true
	}
	if st.orDefault() == StylePOSIX {
		return strings.HasPrefix(p, "/")
	}
	p = strings.ReplaceAll(p, "/", `\`)
	v := VolumeName(p, st)
	if strings.HasPrefix(v, `\\`) {
		return true
	}
	return v != "" && strings.HasPrefix(p[len(v):], `\`)
}

// CleanPath removes "." and ".." elements and doubled separators.
func CleanPath(p string, st Style) string {
	if IsVirtual(p) {
		pre, rest := splitVirtual(p)
		return pre + path.Clean(rest)
	}
	if st.orDefault() == StylePOSIX {
		if p == "" {
			return "."
		}
		return path.Clean(p)
	}
	p = strings.ReplaceAll(p, "/", `\`)
	v := VolumeName(p, st)
	rest := p[len(v):]
	if strings.HasPrefix(v, `\\`) && rest == "" {
		rest = `\`
	}
	if rest == "" {
		if v != "" {
			return v + "."
		}
		return "."
	}
	c := path.Clean(strings.ReplaceAll(rest, `\`, "/"))
	return v + strings.ReplaceAll(c, "/", `\`)
}

// JoinPath resolves rel against base: an absolute rel replaces base, a
// Windows rel starting with "\" keeps only base's drive, and "X:name"
// (drive-relative) is treated as "X:\name".
func JoinPath(base, rel string, st Style) string {
	if rel == "" {
		return CleanPath(base, st)
	}
	if IsAbs(rel, st) {
		return CleanPath(rel, st)
	}
	if IsVirtual(base) {
		pre, rest := splitVirtual(base)
		return pre + path.Clean(path.Join(rest, strings.ReplaceAll(rel, `\`, "/")))
	}
	if st.orDefault() == StyleWindows {
		r := strings.ReplaceAll(rel, "/", `\`)
		if v := VolumeName(r, st); v != "" {
			return CleanPath(v+`\`+r[len(v):], st)
		}
		if strings.HasPrefix(r, `\`) {
			return CleanPath(VolumeName(base, st)+r, st)
		}
		return CleanPath(base+`\`+r, st)
	}
	return CleanPath(base+"/"+rel, st)
}

// RootOf returns the root of the volume p lives on: "C:\", "\\srv\share\",
// "/" or "ftp://host/".
func RootOf(p string, st Style) string {
	if IsVirtual(p) {
		pre, _ := splitVirtual(p)
		return pre + "/"
	}
	if st.orDefault() == StylePOSIX {
		return "/"
	}
	return VolumeName(p, st) + `\`
}

// WithTrailingSep returns p ending in exactly one separator, as %P and %T
// deliver it ("C:\dir\", "/tmp/").
func WithTrailingSep(p string, st Style) string {
	if p == "" {
		return ""
	}
	sep := st.Sep()
	if IsVirtual(p) {
		sep = "/"
	}
	if strings.HasSuffix(p, sep) {
		return p
	}
	return p + sep
}

// splitExt splits a file name into base and extension (without the dot),
// as %O and %E do. A leading dot (".bashrc") is part of the name, and
// folders are never split.
func splitExt(name string, dir bool) (base, ext string) {
	if dir {
		return name, ""
	}
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}
