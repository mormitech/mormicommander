// This file describes how to reach an FTP server: the Config struct that the
// connection dialog, the connection manager and quick-connect URLs all fill
// in, plus the small enums for the choices Total Commander offers in its
// connection settings (TLS mode, passive or active data connections,
// transfer mode, file name encoding). Every field has a sensible zero value,
// so a Config with only Host set connects anonymously in passive binary
// mode, exactly like typing "ftp://host" into Total Commander.
package ftp

import (
	"crypto/tls"
	"net"
	"strconv"
	"strings"
	"time"
)

// Security selects whether and how the control and data connections are
// encrypted with TLS (FTPS).
type Security int

const (
	// SecurityNone is plain, unencrypted FTP.
	SecurityNone Security = iota
	// SecurityExplicitTLS connects in plain text and then upgrades with
	// AUTH TLS (RFC 4217); the connection fails if the server refuses.
	SecurityExplicitTLS
	// SecurityImplicitTLS speaks TLS from the first byte, traditionally on
	// port 990 ("ftps://" URLs).
	SecurityImplicitTLS
	// SecurityExplicitTLSIfAvailable tries AUTH TLS but quietly continues
	// in plain text when the server does not support it.
	SecurityExplicitTLSIfAvailable
)

// String returns a short name for logs and settings files.
func (s Security) String() string {
	switch s {
	case SecurityExplicitTLS:
		return "explicit-tls"
	case SecurityImplicitTLS:
		return "implicit-tls"
	case SecurityExplicitTLSIfAvailable:
		return "explicit-tls-if-available"
	}
	return "none"
}

// DataMode selects who opens the data connection for listings and
// transfers.
type DataMode int

const (
	// Passive (PASV/EPSV) lets the client connect to the server; it works
	// through NAT routers and is Total Commander's default.
	Passive DataMode = iota
	// Active (PORT/EPRT) makes the server connect back to the client.
	Active
)

// String returns "passive" or "active".
func (m DataMode) String() string {
	if m == Active {
		return "active"
	}
	return "passive"
}

// TransferMode selects binary (TYPE I) or ASCII (TYPE A) transfers.
type TransferMode int

const (
	// TransferDefault means "use the connection's setting"; in a Config it
	// behaves like TransferAuto.
	TransferDefault TransferMode = iota
	// TransferAuto uses ASCII for names matching Config.ASCIIMasks and
	// binary for everything else, like Total Commander's "Auto" button.
	TransferAuto
	// TransferBinary copies bytes unchanged.
	TransferBinary
	// TransferASCII converts line endings between the server (CRLF on the
	// wire) and the local system.
	TransferASCII
)

// String returns "default", "auto", "binary" or "ascii".
func (m TransferMode) String() string {
	switch m {
	case TransferAuto:
		return "auto"
	case TransferBinary:
		return "binary"
	case TransferASCII:
		return "ascii"
	}
	return "default"
}

// ParseTransferMode is the inverse of TransferMode.String; unknown text
// gives TransferDefault.
func ParseTransferMode(s string) TransferMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto":
		return TransferAuto
	case "binary", "bin", "i", "image":
		return TransferBinary
	case "ascii", "a", "text":
		return TransferASCII
	}
	return TransferDefault
}

// Encoding selects how file names are encoded on the wire (Total
// Commander's "codepage" setting).
type Encoding int

const (
	// EncodingAuto uses UTF-8 and sends OPTS UTF8 ON when the server
	// advertises UTF8 in FEAT.
	EncodingAuto Encoding = iota
	// EncodingUTF8 always treats names as UTF-8.
	EncodingUTF8
	// EncodingLatin1 treats names as ISO-8859-1 bytes, for old servers
	// that store names in a Western European code page.
	EncodingLatin1
)

// String returns "auto", "utf-8" or "latin1".
func (e Encoding) String() string {
	switch e {
	case EncodingUTF8:
		return "utf-8"
	case EncodingLatin1:
		return "latin1"
	}
	return "auto"
}

// ParseEncoding is the inverse of Encoding.String.
func ParseEncoding(s string) Encoding {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "utf-8", "utf8":
		return EncodingUTF8
	case "latin1", "latin-1", "iso-8859-1", "local", "ansi":
		return EncodingLatin1
	}
	return EncodingAuto
}

// PassiveHost decides which address the client dials after PASV. Servers
// behind NAT often announce their private address; the control
// connection's address is then the one that actually works.
type PassiveHost int

const (
	// PassiveHostAuto replaces an announced address that cannot be right
	// (0.0.0.0, a private address while the server itself is public, or
	// 127.0.0.1 while the server is another computer).
	PassiveHostAuto PassiveHost = iota
	// PassiveHostServer always dials the address the server announced.
	PassiveHostServer
	// PassiveHostControl always dials the control connection's host and
	// only takes the port from the PASV reply.
	PassiveHostControl
)

// Default ports and the password sent for anonymous logins.
const (
	DefaultPort            = 21
	DefaultImplicitTLSPort = 990
	AnonymousPassword      = "mormicommander@"
	defaultTimeout         = 30 * time.Second
)

// DefaultASCIIMasks are the file name patterns transferred in ASCII mode
// when the transfer mode is TransferAuto and Config.ASCIIMasks is nil.
var DefaultASCIIMasks = []string{
	"*.txt", "*.htm", "*.html", "*.shtml", "*.php", "*.cgi", "*.pl", "*.py",
	"*.sh", "*.js", "*.css", "*.xml", "*.csv", "*.ini", "*.conf", "*.cfg",
	"*.md", "*.sql", "*.bat", "*.cmd", "*.asp", "*.jsp",
}

// Config holds everything needed to open one FTP connection.
type Config struct {
	Host string // host name or IP address, no port
	Port int    // 0 means 21, or 990 for implicit TLS

	// User and Password; an empty User, "anonymous" or "ftp" logs in
	// anonymously, using AnonymousPassword when Password is empty.
	User     string
	Password string
	Account  string // sent with ACCT when the server asks for one (332)

	Security  Security
	TLSConfig *tls.Config // optional; ServerName and a session cache are filled in

	DataMode      DataMode
	PassiveHost   PassiveHost
	DisableEPSV   bool   // never try EPSV, go straight to PASV
	ActiveIP      string // address announced in PORT (for NAT); default: our control address
	ActivePortMin int    // optional port range for active mode listeners
	ActivePortMax int

	DisableMLSD bool   // use LIST even when the server offers MLSD
	ListCommand string // default "LIST"; Total Commander lets users set e.g. "LIST -la"

	TransferMode TransferMode
	ASCIIMasks   []string // nil means DefaultASCIIMasks

	Encoding Encoding

	// ServerLocation is the time zone the server's LIST timestamps are in
	// (default: the local zone, as Total Commander assumes); TimeOffset is
	// the extra user-chosen correction added to them.
	ServerLocation *time.Location
	TimeOffset     time.Duration

	Timeout time.Duration // connect and per-reply timeout; default 30s
	// Retries is how many more times a connection that could not be made is tried, RetryDelay apart (0: once only).
	Retries    int
	RetryDelay time.Duration
	Keepalive  time.Duration // send NOOP after this much idle time; 0 = off

	Proxy *Proxy // nil means a direct connection

	InitialDir string // changed into right after login

	// Logf, when set, receives the protocol log (commands and replies),
	// with passwords masked, for Total Commander's FTP log window.
	Logf func(format string, args ...any)
}

// IsAnonymous reports whether the login will be anonymous.
func (c Config) IsAnonymous() bool {
	u := strings.ToLower(strings.TrimSpace(c.User))
	return u == "" || u == "anonymous" || u == "ftp"
}

// EffectivePort is the port that will be dialled.
func (c Config) EffectivePort() int {
	if c.Port > 0 {
		return c.Port
	}
	if c.Security == SecurityImplicitTLS {
		return DefaultImplicitTLSPort
	}
	return DefaultPort
}

// Address is "host:port" for the server.
func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.EffectivePort()))
}

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if strings.TrimSpace(c.ListCommand) == "" {
		c.ListCommand = "LIST"
	}
	if c.ServerLocation == nil {
		c.ServerLocation = time.Local
	}
	if c.TransferMode == TransferDefault {
		c.TransferMode = TransferAuto
	}
	if c.ASCIIMasks == nil {
		c.ASCIIMasks = DefaultASCIIMasks
	}
	return c
}

func (c Config) loginUser() (user, pass string) {
	if c.IsAnonymous() {
		user = "anonymous"
		if c.User != "" {
			user = c.User
		}
		pass = c.Password
		if pass == "" {
			pass = AnonymousPassword
		}
		return user, pass
	}
	return c.User, c.Password
}
