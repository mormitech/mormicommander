// This file holds the path handling shared by the FTP and SFTP file systems.
// Total Commander shows a server connection in the panel's path bar as a URL
// such as ftp://peter@example.com/pub/linux or sftp://bob@host:2222/home/bob,
// and the user may type such a URL to connect. Panel paths are therefore URLs
// whose path part is the raw remote path (not percent-encoded, because file
// names with spaces or umlauts must look like themselves). The helpers here
// split such a URL into its parts, build the canonical form used as the panel
// path (lower-case scheme and host, default port dropped, cleaned path, and
// never the password, which must not end up in history or window titles),
// and compute parents and joins with forward slashes on every OS. The
// connection key (scheme, user, host, port) identifies one logged-in session,
// so all panels and jobs showing the same server share its connections.
package vfs

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"

	"mormicommander/internal/remotefs"
)

// remoteURL is a parsed panel URL.
type remoteURL struct {
	Scheme   string // lower case: ftp, ftps, ftpes, sftp, scp
	User     string
	Password string
	HasPass  bool
	Host     string // without brackets
	Port     int    // 0 = default
	Path     string // cleaned absolute remote path, or "" when the URL had none
}

// defaultPorts are dropped from canonical URLs.
var defaultPorts = map[string]int{"ftp": 21, "ftpes": 21, "ftps": 990, "sftp": 22, "scp": 22}

// parseRemote splits a panel URL. The path is taken verbatim (spaces, '%'
// and '#' are ordinary characters in file names); only the user name and
// password may be percent-encoded, so that "@" and ":" can appear in them.
func parseRemote(p string) (remoteURL, error) {
	i := strings.Index(p, "://")
	if i <= 0 {
		return remoteURL{}, fmt.Errorf("vfs: %q is not a URL", remotefs.HidePassword(p))
	}
	u := remoteURL{Scheme: strings.ToLower(p[:i])}
	rest := p[i+3:]
	auth, rpath := rest, ""
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		auth, rpath = rest[:j], rest[j:]
	}
	if k := strings.LastIndexByte(auth, '@'); k >= 0 {
		info := auth[:k]
		auth = auth[k+1:]
		user, pass, hasPass := strings.Cut(info, ":")
		var err error
		if u.User, err = url.PathUnescape(user); err != nil {
			u.User = user
		}
		if hasPass {
			u.HasPass = true
			if u.Password, err = url.PathUnescape(pass); err != nil {
				u.Password = pass
			}
		}
	}
	host, port := auth, ""
	if strings.HasPrefix(auth, "[") {
		end := strings.IndexByte(auth, ']')
		if end < 0 {
			return remoteURL{}, fmt.Errorf("vfs: bad host in %q", remotefs.HidePassword(p))
		}
		host = auth[1:end]
		if r := auth[end+1:]; strings.HasPrefix(r, ":") {
			port = r[1:]
		}
	} else if c := strings.LastIndexByte(auth, ':'); c >= 0 {
		host, port = auth[:c], auth[c+1:]
	}
	if host == "" {
		return remoteURL{}, fmt.Errorf("vfs: %q has no host", remotefs.HidePassword(p))
	}
	u.Host = strings.ToLower(host)
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return remoteURL{}, fmt.Errorf("vfs: bad port in %q", remotefs.HidePassword(p))
		}
		if n != defaultPorts[u.Scheme] {
			u.Port = n
		}
	}
	if rpath != "" {
		rpath = strings.ReplaceAll(rpath, `\`, "/")
		u.Path = path.Clean(rpath)
	}
	return u, nil
}

// base is the connection part without password: "ftp://user@host:port".
func (u remoteURL) base() string {
	var b strings.Builder
	b.WriteString(u.Scheme)
	b.WriteString("://")
	if u.User != "" {
		b.WriteString(escapeUserinfo(u.User))
		b.WriteByte('@')
	}
	host := u.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	b.WriteString(host)
	if u.Port != 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(u.Port))
	}
	return b.String()
}

// String is the canonical panel path: base plus the path ("/" when none).
func (u remoteURL) String() string {
	p := u.Path
	if p == "" {
		p = "/"
	}
	return u.base() + p
}

// key identifies a session: scheme, user, host and port.
func (u remoteURL) key() string { return u.base() }

// hostPort is "host:port" with the given default port.
func (u remoteURL) hostPort(def int) string {
	port := u.Port
	if port == 0 {
		port = def
	}
	return net.JoinHostPort(u.Host, strconv.Itoa(port))
}

// escapeUserinfo percent-encodes the characters that would break parsing.
func escapeUserinfo(s string) string {
	r := strings.NewReplacer("%", "%25", "@", "%40", ":", "%3A", "/", "%2F")
	return r.Replace(s)
}

// cleanURL returns the canonical form of a panel URL, or p unchanged when it
// cannot be parsed (the FS call will report the error).
func cleanURL(p string) string {
	u, err := parseRemote(p)
	if err != nil {
		return p
	}
	return u.String()
}

// urlParent is the folder above a panel URL, "" at the server root.
func urlParent(p string) string {
	u, err := parseRemote(p)
	if err != nil {
		return ""
	}
	if u.Path == "" || u.Path == "/" {
		return ""
	}
	u.Path = path.Dir(u.Path)
	return u.String()
}

// urlRoot is the server root of a panel URL.
func urlRoot(p string) string {
	u, err := parseRemote(p)
	if err != nil {
		return p
	}
	u.Path = "/"
	return u.String()
}

// urlJoin puts name under the folder URL dir.
func urlJoin(dir, name string) string {
	u, err := parseRemote(dir)
	if err != nil {
		return strings.TrimSuffix(dir, "/") + "/" + name
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	u.Path = path.Join(p, name)
	return u.String()
}
