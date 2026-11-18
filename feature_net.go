package main

// The Net menu (FEATURES.md sections 15 and 30): FTP, FTPS, SFTP and WebDAV servers in a panel. The panels already
// show any URL through the vfs router (FS.List → vfs.For → vfs.Router → FTPFS / SFTPFS); this feature is the
// connection layer on top of it:
//
//   - it makes sure a router serves the server schemes (it uses the router another feature registered, or registers
//     one of its own) and plugs WebDAV in as a file system of its own (feature_net_webdav.go);
//   - its Configure hooks fill in what a URL cannot carry: the saved site's settings (TLS, passive mode, transfer mode,
//     charset, list command, proxy, SSH key), the global FTP options, a password typed once (kept in memory for the
//     session, so an automatic reconnect never asks again), and the questions only the user can answer: password,
//     key passphrase, keyboard-interactive codes and the SFTP host key (feature_net_ask.go);
//   - it keeps the connection manager's list (feature_net_sites.go), the protocol log, the per-session transfer
//     mode, and runs the transfers to and from servers with resume (feature_net_xfer.go).
//
// The page side is frontend/src/features/net.js.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"mormicommander/internal/ftp"
	"mormicommander/internal/keychain"
	"mormicommander/internal/sftp"
	"mormicommander/internal/vfs"
)

// Net is what the page calls for server connections (window.go.main.Net).
type Net struct {
	app *App

	mu     sync.Mutex
	own    *vfs.Router // a router this feature made, when no other feature registered one
	hooked *vfs.Router // the router whose Configure hooks are ours
	dav    *netDAV

	store     *ftp.Store
	storePath string
	key       *ftp.MasterKey // the unlocked master password, for this run only

	kc           keychain.Keychain                 // the system keychain (feature_net_keychain.go), once opened
	kcErr        error                             // why there is none
	kcOpened     bool                              // kc and kcErr are known
	openKeychain func() (keychain.Keychain, error) // tests put a fake keychain here; nil = keychain.Open
	askNotes     map[string]string                 // session key → why the password is asked (said once)

	sites     map[string]ftp.Site         // session key → the saved site it was connected from
	passwords map[string]string           // session key → password typed or saved (memory only)
	modes     map[string]ftp.TransferMode // session key → transfer mode chosen with Ctrl+Shift+M
	logs      map[string]*netLog          // session key → protocol log

	asks    map[int]chan map[string]any
	askSeq  int
	asker   func(kind string, data map[string]any) (map[string]any, bool) // tests answer questions here
	connCtx context.Context                                               // the connection being made (Cancel stops it)
	cancel  context.CancelFunc

	xfers    map[int]*netXfer
	xferDone map[int]NetJobProgress // finished transfers, by number
	xferID   int

	stopIdle chan struct{}
	tempDir  string // downloads opened with their app (OpenCopy)
}

// netOptions are the global FTP options (settings key "net", written by the page: Configuration > FTP).
type netOptions struct {
	ShowHidden        bool   `json:"showHidden"`        // send LIST -a (F-380)
	TransferMode      string `json:"transferMode"`      // "auto" (default), "binary", "ascii"
	Active            bool   `json:"active"`            // active instead of passive mode by default
	Keepalive         int    `json:"keepalive"`         // seconds between NOOPs, 0 = off
	IdleMinutes       int    `json:"idleMinutes"`       // close unused connections after this many minutes (default 10)
	AnonymousPassword string `json:"anonymousPassword"` // the e-mail address sent for anonymous logins
}

var theNet *Net // the one Net object (control commands reach it here)

func init() {
	registerFeature("net", func(a *App) []exposed {
		n := newNet(a)
		theNet = n
		return []exposed{expose(n)}
	})
	onStartup(func(a *App) {
		if theNet != nil && a.mode == "" {
			theNet.router()
			theNet.startIdle()
		}
	})
	onShutdown(func(a *App) {
		if theNet != nil {
			theNet.close()
		}
	})
	registerControl("net", func(a *App, m map[string]any) (map[string]any, error) {
		if theNet == nil {
			return nil, errors.New("the network feature is not loaded")
		}
		sites, err := theNet.Sites()
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessions": theNet.Sessions(), "sites": sites, "options": theNet.options()}, nil
	})
}

func newNet(a *App) *Net {
	n := &Net{app: a, sites: map[string]ftp.Site{}, passwords: map[string]string{}, modes: map[string]ftp.TransferMode{},
		logs: map[string]*netLog{}, askNotes: map[string]string{}, asks: map[int]chan map[string]any{}, xfers: map[int]*netXfer{}}
	n.dav = newNetDAV(n.configureDAV)
	for _, s := range []string{"webdav", "webdavs", "dav", "davs"} {
		vfs.Register(s, n.dav)
	}
	if dir := configDir(); dir != "" {
		n.storePath = filepath.Join(dir, "ftpsites.json")
	}
	return n
}

// router is the vfs router that serves server URLs: the one some feature registered for "ftp://", else one of our
// own. Our Configure hooks are put on it once (after any hook that was there before).
func (n *Net) router() *vfs.Router {
	n.mu.Lock()
	defer n.mu.Unlock()
	r, _ := vfs.For("ftp://x", nil).(*vfs.Router)
	if r == nil {
		if n.own == nil {
			n.own = vfs.NewRouter()
			n.own.RegisterSchemes()
		}
		r = n.own
	}
	if r != n.hooked {
		n.hooked = r
		if r.FTP != nil {
			prev := r.FTP.Configure
			r.FTP.Configure = func(c *ftp.Config) error {
				if prev != nil {
					if err := prev(c); err != nil {
						return err
					}
				}
				return n.configureFTP(c)
			}
			r.FTP.MaxConnsFunc = netMaxConns
		}
		if r.SFTP != nil {
			prev := r.SFTP.Configure
			r.SFTP.Configure = func(c *sftp.Config) error {
				if prev != nil {
					if err := prev(c); err != nil {
						return err
					}
				}
				return n.configureSFTP(c)
			}
		}
	}
	return r
}

func (n *Net) close() {
	n.mu.Lock()
	if n.stopIdle != nil {
		close(n.stopIdle)
		n.stopIdle = nil
	}
	for _, x := range n.xfers {
		x.cancel()
	}
	own := n.own
	n.mu.Unlock()
	if own != nil {
		own.Close() // F-650: disconnect on exit
	}
	n.dav.Close()
	n.mu.Lock()
	if n.tempDir != "" {
		os.RemoveAll(n.tempDir)
		n.tempDir = ""
	}
	n.mu.Unlock()
}

// startIdle closes pooled FTP connections nobody used for a while (F-650); the session itself stays and dials again
// when the panel needs it.
func (n *Net) startIdle() {
	n.mu.Lock()
	if n.stopIdle != nil {
		n.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	n.stopIdle = stop
	n.mu.Unlock()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				m := netIdleMinutes(n.options())
				if r := n.router(); r.FTP != nil {
					r.FTP.CloseIdle(time.Duration(m) * time.Minute)
				}
			}
		}
	}()
}

// options reads the global FTP options.
func (n *Net) options() netOptions {
	o := netOptions{}
	if n.app == nil || n.app.settings == nil {
		return o
	}
	if v := n.app.settings.Get("net"); v != nil {
		if b, err := json.Marshal(v); err == nil {
			json.Unmarshal(b, &o)
		}
	}
	return o
}

// ---- Configure hooks ----------------------------------------------------------------------------------------------

// ftpKey is the session key of an FTP configuration as the vfs adapter builds it from a URL.
func ftpKey(c *ftp.Config) string {
	u := netURL{Scheme: "ftp", User: c.User, Host: strings.ToLower(c.Host), Port: c.Port}
	switch c.Security {
	case ftp.SecurityImplicitTLS:
		u.Scheme = "ftps"
	case ftp.SecurityExplicitTLS:
		u.Scheme = "ftpes"
	}
	if u.Port == netDefaultPorts[u.Scheme] {
		u.Port = 0
	}
	return u.key()
}

// configureFTP fills in an FTP configuration before its session is dialled.
func (n *Net) configureFTP(c *ftp.Config) error {
	key := ftpKey(c)
	o := n.options()
	// Options > FTP and Options > Proxy (board issue #94): the defaults of every connection; a saved site's own
	// settings below still win
	if s, ok := currentSettings(); ok {
		if !s.FTP.Passive {
			o.Active = true
		}
		o.ShowHidden = o.ShowHidden || s.FTP.ShowHidden
		if o.Keepalive == 0 && s.FTP.KeepAliveSec > 0 {
			o.Keepalive = s.FTP.KeepAliveSec
		}
		if o.AnonymousPassword == "" && strings.TrimSpace(s.FTP.AnonymousEmail) != "" {
			o.AnonymousPassword = strings.TrimSpace(s.FTP.AnonymousEmail)
		}
		if s.FTP.TimeoutSec > 0 {
			c.Timeout = time.Duration(s.FTP.TimeoutSec) * time.Second
		}
		c.Retries, c.RetryDelay = s.FTP.Retries, time.Duration(s.FTP.RetryDelaySec)*time.Second
		if m := strings.Fields(strings.ReplaceAll(s.FTP.AsciiMasks, ";", " ")); len(m) > 0 && len(c.ASCIIMasks) == 0 {
			c.ASCIIMasks = m
		}
		if p := netGlobalProxy(s.Proxy.Type, s.Proxy.Host, s.Proxy.Port, s.Proxy.User); p != nil && c.Proxy == nil {
			// the proxy's password is not kept in the settings file: it is asked once per session (F-637)
			pw, err := n.proxyPassword(p.User, p.Address())
			if err != nil {
				return err
			}
			p.Password = pw
			c.Proxy = p
		}
	}
	if o.Active {
		c.DataMode = ftp.Active
	}
	if m := ftp.ParseTransferMode(o.TransferMode); m != ftp.TransferDefault && c.TransferMode == ftp.TransferDefault {
		c.TransferMode = m
	}
	if o.Keepalive > 0 {
		c.Keepalive = time.Duration(o.Keepalive) * time.Second
	}
	if o.ShowHidden && c.ListCommand == "" {
		c.ListCommand = "LIST -a"
	}
	n.mu.Lock()
	site, hasSite := n.sites[key]
	pw, hasPw := n.passwords[key]
	mode, hasMode := n.modes[key]
	n.mu.Unlock()
	if hasSite {
		sc, err := site.Config()
		if err != nil {
			return err
		}
		c.Security, c.DataMode = sc.Security, sc.DataMode
		if sc.TransferMode != ftp.TransferDefault {
			c.TransferMode = sc.TransferMode
		}
		c.ASCIIMasks, c.Encoding, c.TimeOffset = sc.ASCIIMasks, sc.Encoding, sc.TimeOffset
		if sc.Keepalive > 0 {
			c.Keepalive = sc.Keepalive
		}
		if sc.ListCommand != "" {
			c.ListCommand = sc.ListCommand
		}
		c.DisableMLSD, c.DisableEPSV = sc.DisableMLSD, sc.DisableEPSV
		c.InitialDir = sc.InitialDir
		if site.ProxyName != "" {
			box, err := n.unlockIfNeeded()
			if err != nil {
				return err
			}
			st, err := n.loadStore()
			if err != nil {
				return err
			}
			if c.Proxy, err = st.ProxyByName(site.ProxyName, box); err != nil {
				return err
			}
		}
	}
	if hasMode {
		c.TransferMode = mode
	}
	if c.Password == "" && hasPw {
		c.Password = pw
	}
	if c.Password == "" && c.IsAnonymous() && o.AnonymousPassword != "" {
		c.Password = o.AnonymousPassword
	}
	if c.Password == "" && !c.IsAnonymous() {
		p, ok := n.askPassword(fmt.Sprintf("Password for %s on %s", c.User, c.Host), n.askNote(key))
		if !ok {
			return errNetCancelled
		}
		c.Password = p
	}
	if c.Password != "" && !c.IsAnonymous() {
		n.mu.Lock()
		n.passwords[key] = c.Password
		n.mu.Unlock()
	}
	c.Logf = n.logFor(key).printf
	return nil
}

// configureSFTP fills in an SFTP configuration before its session is dialled.
func (n *Net) configureSFTP(c *sftp.Config) error {
	u := netURL{Scheme: "sftp", User: c.User, Host: strings.ToLower(c.Host), Port: c.Port}
	if u.Port == 22 {
		u.Port = 0
	}
	key := u.key()
	n.mu.Lock()
	site, hasSite := n.sites[key]
	pw, hasPw := n.passwords[key]
	n.mu.Unlock()
	o := n.options()
	c.UseAgent = true
	c.DefaultKeys = true
	c.KnownHostsFiles = netKnownHosts()
	c.HostKeyPrompt = n.askHostKey
	c.PassphraseFunc = func(name string) (string, error) {
		p, ok := n.askPassword("Passphrase for the key "+name, "The SSH key is protected by a passphrase.")
		if !ok {
			return "", errNetCancelled
		}
		return p, nil
	}
	c.KeyboardInteractive = n.askInteractive(c.User, c.Host)
	// A server that only takes a plain password, reached without one (an sftp:// URL typed in the path bar or after
	// "cd"), asks for it once the agent and the keys did not log in, like OpenSSH does; a refused one is asked again.
	// The password is remembered for the session, so a reconnect does not ask again (F-632, F-636).
	c.PasswordFunc = func(attempt int) (string, error) {
		msg := netRetryMessage(attempt > 0)
		if attempt == 0 {
			msg = n.askNote(key)
		}
		pw, ok := n.askPassword(fmt.Sprintf("Password for %s on %s", c.User, c.Host), msg)
		if !ok {
			return "", errNetCancelled
		}
		n.mu.Lock()
		n.passwords[key] = pw
		n.mu.Unlock()
		return pw, nil
	}
	if o.Keepalive > 0 {
		c.KeepAlive = time.Duration(o.Keepalive) * time.Second
	}
	// Options > Proxy with "Use the proxy for SFTP connections too" (off by default, F-637)
	if s, ok := currentSettings(); ok && s.Proxy.ForSFTP && c.Proxy == nil && c.Dial == nil {
		if p := netSFTPProxy(s.Proxy.Type, s.Proxy.Host, s.Proxy.Port, s.Proxy.User); p != nil {
			pw, err := n.proxyPassword(p.User, p.Addr)
			if err != nil {
				return err
			}
			p.Password = pw
			c.Proxy = p
		}
	}
	if hasSite {
		if site.KeyFile != "" {
			c.KeyFiles = []string{netExpandHome(site.KeyFile)}
		}
		if extra := n.siteExtra(site.ID); extra.NoAgent {
			c.UseAgent = false
		}
		if site.Keepalive > 0 {
			c.KeepAlive = time.Duration(site.Keepalive) * time.Second
		}
		c.StartDir = site.RemoteDir
	}
	if c.Password == "" && hasPw {
		c.Password = pw
	}
	if c.Password != "" {
		n.mu.Lock()
		n.passwords[key] = c.Password
		n.mu.Unlock()
	}
	return nil
}

// configureDAV fills in a WebDAV login before the first request of a session.
func (n *Net) configureDAV(key string, c *netDAVConfig) error {
	n.mu.Lock()
	pw, hasPw := n.passwords[key]
	n.mu.Unlock()
	if c.Password == "" && hasPw {
		c.Password = pw
	}
	if c.Password != "" {
		n.mu.Lock()
		n.passwords[key] = c.Password
		n.mu.Unlock()
	}
	return nil
}

// netKnownHosts are the known_hosts files: the app's own first (new keys are saved there, so the app never writes to
// ~/.ssh), then the user's OpenSSH file, read only.
func netKnownHosts() []string {
	var out []string
	if dir := configDir(); dir != "" {
		out = append(out, filepath.Join(dir, "known_hosts"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".ssh", "known_hosts"))
	}
	return out
}

// netExpandHome turns a leading "~" into the home folder.
func netExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// ---- connecting -----------------------------------------------------------------------------------------------------

// NetConnected is the answer to a connect: the panel path to show, and the local folder the site wants in the other
// panel ("" for none).
type NetConnected struct {
	Path     string `json:"path"`
	LocalDir string `json:"localDir"`
}

var errNetCancelled = errors.New("Connecting was cancelled.")

// Connect opens a server by URL or host name (quick connect, Ctrl+N; also what the path bar does).
func (n *Net) Connect(raw string) (NetConnected, error) {
	u, err := netParseURL(raw)
	if err != nil {
		return NetConnected{}, err
	}
	p, err := n.connect(u, nil)
	return NetConnected{Path: p}, err
}

// ConnectSite opens a saved connection of the connection manager (Ctrl+F). An open session to the same server is
// replaced, so changed settings take effect.
func (n *Net) ConnectSite(id string) (NetConnected, error) {
	st, err := n.loadStore()
	if err != nil {
		return NetConnected{}, err
	}
	site, ok := st.Get(id)
	if !ok {
		return NetConnected{}, errors.New("This saved connection is not there any more.")
	}
	u, err := netSiteURL(site)
	if err != nil {
		return NetConnected{}, err
	}
	missing := false
	if site.Password != "" {
		pw, miss, err := n.sitePassword(st, site)
		if err != nil {
			return NetConnected{}, err
		}
		u.Password, missing = pw, miss
	}
	n.disconnectSession(u.String(), false)
	n.mu.Lock()
	n.sites[u.key()] = site
	if missing {
		n.askNotes[u.key()] = "The saved password is not in the system keychain any more."
	}
	n.mu.Unlock()
	p, err := n.connect(u, &site)
	n.mu.Lock()
	delete(n.askNotes, u.key()) // logged in without asking (a key, the agent): the note is not needed
	n.mu.Unlock()
	if err != nil {
		return NetConnected{}, err
	}
	n.mu.Lock()
	if i := netSiteIndex(st, id); i >= 0 {
		st.Sites[i].LastUsed = time.Now()
		n.saveStoreLocked()
	}
	n.mu.Unlock()
	return NetConnected{Path: p, LocalDir: site.LocalDir}, nil
}

// netSiteURL is the URL a saved site connects to, with its remote folder.
func netSiteURL(site ftp.Site) (netURL, error) {
	proto := string(site.Protocol)
	if proto == "" {
		proto = "ftp"
	}
	if netSchemes[proto] == "" {
		return netURL{}, fmt.Errorf("%s is not a protocol mormiCommander knows.", proto)
	}
	if strings.TrimSpace(site.Host) == "" {
		return netURL{}, fmt.Errorf("The connection %s has no server name.", site.Name)
	}
	u := netURL{Scheme: proto, User: site.User, Host: strings.ToLower(strings.TrimSpace(site.Host)), Port: site.Port}
	if u.Port == netDefaultPorts[u.Scheme] {
		u.Port = 0
	}
	if u.Scheme == "sftp" && u.User == "" {
		u.User = netLocalUser()
	}
	if d := strings.TrimSpace(site.RemoteDir); d != "" {
		d = strings.ReplaceAll(d, `\`, "/")
		if !strings.HasPrefix(d, "/") {
			d = "/~/" + d // relative to the login folder
			if u.Scheme != "sftp" {
				d = "/" + strings.TrimPrefix(d, "/~/")
			}
		}
		u.Path = d
	}
	return u, nil
}

// connect dials u (asking again for the password when the server refuses the login) and returns the panel path.
func (n *Net) connect(u netURL, site *ftp.Site) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	n.mu.Lock()
	if n.cancel != nil {
		n.cancel()
	}
	n.connCtx, n.cancel = ctx, cancel
	key := u.key()
	if u.Password != "" {
		n.passwords[key] = u.Password
	}
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		if n.connCtx == ctx {
			n.connCtx, n.cancel = nil, nil
		}
		n.mu.Unlock()
		cancel()
	}()
	plain := u
	plain.Password, plain.HasPass = "", false
	for attempt := 0; ; attempt++ {
		p, err := n.dial(ctx, plain)
		if err == nil {
			return p, nil
		}
		if ctx.Err() != nil || errors.Is(err, errNetCancelled) {
			return "", errNetCancelled
		}
		if !netAuthFailed(err) || attempt >= 3 {
			return "", netConnectErr(u, err)
		}
		// The engines remember the password they logged in with, so a known password means one was sent and
		// refused. Without one (a WebDAV share tried without a password first), the question is a plain prompt.
		n.mu.Lock()
		sent := n.passwords[key] != ""
		delete(n.passwords, key)
		n.mu.Unlock()
		user := u.User
		if user == "" {
			user = "anonymous"
		}
		msg := netRetryMessage(sent)
		if !sent {
			msg = n.askNote(key)
		}
		pw, ok := n.askPassword(fmt.Sprintf("Password for %s on %s", user, u.Host), msg)
		if !ok {
			return "", errNetCancelled
		}
		n.mu.Lock()
		n.passwords[key] = pw
		n.mu.Unlock()
	}
}

// askNote is the text under the first password question of a session (why it is asked), said once; usually "".
func (n *Net) askNote(key string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	note := n.askNotes[key]
	delete(n.askNotes, key)
	return note
}

// netRetryMessage is the text under the password question of connect: it says the login was refused only when a
// password was actually sent.
func netRetryMessage(sent bool) string {
	if sent {
		return "The server did not accept the login. Type the password again."
	}
	return ""
}

// dial opens (or reuses) the session of u and returns the panel path to show.
func (n *Net) dial(ctx context.Context, u netURL) (string, error) {
	switch netSchemes[u.Scheme] {
	case "webdav":
		return n.dav.Connect(ctx, u)
	default:
		r := n.router()
		p := u.String()
		if u.Path == "" {
			p = u.key() // no path: the login folder
		}
		return r.Connect(ctx, p)
	}
}

// CancelConnect stops the connection being made (the Cancel button of "Connecting…").
func (n *Net) CancelConnect() {
	n.mu.Lock()
	c := n.cancel
	ctx := n.connCtx
	n.mu.Unlock()
	if c != nil {
		c()
	}
	if ctx != nil {
		n.dropAsks()
	}
}

// netAuthFailed says whether err means "wrong user or password".
func netAuthFailed(err error) bool {
	if errors.Is(err, ftp.ErrLoginFailed) || ftp.ReplyCode(err) == 530 {
		return true
	}
	if errors.Is(err, sftp.ErrNoAuthMethod) || errors.Is(err, fs.ErrPermission) && strings.Contains(err.Error(), "401") {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "unable to authenticate") || strings.Contains(s, "401 Unauthorized")
}

// netConnectErr says in plain words why a connection failed.
func netConnectErr(u netURL, err error) error {
	host := u.Host
	var hk *sftp.HostKeyError
	switch {
	case errors.As(err, &hk):
		return fmt.Errorf("The key of %s was not accepted, so mormiCommander did not connect.", host)
	case errors.Is(err, sftp.ErrHostKeyRevoked):
		return fmt.Errorf("The key of %s is marked as revoked in known_hosts. mormiCommander did not connect.", host)
	case netAuthFailed(err):
		return fmt.Errorf("%s did not accept the user name or password.", host)
	case errors.Is(err, ftp.ErrTLSRefused):
		return fmt.Errorf("%s does not offer an encrypted (TLS) connection. Turn off TLS in the connection settings to connect without encryption.", host)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s did not answer in time.", host)
	}
	msg := err.Error()
	for _, p := range []string{"vfs: ", "ftp: ", "sftp: ", "webdav: "} {
		msg = strings.TrimPrefix(msg, p)
	}
	if strings.Contains(msg, "connection refused") {
		return fmt.Errorf("%s refused the connection. Check the server name and port.", host)
	}
	if strings.Contains(msg, "no such host") {
		return fmt.Errorf("The server %s was not found. Check its name.", host)
	}
	return fmt.Errorf("Can't connect to %s: %s", host, msg)
}

// Disconnect closes the connection of the server p is on (Ctrl+Shift+F) and forgets the password typed for it.
func (n *Net) Disconnect(p string) error {
	if !netIsRemote(p) {
		return errors.New("This panel shows no server.")
	}
	n.disconnectSession(p, true)
	return nil
}

// disconnectSession closes the session of p; forget also drops the remembered site, password and transfer mode.
func (n *Net) disconnectSession(p string, forget bool) {
	key := netKeyFor(p)
	if key == "" {
		return
	}
	if netSchemes[netSchemeOf(p)] == "webdav" {
		n.dav.Disconnect(key)
	} else {
		n.router().Disconnect(key + "/")
	}
	if forget {
		n.mu.Lock()
		delete(n.sites, key)
		delete(n.passwords, key)
		delete(n.modes, key)
		n.mu.Unlock()
	}
}

// NetSession is one open server connection.
type NetSession struct {
	URL  string `json:"url"`  // the server root as a panel URL
	Kind string `json:"kind"` // "ftp", "sftp" or "webdav"
	Name string `json:"name"` // the saved connection's name, "" for a quick connection
	Mode string `json:"mode"` // FTP transfer mode: "auto", "binary", "ascii" ("" for others)
}

// Sessions lists the open server connections.
func (n *Net) Sessions() []NetSession {
	r := n.router()
	var out []NetSession
	add := func(urls []string, kind string) {
		for _, u := range urls {
			s := NetSession{URL: u, Kind: kind}
			key := netKeyFor(u)
			n.mu.Lock()
			if site, ok := n.sites[key]; ok {
				s.Name = site.Name
			}
			n.mu.Unlock()
			if kind == "ftp" {
				if m, err := r.FTP.TransferMode(u); err == nil {
					s.Mode = m.String()
				}
			}
			out = append(out, s)
		}
	}
	if r.FTP != nil {
		add(r.FTP.Sessions(), "ftp")
	}
	if r.SFTP != nil {
		add(r.SFTP.Sessions(), "sftp")
	}
	add(n.dav.Sessions(), "webdav")
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

// NetStatus is what the bar above a server panel shows.
type NetStatus struct {
	Remote    bool   `json:"remote"`    // p is a server URL
	Connected bool   `json:"connected"` // its session is open
	Kind      string `json:"kind"`      // "ftp", "sftp", "webdav"
	Name      string `json:"name"`      // the saved connection's name
	Mode      string `json:"mode"`      // FTP transfer mode
	Log       bool   `json:"log"`       // a protocol log exists
}

// Status describes the connection of the server p is on.
func (n *Net) Status(p string) NetStatus {
	if !netIsRemote(p) {
		return NetStatus{}
	}
	key := netKeyFor(p)
	st := NetStatus{Remote: true, Kind: netSchemes[netSchemeOf(p)]}
	for _, s := range n.Sessions() {
		if netKeyFor(s.URL) == key {
			st.Connected, st.Name, st.Mode = true, s.Name, s.Mode
		}
	}
	if st.Kind == "ftp" && st.Mode == "" {
		st.Mode = n.modeFor(key).String()
	}
	n.mu.Lock()
	_, st.Log = n.logs[key]
	n.mu.Unlock()
	return st
}

// modeFor is the transfer mode a new session of key would use.
func (n *Net) modeFor(key string) ftp.TransferMode {
	n.mu.Lock()
	m, ok := n.modes[key]
	site, hasSite := n.sites[key]
	n.mu.Unlock()
	if ok {
		return m
	}
	if hasSite {
		if m := ftp.ParseTransferMode(site.Transfer); m != ftp.TransferDefault {
			return m
		}
	}
	if m := ftp.ParseTransferMode(n.options().TransferMode); m != ftp.TransferDefault {
		return m
	}
	return ftp.TransferAuto
}

// CycleTransferMode switches the FTP transfer mode of the server p is on: binary → ASCII → automatic → binary
// (Ctrl+Shift+M, F-387, F-816). It returns the new mode.
func (n *Net) CycleTransferMode(p string) (string, error) {
	key := netKeyFor(p)
	if key == "" || netSchemes[netSchemeOf(p)] != "ftp" {
		return "", errors.New("The transfer mode is for FTP connections only.")
	}
	next := ftp.TransferBinary
	switch n.modeFor(key) {
	case ftp.TransferBinary:
		next = ftp.TransferASCII
	case ftp.TransferASCII:
		next = ftp.TransferAuto
	}
	return next.String(), n.setMode(p, key, next)
}

// SetTransferMode sets the FTP transfer mode ("binary", "ascii" or "auto") of the server p is on.
func (n *Net) SetTransferMode(p, mode string) error {
	key := netKeyFor(p)
	if key == "" || netSchemes[netSchemeOf(p)] != "ftp" {
		return errors.New("The transfer mode is for FTP connections only.")
	}
	m := ftp.ParseTransferMode(mode)
	if m == ftp.TransferDefault {
		return fmt.Errorf("%q is not a transfer mode (binary, ascii or auto).", mode)
	}
	return n.setMode(p, key, m)
}

func (n *Net) setMode(p, key string, m ftp.TransferMode) error {
	n.mu.Lock()
	n.modes[key] = m
	n.mu.Unlock()
	if err := n.router().FTP.SetTransferMode(key+"/", m); err != nil && !errors.Is(err, vfs.ErrNoSession) {
		return err
	}
	return nil
}

// ReconnectFTP closes the FTP sessions (keeping what was typed for them) so the next listing dials again with
// changed options, such as "show hidden files" (F-380). It returns the server roots that were open.
func (n *Net) ReconnectFTP() []string {
	r := n.router()
	if r.FTP == nil {
		return nil
	}
	urls := r.FTP.Sessions()
	for _, u := range urls {
		r.Disconnect(u)
	}
	return urls
}

// SendCommand sends a raw FTP command to the server p is on and returns the reply (cm_FtpSendCommand, F-390).
func (n *Net) SendCommand(p, line string) (string, error) {
	if netSchemes[netSchemeOf(p)] != "ftp" {
		return "", errors.New("Commands can be sent to FTP servers only.")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", errors.New("Type a command, such as SITE HELP.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	reply, err := n.router().FTP.Quote(ctx, p, line)
	if err != nil && reply == "" {
		return "", fmt.Errorf("The command failed: %v", strings.TrimPrefix(err.Error(), "ftp: "))
	}
	return reply, nil
}

// ---- protocol log ---------------------------------------------------------------------------------------------------

// netLog keeps the last lines of a session's FTP protocol log (F-391); passwords are masked by the engine.
type netLog struct {
	mu    sync.Mutex
	lines []string
}

const netLogMax = 1000

func (n *Net) logFor(key string) *netLog {
	n.mu.Lock()
	defer n.mu.Unlock()
	l := n.logs[key]
	if l == nil {
		l = &netLog{}
		n.logs[key] = l
	}
	return l
}

func (l *netLog) printf(format string, args ...any) {
	line := time.Now().Format("15:04:05 ") + strings.TrimRight(fmt.Sprintf(format, args...), "\r\n")
	l.mu.Lock()
	l.lines = append(l.lines, line)
	if len(l.lines) > netLogMax {
		l.lines = append([]string(nil), l.lines[len(l.lines)-netLogMax:]...)
	}
	l.mu.Unlock()
}

// Log returns the protocol log of the server p is on, oldest line first.
func (n *Net) Log(p string) []string {
	key := netKeyFor(p)
	n.mu.Lock()
	l := n.logs[key]
	n.mu.Unlock()
	if l == nil {
		return []string{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string{}, l.lines...)
}

// IsRemote says whether p is a server URL this feature serves (the page asks before treating a path as local).
func (n *Net) IsRemote(p string) bool { return netIsRemote(p) }

// netGlobalProxy is the proxy of Options > Proxy for FTP connections that have none of their own: 0 none, 1 HTTP
// CONNECT, 2 SOCKS4, 3 SOCKS5, 4 USER@host. nil when there is none (or no host). The password is asked by the
// connection when the proxy wants one.
func netGlobalProxy(kind int, host string, port int, user string) *ftp.Proxy {
	host = strings.TrimSpace(host)
	if kind <= 0 || host == "" {
		return nil
	}
	t := map[int]ftp.ProxyType{1: ftp.ProxyHTTP, 2: ftp.ProxySOCKS4, 3: ftp.ProxySOCKS5, 4: ftp.ProxyFTPUserAtHost}[kind]
	if t == ftp.ProxyNone {
		return nil
	}
	return &ftp.Proxy{Type: t, Host: host, Port: port, User: strings.TrimSpace(user)}
}

// netMaxConns is the number of connections a new FTP session may open (Options > FTP > "Connections per FTP server",
// F-643): one browses in the panel, the others run transfers next to it. The default 2 is what it always was.
func netMaxConns() int {
	if s, ok := currentSettings(); ok && s.FTP.MaxConns > 0 {
		return min(s.FTP.MaxConns, 8)
	}
	return 2
}

// netIdleMinutes is how long an unused extra FTP connection stays open (F-650): the page's own "net" option when
// set, else Options > FTP > "Close unused extra connections after", else 10 minutes as before.
func netIdleMinutes(o netOptions) int {
	if o.IdleMinutes > 0 {
		return o.IdleMinutes
	}
	if s, ok := currentSettings(); ok && s.FTP.IdleMinutes > 0 {
		return s.FTP.IdleMinutes
	}
	return 10
}

// netSFTPProxy is the proxy of Options > Proxy for SFTP connections: SOCKS4, SOCKS5 or HTTP CONNECT tunnel any TCP
// connection; the FTP-only kind (USER@host) and "none" give nil.
func netSFTPProxy(kind int, host string, port int, user string) *sftp.Proxy {
	fp := netGlobalProxy(kind, host, port, user)
	if fp == nil {
		return nil
	}
	t, ok := map[ftp.ProxyType]sftp.ProxyType{ftp.ProxyHTTP: sftp.ProxyHTTP, ftp.ProxySOCKS4: sftp.ProxySOCKS4, ftp.ProxySOCKS5: sftp.ProxySOCKS5}[fp.Type]
	if !ok {
		return nil
	}
	return &sftp.Proxy{Type: t, Addr: fp.Address(), User: fp.User}
}

// proxyPassword is the password of the proxy user at addr (Options > Proxy, F-637): "" when no user is set, else the
// one typed before in this session, else it is asked (and kept in memory only, never in the settings file).
func (n *Net) proxyPassword(user, addr string) (string, error) {
	if user == "" {
		return "", nil
	}
	key := "proxy://" + user + "@" + strings.ToLower(addr)
	n.mu.Lock()
	pw, ok := n.passwords[key]
	n.mu.Unlock()
	if ok {
		return pw, nil
	}
	pw, ok = n.askPassword(fmt.Sprintf("Password for %s on the proxy %s", user, addr), "Options > Proxy names this user. Leave the password empty when the proxy needs none.")
	if !ok {
		return "", errNetCancelled
	}
	n.mu.Lock()
	n.passwords[key] = pw
	n.mu.Unlock()
	return pw, nil
}
