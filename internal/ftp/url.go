// This file implements quick connect by URL: typing "ftp://user@host/dir"
// into the path bar or command line opens a connection, as in Total
// Commander. The scheme picks the security mode (ftp = plain, ftpes =
// explicit TLS, ftps = implicit TLS on port 990), user name and password
// may be given in the URL, and the path becomes the folder to open. A bare
// "host/dir" is treated as ftp://. FormatURL goes the other way for the
// path bar and history, and never includes the password.
package ftp

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// IsURL reports whether s looks like an FTP URL the path bar should hand
// to ParseURL.
func IsURL(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(l, "ftp://") || strings.HasPrefix(l, "ftps://") || strings.HasPrefix(l, "ftpes://")
}

// ParseURL turns an FTP URL into a Config and the remote path to open
// ("" when the URL has none).
func ParseURL(raw string) (Config, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Config{}, "", errors.New("ftp: empty URL")
	}
	if !strings.Contains(raw, "://") {
		raw = "ftp://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Config{}, "", fmt.Errorf("ftp: bad URL: %w", err)
	}
	var cfg Config
	switch strings.ToLower(u.Scheme) {
	case "ftp":
	case "ftpes":
		cfg.Security = SecurityExplicitTLS
	case "ftps":
		cfg.Security = SecurityImplicitTLS
	default:
		return Config{}, "", fmt.Errorf("ftp: unsupported URL scheme %q", u.Scheme)
	}
	cfg.Host = u.Hostname()
	if cfg.Host == "" {
		return Config{}, "", errors.New("ftp: URL has no host")
	}
	if ps := u.Port(); ps != "" {
		port, err := strconv.Atoi(ps)
		if err != nil || port <= 0 || port > 65535 {
			return Config{}, "", fmt.Errorf("ftp: bad port %q", ps)
		}
		cfg.Port = port
	}
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}
	p := u.Path
	// RFC 1738 ";type=a|i|d" suffix selects the transfer mode.
	if i := strings.LastIndex(p, ";type="); i >= 0 {
		switch strings.ToLower(p[i+6:]) {
		case "a":
			cfg.TransferMode = TransferASCII
		case "i":
			cfg.TransferMode = TransferBinary
		}
		p = p[:i]
	}
	q := u.Query()
	switch strings.ToLower(q.Get("mode")) {
	case "active":
		cfg.DataMode = Active
	case "passive":
		cfg.DataMode = Passive
	}
	return cfg, p, nil
}

// FormatURL builds the URL shown for a connection and path. The password
// is never included.
func FormatURL(cfg Config, p string) string {
	scheme := "ftp"
	switch cfg.Security {
	case SecurityImplicitTLS:
		scheme = "ftps"
	case SecurityExplicitTLS:
		scheme = "ftpes"
	}
	u := url.URL{Scheme: scheme}
	host := cfg.Host
	defPort := DefaultPort
	if cfg.Security == SecurityImplicitTLS {
		defPort = DefaultImplicitTLSPort
	}
	if cfg.Port > 0 && cfg.Port != defPort {
		host = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	if !cfg.IsAnonymous() {
		u.User = url.User(cfg.User)
	}
	if p != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u.Path = p
	return u.String()
}
