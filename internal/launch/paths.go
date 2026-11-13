// Path helpers that follow the conventions of a chosen operating system instead of the one the code runs on. The
// launcher builds Windows command lines in tests on a Mac, and resolves "cd ..\x" typed into the command line, so it
// cannot rely on path/filepath, which always uses the conventions of the running system.
package launch

import (
	"path"
	"strings"
)

func sepFor(osName string) string {
	if osName == "windows" {
		return `\`
	}
	return "/"
}

// volumeOf returns the Windows volume prefix of p ("C:" or `\\server\share`), or "".
func volumeOf(osName, p string) string {
	if osName != "windows" {
		return ""
	}
	if len(p) >= 2 && p[1] == ':' && isLetter(p[0]) {
		return p[:2]
	}
	if len(p) > 2 && isSlash(p[0]) && isSlash(p[1]) {
		rest := p[2:]
		i := strings.IndexAny(rest, `\/`)
		if i < 0 {
			return p
		}
		j := strings.IndexAny(rest[i+1:], `\/`)
		if j < 0 {
			return p
		}
		return p[:2+i+1+j]
	}
	return ""
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isSlash(c byte) bool  { return c == '/' || c == '\\' }

// isAbsPath says whether p is absolute for osName. On Windows a drive with a root ("C:\x") or a UNC path counts.
func isAbsPath(osName, p string) bool {
	if osName != "windows" {
		return strings.HasPrefix(p, "/")
	}
	v := volumeOf(osName, p)
	if strings.HasPrefix(v, `\\`) || strings.HasPrefix(v, "//") {
		return true
	}
	return v != "" && len(p) > 2 && isSlash(p[2])
}

// cleanPath is path.Clean with the conventions of osName.
func cleanPath(osName, p string) string {
	if osName != "windows" {
		return path.Clean(p)
	}
	vol := volumeOf(osName, p)
	rest := strings.ReplaceAll(p[len(vol):], `\`, "/")
	rooted := strings.HasPrefix(rest, "/") || strings.HasPrefix(vol, `\\`) || strings.HasPrefix(vol, "//")
	var c string
	if rooted {
		c = path.Clean("/" + rest)
	} else {
		c = path.Clean(rest)
		if c == "." && vol != "" {
			c = ""
		}
	}
	vol = strings.ReplaceAll(vol, "/", `\`)
	c = strings.ReplaceAll(c, "/", `\`)
	if strings.HasPrefix(vol, `\\`) && c == `\` {
		return vol
	}
	return vol + c
}

// joinPath resolves rel against the folder base, the way "cd rel" does in base.
func joinPath(osName, base, rel string) string {
	if rel == "" {
		return cleanPath(osName, base)
	}
	if isAbsPath(osName, rel) {
		return cleanPath(osName, rel)
	}
	if osName == "windows" {
		if v := volumeOf(osName, rel); v != "" {
			// "D:" or "D:x" is relative to the root of drive D (we keep no per-drive current folder).
			return cleanPath(osName, v+`\`+rel[len(v):])
		}
		if isSlash(rel[0]) {
			// "\x" is relative to the root of base's drive.
			return cleanPath(osName, volumeOf(osName, base)+rel)
		}
	}
	return cleanPath(osName, base+sepFor(osName)+rel)
}

// withTrailingSep adds the folder separator at the end of p unless it is already there (Total Commander's %P and %T
// end with a backslash).
func withTrailingSep(osName, p string) string {
	if p == "" {
		return p
	}
	if isSlash(p[len(p)-1]) {
		return p
	}
	return p + sepFor(osName)
}

// baseName returns the last element of p for osName.
func baseName(osName, p string) string {
	cut := "/"
	if osName == "windows" {
		cut = `\/`
	}
	p = strings.TrimRight(p, cut)
	if i := strings.LastIndexAny(p, cut); i >= 0 {
		return p[i+1:]
	}
	return p
}

// splitExt splits a file name into the part before the last dot and the extension after it (without the dot). A
// leading dot does not start an extension, so ".profile" has none.
func splitExt(name string) (stem, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}
