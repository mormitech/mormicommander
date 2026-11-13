// This file tests quick-connect URLs: which security mode, port, user,
// password, path and transfer mode each URL form produces, that invalid
// URLs are rejected, and that FormatURL never reveals the password.
package ftp

import (
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		in       string
		ok       bool
		host     string
		port     int
		user     string
		pass     string
		sec      Security
		path     string
		mode     TransferMode
		dataMode DataMode
	}{
		{"ftp://ftp.example.com", true, "ftp.example.com", 0, "", "", SecurityNone, "", TransferDefault, Passive},
		{"ftp://ftp.example.com/", true, "ftp.example.com", 0, "", "", SecurityNone, "/", TransferDefault, Passive},
		{"ftp://user:p%40ss@host:2121/pub/dir", true, "host", 2121, "user", "p@ss", SecurityNone, "/pub/dir", TransferDefault, Passive},
		{"host.only/some%20dir", true, "host.only", 0, "", "", SecurityNone, "/some dir", TransferDefault, Passive},
		{"ftps://secure.example.com/x", true, "secure.example.com", 0, "", "", SecurityImplicitTLS, "/x", TransferDefault, Passive},
		{"FTPES://me@secure.example.com:21", true, "secure.example.com", 21, "me", "", SecurityExplicitTLS, "", TransferDefault, Passive},
		{"ftp://[::1]:2100/v6", true, "::1", 2100, "", "", SecurityNone, "/v6", TransferDefault, Passive},
		{"ftp://h/file.txt;type=a", true, "h", 0, "", "", SecurityNone, "/file.txt", TransferASCII, Passive},
		{"ftp://h/file.bin;type=i", true, "h", 0, "", "", SecurityNone, "/file.bin", TransferBinary, Passive},
		{"ftp://h/?mode=active", true, "h", 0, "", "", SecurityNone, "/", TransferDefault, Active},
		{"", false, "", 0, "", "", 0, "", 0, 0},
		{"http://example.com", false, "", 0, "", "", 0, "", 0, 0},
		{"ftp://", false, "", 0, "", "", 0, "", 0, 0},
		{"ftp://h:99999/", false, "", 0, "", "", 0, "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			cfg, p, err := ParseURL(tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
			if !tt.ok {
				return
			}
			if cfg.Host != tt.host || cfg.Port != tt.port || cfg.User != tt.user || cfg.Password != tt.pass ||
				cfg.Security != tt.sec || p != tt.path || cfg.TransferMode != tt.mode || cfg.DataMode != tt.dataMode {
				t.Fatalf("got %+v path %q", cfg, p)
			}
		})
	}
}

func TestFormatURL(t *testing.T) {
	tests := []struct {
		cfg  Config
		path string
		want string
	}{
		{Config{Host: "h"}, "/pub", "ftp://h/pub"},
		{Config{Host: "h", Port: 21, User: "anonymous"}, "", "ftp://h"},
		{Config{Host: "h", Port: 2121, User: "peter", Password: "secret"}, "dir/x y", "ftp://peter@h:2121/dir/x%20y"},
		{Config{Host: "h", Security: SecurityImplicitTLS, Port: 990}, "/", "ftps://h/"},
		{Config{Host: "h", Security: SecurityExplicitTLS}, "/", "ftpes://h/"},
		{Config{Host: "::1", Port: 21}, "/", "ftp://[::1]/"},
	}
	for _, tt := range tests {
		got := FormatURL(tt.cfg, tt.path)
		if got != tt.want {
			t.Errorf("FormatURL(%+v, %q) = %q, want %q", tt.cfg, tt.path, got, tt.want)
		}
		if tt.cfg.Password != "" && strings.Contains(got, tt.cfg.Password) {
			t.Errorf("password leaked: %s", got)
		}
		// Round trip.
		cfg, p, err := ParseURL(got)
		if err != nil || cfg.Host != tt.cfg.Host || (p != tt.path && p != "/"+tt.path) {
			t.Errorf("round trip of %q: %+v %q %v", got, cfg, p, err)
		}
	}
	for _, s := range []string{"ftp://x", "FTPS://x", " ftpes://x"} {
		if !IsURL(s) {
			t.Errorf("IsURL(%q) = false", s)
		}
	}
	if IsURL("/usr/local") || IsURL("sftp://x") {
		t.Error("IsURL accepts non-FTP text")
	}
}

func TestConfigHelpers(t *testing.T) {
	if (Config{Host: "h"}).Address() != "h:21" || (Config{Host: "h", Security: SecurityImplicitTLS}).Address() != "h:990" {
		t.Error("default ports wrong")
	}
	for _, s := range []string{"", "anonymous", "FTP"} {
		if !(Config{User: s}).IsAnonymous() {
			t.Errorf("%q not anonymous", s)
		}
	}
	u, p := (Config{}).loginUser()
	if u != "anonymous" || p != AnonymousPassword {
		t.Errorf("anonymous login = %q %q", u, p)
	}
	for _, m := range []TransferMode{TransferAuto, TransferBinary, TransferASCII} {
		if ParseTransferMode(m.String()) != m {
			t.Errorf("transfer mode %v round trip", m)
		}
	}
	for _, e := range []Encoding{EncodingAuto, EncodingUTF8, EncodingLatin1} {
		if ParseEncoding(e.String()) != e {
			t.Errorf("encoding %v round trip", e)
		}
	}
	if SecurityExplicitTLS.String() != "explicit-tls" || Active.String() != "active" || Passive.String() != "passive" {
		t.Error("String methods wrong")
	}
}
