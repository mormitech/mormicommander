// This file defines what a connection needs to know before it dials: the
// host, the user, every way the user might authenticate, how the server's
// identity is checked against known_hosts, optional proxy and keep-alive
// settings. It also parses sftp:// and scp:// URLs, because Total Commander
// lets the user type such a URL straight into the path bar or command line to
// open a quick connection without visiting the connection manager first.
package sftp

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"mormicommander/internal/remotefs"
)

// DefaultPort is the standard SSH port.
const DefaultPort = 22

// Config describes one SFTP/SCP connection. The zero value of every optional
// field means "use the sensible default"; at minimum Host and User must be set
// and some way of authenticating must be possible.
type Config struct {
	Host string // host name or IP address
	Port int    // TCP port, DefaultPort when 0
	User string // login name

	// Password is offered via the "password" method and also answers
	// keyboard-interactive prompts that ask for a password, which is how many
	// servers (PAM based) actually ask for it.
	Password string

	// KeyFiles lists private key files to try, in order (OpenSSH, PEM or
	// PKCS#8 format). When KeyFiles, KeyData and the agent are all unused and
	// DefaultKeys is true, the usual ~/.ssh/id_* files are tried.
	KeyFiles []string
	// KeyData holds private keys already loaded into memory, for example from
	// the encrypted connection store.
	KeyData [][]byte
	// Passphrase decrypts encrypted private keys. When it is empty or wrong
	// and PassphraseFunc is set, PassphraseFunc is asked instead (the UI shows
	// Total Commander's "enter passphrase for key" prompt there).
	Passphrase string
	// PassphraseFunc is asked for the passphrase of an encrypted key. keyName
	// is the file name, or "key #n" for KeyData entries. Returning an error
	// skips the key.
	PassphraseFunc func(keyName string) (string, error)
	// DefaultKeys tries ~/.ssh/id_ed25519, id_ecdsa and id_rsa when they exist.
	DefaultKeys bool

	// UseAgent enables authentication through a running SSH agent (ssh-agent,
	// the Windows OpenSSH agent service, or a compatible agent such as the
	// 1Password or KeePassXC agents). AgentSocket overrides the location;
	// by default $SSH_AUTH_SOCK is used on Unix and the
	// \\.\pipe\openssh-ssh-agent named pipe on Windows.
	UseAgent    bool
	AgentSocket string
	// Agent lets the caller hand in an already connected agent (mostly for
	// tests, or for an agent the app runs itself). It implies UseAgent.
	Agent agent.Agent

	// KeyboardInteractive answers keyboard-interactive challenges that are
	// not plain password prompts (one-time codes, "Verification code:" and so
	// on). When nil, only password-looking prompts are answered, using
	// Password.
	KeyboardInteractive func(name, instruction string, questions []string, echos []bool) ([]string, error)

	// PasswordFunc is asked for the password when Password is empty and the
	// server wants the plain "password" method. It is offered last, after the
	// agent, the keys and keyboard-interactive, so it is only called when
	// nothing else logged in (like OpenSSH asking "bob@host's password:").
	// attempt counts from 0; a refused password is asked again up to three
	// times in all. Returning an error stops the login with that error.
	PasswordFunc func(attempt int) (string, error)

	// KnownHostsFiles are the known_hosts files to check the server key
	// against. The first file is the one new keys are written to. When empty,
	// ~/.ssh/known_hosts is used.
	KnownHostsFiles []string
	// HostKeyPrompt decides what to do with a server key that is not known
	// yet or that changed. When nil, unknown and changed keys are rejected,
	// which is the safe behaviour for unattended connections.
	HostKeyPrompt HostKeyPrompt
	// HashKnownHosts writes new known_hosts entries with hashed host names,
	// like OpenSSH's HashKnownHosts option.
	HashKnownHosts bool
	// InsecureIgnoreHostKey disables host key checking entirely. It exists
	// only for throw-away test servers and must never be the default in UI.
	InsecureIgnoreHostKey bool

	// Timeout limits TCP connect plus SSH handshake and login. Default 30s.
	Timeout time.Duration
	// KeepAlive sends an SSH keep-alive request at this interval so that
	// NAT routers and firewalls do not drop an idle connection, and so that a
	// dead connection is noticed (Total Commander's "send keepalive" option).
	// Zero disables it.
	KeepAlive time.Duration
	// IdleTimeout disconnects automatically after this long without any
	// operation (Total Commander's "disconnect when idle"). Zero disables it.
	IdleTimeout time.Duration

	// Proxy, when set, tunnels the TCP connection through a SOCKS4/5 or HTTP
	// CONNECT proxy.
	Proxy *Proxy
	// Dial replaces the network dialer entirely (tests, custom tunnels). When
	// set, Proxy is ignored.
	Dial func(network, addr string) (net.Conn, error)

	// StartDir is the folder the panel shows after connecting. Relative
	// paths are relative to the login folder. Empty means the login folder.
	StartDir string

	// ClientVersion overrides the SSH identification string.
	ClientVersion string
}

// Addr returns host:port for the configuration, using DefaultPort when Port
// is zero. IPv6 literals are bracketed correctly.
func (c *Config) Addr() string {
	port := c.Port
	if port == 0 {
		port = DefaultPort
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(port))
}

func (c *Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 30 * time.Second
}

func (c *Config) knownHostsFiles() []string {
	if len(c.KnownHostsFiles) > 0 {
		return c.KnownHostsFiles
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, ".ssh", "known_hosts")}
}

func (c *Config) validate() error {
	if c.Host == "" {
		return errors.New("sftp: no host given")
	}
	if c.User == "" {
		return errors.New("sftp: no user name given")
	}
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("sftp: invalid port %d", c.Port)
	}
	return nil
}

func (c *Config) clientVersion() string {
	if c.ClientVersion != "" {
		return c.ClientVersion
	}
	return "SSH-2.0-mormiCommander"
}

// sshConfig builds the x/crypto/ssh client configuration (minus the host key
// callback, which Dial adds because it needs per-connection state).
func (c *Config) sshConfig() (*ssh.ClientConfig, func(), error) {
	auths, cleanup, err := c.authMethods()
	if err != nil {
		return nil, nil, err
	}
	return &ssh.ClientConfig{
		User:          c.User,
		Auth:          auths,
		Timeout:       c.timeout(),
		ClientVersion: c.clientVersion(),
	}, cleanup, nil
}

// ParsedURL is the result of ParseURL: a connection configuration plus the
// remote path the user wants to open and the scheme they typed.
type ParsedURL struct {
	Scheme string // "sftp" or "scp"
	Config Config
	Path   string // remote path, "" when the URL had none
}

// ParseURL parses sftp://[user[:password]@]host[:port][/path] (and the same
// with scp://), the forms Total Commander accepts in the path bar. A missing
// user is left empty so the caller can fill in the current login name or ask.
// Following the URI draft for SSH, a path starting with "/~/" is made relative
// to the home folder by returning it without the leading slash.
func ParseURL(raw string) (ParsedURL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ParsedURL{}, fmt.Errorf("sftp: bad URL: %w", remotefs.HideURLErrorPassword(err))
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "sftp" && scheme != "scp" && scheme != "ssh" {
		return ParsedURL{}, fmt.Errorf("sftp: unsupported URL scheme %q", u.Scheme)
	}
	if scheme == "ssh" {
		scheme = "sftp"
	}
	if u.Hostname() == "" {
		return ParsedURL{}, errors.New("sftp: URL has no host")
	}
	p := ParsedURL{Scheme: scheme}
	p.Config.Host = u.Hostname()
	if ps := u.Port(); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n <= 0 || n > 65535 {
			return ParsedURL{}, fmt.Errorf("sftp: bad port %q", ps)
		}
		p.Config.Port = n
	}
	if u.User != nil {
		user := u.User.Username()
		// The SSH URI draft allows ";fingerprint=..." connection parameters
		// after the user name; they are not used here, so drop them.
		if i := strings.IndexByte(user, ';'); i >= 0 {
			user = user[:i]
		}
		p.Config.User = user
		if pw, ok := u.User.Password(); ok {
			p.Config.Password = pw
		}
	}
	switch {
	case u.Path == "":
		p.Path = ""
	case strings.HasPrefix(u.Path, "/~/"):
		p.Path = u.Path[3:]
	case u.Path == "/~":
		p.Path = ""
	default:
		p.Path = u.Path
	}
	p.Config.StartDir = p.Path
	return p, nil
}

// String renders the configuration back as an sftp:// URL without the
// password, suitable for a window title, history list or tab caption.
func (c *Config) String() string {
	u := url.URL{Scheme: "sftp", Host: c.Host}
	if c.Port != 0 && c.Port != DefaultPort {
		u.Host = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	} else if strings.Contains(c.Host, ":") {
		u.Host = "[" + c.Host + "]"
	}
	if c.User != "" {
		u.User = url.User(c.User)
	}
	if c.StartDir != "" {
		if strings.HasPrefix(c.StartDir, "/") {
			u.Path = c.StartDir
		} else {
			u.Path = "/~/" + c.StartDir
		}
	}
	return u.String()
}
