package netbrowse

import (
	"bufio"
	"net/url"
	"strings"
)

// MountOptions tune Mount.
type MountOptions struct {
	// Letter is the Windows drive letter to map to ("Z:"); "" = no letter (the UNC path is used). Ignored elsewhere.
	Letter string
	// Persistent asks Windows to reconnect the drive at sign-in. Ignored elsewhere (macOS and GVfs mounts last until
	// they are disconnected or the user signs out).
	Persistent bool
}

// Mounted is where a share can be read.
type Mounted struct {
	Path string `json:"path"` // a local folder, a drive ("Z:\") or a UNC path
	Made bool   `json:"made"` // this call mounted it (false: it was mounted already, or nothing needed mounting)
}

// mountEntry is one line of the system's mount table.
type mountEntry struct{ Source, Point, Type string }

// ParseMountTable reads `mount` (macOS: "//user@nas/media on /Volumes/media (smbfs, nodev, ...)").
func ParseMountTable(out string) []mountEntry {
	var list []mountEntry
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		i := strings.Index(line, " on ")
		j := strings.LastIndex(line, " (")
		if i <= 0 || j <= i+4 {
			continue
		}
		typ := line[j+2:]
		if k := strings.IndexAny(typ, ",)"); k >= 0 {
			typ = typ[:k]
		}
		list = append(list, mountEntry{Source: line[:i], Point: line[i+4 : j], Type: typ})
	}
	return list
}

// matchMount finds where t is mounted in the table: an SMB or AFP source "//[user@]host/share", an NFS source
// "host:/export". Hosts compare without case and by their first label too (Bonjour names come as "nas._smb._tcp.local"
// or "nas.local").
func matchMount(list []mountEntry, t Target) string {
	sameHost := func(h string) bool {
		h = strings.ToLower(h)
		want := strings.ToLower(t.Host)
		if h == want {
			return true
		}
		a, _, _ := strings.Cut(h, ".")
		b, _, _ := strings.Cut(want, ".")
		return a != "" && a == b
	}
	for _, m := range list {
		switch t.Kind {
		case "nfs":
			host, path, ok := strings.Cut(m.Source, ":")
			if ok && sameHost(host) && strings.TrimRight(path, "/") == strings.TrimRight(t.Share, "/") {
				return m.Point
			}
		default:
			src := strings.TrimPrefix(m.Source, "//")
			if src == m.Source {
				continue
			}
			if at := strings.LastIndex(src, "@"); at >= 0 {
				src = src[at+1:]
			}
			host, share, ok := strings.Cut(src, "/")
			if !ok {
				continue
			}
			if h, _, ok := strings.Cut(host, ":"); ok {
				host = h
			}
			if u, err := url.PathUnescape(share); err == nil {
				share = u
			}
			if sameHost(host) && strings.EqualFold(share, t.Share) {
				return m.Point
			}
		}
	}
	return ""
}

// appleScriptString quotes s for an AppleScript string literal.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// MountScript is the AppleScript that mounts t like Finder's Connect to Server. It goes to osascript on standard
// input, so the password is never on a command line.
func MountScript(t Target, c Creds) string {
	u := t
	u.User = ""
	s := "mount volume " + appleScriptString(u.URL())
	if c.User != "" {
		s += " as user name " + appleScriptString(c.User)
		if c.Password != "" {
			s += " with password " + appleScriptString(c.Password)
		}
	}
	return s + "\n"
}

// ParseGioLocalPath reads the "local path:" line of `gio info <uri>`.
func ParseGioLocalPath(out string) string {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if p, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "local path: "); ok {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

// GVfsURI turns the name of a GVfs folder ("smb-share:server=nas,share=media", "nfs:host=nas,prefix=%2Fexport")
// back into the location gio mounts: smb://nas/media, nfs://nas/export. "" when the name is not one of those.
func GVfsURI(name string) string {
	kind, rest, ok := strings.Cut(name, ":")
	if !ok {
		return ""
	}
	kv := map[string]string{}
	for _, p := range strings.Split(rest, ",") {
		k, v, _ := strings.Cut(p, "=")
		if u, err := url.PathUnescape(v); err == nil {
			v = u
		}
		kv[k] = v
	}
	switch kind {
	case "smb-share":
		t := Target{Kind: "smb", Host: kv["server"], Share: kv["share"], User: kv["user"]}
		if t.Host == "" || t.Share == "" {
			return ""
		}
		return t.URL()
	case "nfs":
		t := Target{Kind: "nfs", Host: kv["host"], Share: kv["prefix"]}
		if t.Host == "" {
			return ""
		}
		return t.URL()
	case "afp-volume":
		t := Target{Kind: "afp", Host: kv["host"], Share: kv["volume"], User: kv["user"]}
		if t.Host == "" {
			return ""
		}
		return t.URL()
	}
	return ""
}

// GVfsName is what a GVfs folder is called in the drive bar: "media on nas".
func GVfsName(name string) string {
	uri := GVfsURI(name)
	if uri == "" {
		return name
	}
	t, err := ParseTarget(uri)
	if err != nil || t.Share == "" {
		return name
	}
	return strings.TrimPrefix(t.Share, "/") + " on " + t.Host
}
