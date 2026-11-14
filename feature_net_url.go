package main

// Server URLs for the network feature (FTP, SFTP, WebDAV). A panel shows a server as a URL such as
// ftp://peter@example.com/pub or sftp://bob@host:2222/home/bob, the same canonical form internal/vfs uses: lower-case
// scheme and host, default port dropped, raw (not percent-encoded) path, and never the password. The connection layer
// needs the same "session key" (scheme, user, host and port) as the vfs adapters, to remember a site's settings and a
// password typed once for every later reconnect of that session, so this file parses URLs the same way.

import (
	"fmt"
	"net/url"
	"os/user"
	"path"
	"strconv"
	"strings"

	"mormicommander/internal/remotefs"
)

// netURL is a parsed server URL.
type netURL struct {
	Scheme   string // lower case: ftp, ftpes, ftps, sftp, webdav, webdavs
	User     string
	Password string
	HasPass  bool
	Host     string // lower case, without brackets
	Port     int    // 0 = the scheme's default
	Path     string // cleaned absolute path, "" when the URL had none
}

// netDefaultPorts are dropped from canonical URLs.
var netDefaultPorts = map[string]int{"ftp": 21, "ftpes": 21, "ftps": 990, "sftp": 22, "scp": 22, "webdav": 80, "dav": 80, "webdavs": 443, "davs": 443}

// netSchemes are the URL schemes the network feature serves, with what they are.
var netSchemes = map[string]string{"ftp": "ftp", "ftpes": "ftp", "ftps": "ftp", "sftp": "sftp", "scp": "sftp", "webdav": "webdav", "dav": "webdav", "webdavs": "webdav", "davs": "webdav"}

// netSchemeOf is the lower-case scheme of a URL, "" for a plain path.
func netSchemeOf(p string) string {
	i := strings.Index(p, "://")
	if i <= 0 {
		return ""
	}
	return strings.ToLower(p[:i])
}

// netIsRemote says whether p is a server URL the network feature serves.
func netIsRemote(p string) bool { return netSchemes[netSchemeOf(p)] != "" }

// netParseURL splits a server URL. A text without "://" is taken as an FTP host ("example.com/pub"), like TC's quick
// connect. Only the user name and password may be percent-encoded.
func netParseURL(raw string) (netURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netURL{}, fmt.Errorf("Type a server address, such as ftp://example.com.")
	}
	if !strings.Contains(raw, "://") {
		raw = "ftp://" + raw
	}
	i := strings.Index(raw, "://")
	u := netURL{Scheme: strings.ToLower(raw[:i])}
	if u.Scheme == "scp" {
		u.Scheme = "sftp"
	}
	if u.Scheme == "dav" {
		u.Scheme = "webdav"
	}
	if u.Scheme == "davs" {
		u.Scheme = "webdavs"
	}
	if netSchemes[u.Scheme] == "" {
		return netURL{}, fmt.Errorf("%s:// is not a server address mormiCommander can open (use ftp://, ftps://, ftpes://, sftp://, webdav:// or webdavs://).", u.Scheme)
	}
	rest := raw[i+3:]
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
			return netURL{}, fmt.Errorf("The server address %s has a broken host name.", remotefs.HidePassword(raw))
		}
		host = auth[1:end]
		if r := auth[end+1:]; strings.HasPrefix(r, ":") {
			port = r[1:]
		}
	} else if c := strings.LastIndexByte(auth, ':'); c >= 0 {
		host, port = auth[:c], auth[c+1:]
	}
	if host == "" {
		return netURL{}, fmt.Errorf("The server address has no host name.")
	}
	u.Host = strings.ToLower(host)
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return netURL{}, fmt.Errorf("%s is not a port number (1 to 65535).", port)
		}
		if n != netDefaultPorts[u.Scheme] {
			u.Port = n
		}
	}
	if rpath != "" {
		u.Path = path.Clean(strings.ReplaceAll(rpath, `\`, "/"))
	}
	if u.Scheme == "sftp" && u.User == "" {
		u.User = netLocalUser() // like the ssh command (and internal/vfs): no user means the local login name
	}
	return u, nil
}

// netLocalUser is the login name of the person running the app (without a Windows domain).
func netLocalUser() string {
	cur, err := user.Current()
	if err != nil {
		return ""
	}
	name := cur.Username
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// netEscapeUser percent-encodes the characters that would break parsing (as internal/vfs does).
func netEscapeUser(s string) string {
	return strings.NewReplacer("%", "%25", "@", "%40", ":", "%3A", "/", "%2F").Replace(s)
}

// key is the session: "scheme://user@host:port", without password and path. It equals the key internal/vfs uses.
func (u netURL) key() string {
	var b strings.Builder
	b.WriteString(u.Scheme)
	b.WriteString("://")
	if u.User != "" {
		b.WriteString(netEscapeUser(u.User))
		b.WriteByte('@')
	}
	if strings.Contains(u.Host, ":") {
		b.WriteString("[" + u.Host + "]")
	} else {
		b.WriteString(u.Host)
	}
	if u.Port != 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(u.Port))
	}
	return b.String()
}

// String is the canonical panel URL (never with the password).
func (u netURL) String() string {
	p := u.Path
	if p == "" {
		p = "/"
	}
	return u.key() + p
}

// withPassword is the URL with the password in it, for the first dial of a session only (it never reaches the page).
func (u netURL) withPassword() string {
	if u.Password == "" {
		return u.String()
	}
	k := u
	k.User = netEscapeUser(u.User) + ":" + netEscapeUser(u.Password)
	s := k.Scheme + "://" + k.User + "@"
	if strings.Contains(u.Host, ":") {
		s += "[" + u.Host + "]"
	} else {
		s += u.Host
	}
	if u.Port != 0 {
		s += ":" + strconv.Itoa(u.Port)
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	return s + p
}

// netKeyFor is the session key of a panel URL, "" when it is no server URL.
func netKeyFor(p string) string {
	if !netIsRemote(p) {
		return ""
	}
	u, err := netParseURL(p)
	if err != nil {
		return ""
	}
	return u.key()
}
