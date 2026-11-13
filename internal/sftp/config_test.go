// Tests for URL parsing (quick connect from the path bar), rendering a
// configuration back to a display URL, and address formatting.
package sftp

import "testing"

func TestParseURL(t *testing.T) {
	tests := []struct {
		in       string
		scheme   string
		host     string
		port     int
		user, pw string
		path     string
		wantErr  bool
	}{
		{in: "sftp://example.org", scheme: "sftp", host: "example.org"},
		{in: "sftp://bob@example.org:2222/var/www", scheme: "sftp", host: "example.org", port: 2222, user: "bob", path: "/var/www"},
		{in: "sftp://bob:p%40ss@example.org/", scheme: "sftp", host: "example.org", user: "bob", pw: "p@ss", path: "/"},
		{in: "SCP://alice@10.0.0.1/~/docs", scheme: "scp", host: "10.0.0.1", user: "alice", path: "docs"},
		{in: "sftp://alice@host/~", scheme: "sftp", host: "host", user: "alice", path: ""},
		{in: "sftp://[::1]:22/tmp", scheme: "sftp", host: "::1", port: 22, path: "/tmp"},
		{in: "ssh://u;fingerprint=ssh-ed25519-abc@h/x", scheme: "sftp", host: "h", user: "u", path: "/x"},
		{in: "  sftp://h/with%20space  ", scheme: "sftp", host: "h", path: "/with space"},
		{in: "ftp://h", wantErr: true},
		{in: "sftp:///nohost", wantErr: true},
		{in: "sftp://h:99999/", wantErr: true},
		{in: "sftp://h:abc/", wantErr: true},
		{in: "://bad", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			p, err := ParseURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", p)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			c := p.Config
			if p.Scheme != tc.scheme || c.Host != tc.host || c.Port != tc.port || c.User != tc.user ||
				c.Password != tc.pw || p.Path != tc.path || c.StartDir != tc.path {
				t.Errorf("got %+v / %+v", p, c)
			}
		})
	}
}

func TestConfigStringAndAddr(t *testing.T) {
	tests := []struct {
		cfg  Config
		str  string
		addr string
	}{
		{Config{Host: "h", User: "u"}, "sftp://u@h", "h:22"},
		{Config{Host: "h", Port: 2200, User: "u", Password: "secret", StartDir: "/srv"}, "sftp://u@h:2200/srv", "h:2200"},
		{Config{Host: "h", StartDir: "docs"}, "sftp://h/~/docs", "h:22"},
		{Config{Host: "::1"}, "sftp://[::1]", "[::1]:22"},
		{Config{Host: "::1", Port: 2222}, "sftp://[::1]:2222", "[::1]:2222"},
	}
	for _, tc := range tests {
		if got := tc.cfg.String(); got != tc.str {
			t.Errorf("String() = %q, want %q", got, tc.str)
		}
		if got := tc.cfg.Addr(); got != tc.addr {
			t.Errorf("Addr() = %q, want %q", got, tc.addr)
		}
	}
	// Round trip through ParseURL.
	cfg := Config{Host: "h", Port: 2200, User: "u", StartDir: "docs"}
	p, err := ParseURL(cfg.String())
	if err != nil || p.Config.Host != "h" || p.Config.Port != 2200 || p.Config.User != "u" || p.Path != "docs" {
		t.Errorf("round trip: %+v %v", p, err)
	}
}
