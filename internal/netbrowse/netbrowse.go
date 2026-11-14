// Package netbrowse is the network neighbourhood (board issue #148): it finds file servers on the local network, lists
// a server's shared folders, and opens, maps (mounts) and disconnects them with each system's own mechanism.
//
//   - Finding servers: Bonjour on macOS (dns-sd), Avahi on Linux (avahi-browse), NetBIOS on Windows (net view). A
//     server can always be typed by hand (smb://server, \\server, nfs://server).
//   - Listing shares: SMB through an SMB2/3 client in the process (go-smb2), so a password never goes on a command
//     line; NFS exports through showmount.
//   - Open and map: macOS mounts under /Volumes like Finder's Connect to Server (AppleScript "mount volume", given its
//     script on standard input); Linux mounts through GVfs (gio mount); Windows connects the UNC path or a drive letter
//     (WNetAddConnection2).
//
// Nothing here runs by itself: every function is called only when the user opens the Network view or connects.
package netbrowse

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Server is a file server found on the network or typed by the user.
type Server struct {
	Name   string `json:"name"`   // what to show: the Bonjour or NetBIOS name, or the host
	Host   string `json:"host"`   // what to connect to: a host name or address
	Port   int    `json:"port"`   // 0 = the protocol's default
	Kind   string `json:"kind"`   // "smb", "afp" or "nfs"
	Source string `json:"source"` // "bonjour", "avahi", "netbios", "recent" or "typed"
}

// Share is a shared folder of a server.
type Share struct {
	Name    string `json:"name"`    // the share name, or the export path for NFS
	Comment string `json:"comment"` // who may mount an NFS export, else ""
}

// Target is one share (or a server, when Share is empty) to connect to.
type Target struct {
	Kind  string `json:"kind"` // "smb", "afp" or "nfs"
	Host  string `json:"host"`
	Port  int    `json:"port"`
	User  string `json:"user"`
	Share string `json:"share"` // the share, or the NFS export path ("/volume1/media"); "" = the server only
}

// Creds are the user name and password for a server; empty means guest (SMB) or none (NFS).
type Creds struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Domain   string `json:"domain"`
}

// ErrAuth means the server wants a (different) user name and password.
var ErrAuth = errors.New("the server wants a user name and password")

// ParseTarget reads what the user typed: smb://[user@]host[:port][/share], afp://..., nfs://host/export,
// \\host[\share] or //host[/share], or a bare host[/share] (SMB).
func ParseTarget(s string) (Target, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Target{}, errors.New("type a server, for example smb://nas or \\\\nas")
	}
	if strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, "//") {
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == '\\' || r == '/' })
		if len(parts) == 0 {
			return Target{}, errors.New("no server name after \\\\")
		}
		t := Target{Kind: "smb", Host: parts[0]}
		if len(parts) > 1 {
			t.Share = parts[1]
		}
		return t, checkHost(t)
	}
	if !strings.Contains(s, "://") {
		s = "smb://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return Target{}, errors.New("that is not a server address")
	}
	t := Target{Kind: strings.ToLower(u.Scheme), Host: u.Hostname()}
	switch t.Kind {
	case "smb", "cifs":
		t.Kind = "smb"
	case "afp", "nfs":
	default:
		return Target{}, errors.New("only smb://, afp:// and nfs:// servers can be browsed here")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return Target{}, errors.New("the port is not a number from 1 to 65535")
		}
		t.Port = n
	}
	if u.User != nil {
		t.User = u.User.Username()
	}
	path := strings.Trim(u.Path, "/")
	if t.Kind == "nfs" {
		if path != "" {
			t.Share = "/" + path // an NFS export is a path on the server
		}
	} else if path != "" {
		t.Share, _, _ = strings.Cut(path, "/")
	}
	return t, checkHost(t)
}

// checkHost refuses an empty host or one with characters no host name has (they would end up in mount commands).
func checkHost(t Target) error {
	if t.Host == "" {
		return errors.New("no server name")
	}
	if strings.ContainsAny(t.Host, " \t\"'`$;&|<>\\/@") {
		return errors.New("the server name has characters a host name can't have")
	}
	return nil
}

// URL is the target as a URL without the password: smb://user@host:port/share, nfs://host/export.
func (t Target) URL() string {
	u := url.URL{Scheme: t.Kind, Host: t.Host}
	if strings.Contains(t.Host, ":") && net.ParseIP(t.Host) != nil {
		u.Host = "[" + t.Host + "]"
	}
	if t.Port > 0 {
		u.Host += ":" + strconv.Itoa(t.Port)
	}
	if t.User != "" && t.Kind != "nfs" {
		u.User = url.User(t.User)
	}
	if t.Share != "" {
		u.Path = "/" + strings.TrimPrefix(t.Share, "/")
	}
	return u.String()
}

// UNC is the Windows path of an SMB target: \\host\share.
func (t Target) UNC() string {
	s := `\\` + t.Host
	if t.Share != "" {
		s += `\` + strings.ReplaceAll(strings.TrimPrefix(t.Share, "/"), "/", `\`)
	}
	return s
}

// Key names a server for saved passwords and the recent list: "smb://user@host" (port included when set).
func (t Target) Key() string {
	s := Target{Kind: t.Kind, Host: strings.ToLower(t.Host), Port: t.Port, User: t.User}
	return s.URL()
}

// visibleShare says whether a share is one users open: administrative and hidden shares end in "$" (IPC$, C$).
func visibleShare(name string) bool { return name != "" && !strings.HasSuffix(name, "$") }
