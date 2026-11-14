package main

// Network neighbourhood (board issue #148; TC's cm_OpenNetwork, cm_NetConnect, cm_NetDisconnect; F-365, F-383,
// F-386): the page's Network dialog asks this service to find servers, list a server's shares, open or map a share
// and disconnect it again. The work is in internal/netbrowse.
//
// Nothing here runs on its own: the local network is searched only when the user opens the Network dialog, and a
// server is contacted only when the user picks or types it (docs/privacy.md).
//
// Servers that were connected are remembered in network.json in the settings folder (address and user name, never a
// password), newest first. A password is kept only when the user ticks "Save the password", in the system keychain
// under a name made from the server and user (netKeyName). A share that "Open" had to mount is disconnected again when the app ends; a mapped
// one stays.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mormicommander/internal/config"
	"mormicommander/internal/keychain"
	"mormicommander/internal/netbrowse"
)

// Network is the page's door to the network neighbourhood.
type Network struct {
	app *App
	// tests replace these
	discover     func(context.Context) ([]netbrowse.Server, string)
	shares       func(context.Context, netbrowse.Target, netbrowse.Creds) ([]netbrowse.Share, error)
	mount        func(context.Context, netbrowse.Target, netbrowse.Creds, netbrowse.MountOptions) (netbrowse.Mounted, error)
	unmount      func(context.Context, string) error
	openKeychain func() (keychain.Keychain, error)
	dir          func() string // where network.json lives

	mu    sync.Mutex
	temps []string // shares Open mounted, disconnected at the end
}

var networkFeature *Network

func init() {
	registerFeature("network", func(a *App) []exposed {
		networkFeature = newNetwork(a)
		return []exposed{expose(networkFeature)}
	})
	onShutdown(func(a *App) {
		if networkFeature != nil {
			networkFeature.shutdown()
		}
	})
}

func newNetwork(a *App) *Network {
	return &Network{app: a, discover: netbrowse.Discover, shares: netbrowse.Shares, mount: netbrowse.Mount,
		unmount: netbrowse.Unmount, openKeychain: keychain.Open, dir: configDir}
}

// NetServer is a server in the Network dialog.
type NetServer struct {
	netbrowse.Server
	Address string `json:"address"` // what to pass back: smb://host, nfs://host
	User    string `json:"user"`    // the user name last used with it
	Saved   bool   `json:"saved"`   // a password is in the keychain
}

// NetDiscovery is the answer of Discover.
type NetDiscovery struct {
	Servers []NetServer `json:"servers"`
	Note    string      `json:"note"` // why the search found nothing, when it couldn't search
}

// NetRequest is what the page sends to list shares or connect.
type NetRequest struct {
	Address    string `json:"address"` // smb://host, \\host, nfs://host, or a typed one with the share in it
	Share      string `json:"share"`   // overrides the share in Address
	User       string `json:"user"`
	Password   string `json:"password"` // "" = the saved one, if any
	Domain     string `json:"domain"`
	Save       bool   `json:"save"`       // keep the password in the system keychain
	Letter     string `json:"letter"`     // Windows: map to this drive ("Z:")
	Persistent bool   `json:"persistent"` // Windows: reconnect at sign-in
}

// NetShares is the answer of Shares.
type NetShares struct {
	Server   netbrowse.Target  `json:"server"`
	Shares   []netbrowse.Share `json:"shares"`
	NeedAuth bool              `json:"needAuth"` // ask for a user name and password, then call again
}

// NetMounted is the answer of Open and Map.
type NetMounted struct {
	Path     string `json:"path"`
	NeedAuth bool   `json:"needAuth"`
}

// Discover searches the local network for file servers (a few seconds) and lists them after the remembered ones.
func (n *Network) Discover(ctx context.Context) NetDiscovery {
	found, note := n.discover(ctx)
	recent := n.recent()
	var all []netbrowse.Server
	for _, r := range recent {
		all = append(all, r.Server)
	}
	all = append(all, found...)
	byKey := map[string]recentServer{}
	for _, r := range recent {
		byKey[r.Kind+"\x00"+strings.ToLower(r.Host)] = r
	}
	out := NetDiscovery{Note: note}
	seen := map[string]bool{}
	for _, s := range all {
		k := s.Kind + "\x00" + strings.ToLower(s.Host)
		if seen[k] {
			continue
		}
		seen[k] = true
		ns := NetServer{Server: s, Address: netbrowse.Target{Kind: s.Kind, Host: s.Host, Port: s.Port}.URL()}
		if r, ok := byKey[k]; ok {
			ns.User = r.User
			ns.Saved = n.savedPassword(netbrowse.Target{Kind: s.Kind, Host: s.Host, Port: s.Port, User: r.User}) != ""
		}
		out.Servers = append(out.Servers, ns)
	}
	return out
}

// target reads the request's address, share and user.
func (r NetRequest) target() (netbrowse.Target, error) {
	t, err := netbrowse.ParseTarget(r.Address)
	if err != nil {
		return t, err
	}
	if r.Share != "" {
		t.Share = r.Share
	}
	if r.User != "" {
		t.User = r.User
	}
	return t, nil
}

// creds are the request's credentials, with the saved password when none was typed.
func (n *Network) creds(t netbrowse.Target, r NetRequest) netbrowse.Creds {
	c := netbrowse.Creds{User: t.User, Password: r.Password, Domain: r.Domain}
	if c.User != "" && c.Password == "" {
		c.Password = n.savedPassword(t)
	}
	return c
}

// Shares lists the shared folders of a server. NeedAuth asks the page for a user name and password.
func (n *Network) Shares(ctx context.Context, r NetRequest) (NetShares, error) {
	t, err := r.target()
	if err != nil {
		return NetShares{}, err
	}
	t.Share = ""
	list, err := n.shares(ctx, t, n.creds(t, r))
	if errors.Is(err, netbrowse.ErrAuth) {
		return NetShares{Server: t, NeedAuth: true}, nil
	}
	if err != nil {
		return NetShares{}, err
	}
	n.remember(t, r)
	return NetShares{Server: t, Shares: list}, nil
}

// Open makes a share readable and returns the folder to show: a UNC path on Windows, a mounted folder elsewhere (a
// mount Open made is disconnected again when the app ends).
func (n *Network) Open(ctx context.Context, r NetRequest) (NetMounted, error) {
	return n.connect(ctx, r, false)
}

// Map mounts a share to stay: a drive letter on Windows (reconnected at sign-in when asked), /Volumes on macOS, GVfs on
// Linux. It then shows in the drive bar like any volume.
func (n *Network) Map(ctx context.Context, r NetRequest) (NetMounted, error) {
	return n.connect(ctx, r, true)
}

func (n *Network) connect(ctx context.Context, r NetRequest, keep bool) (NetMounted, error) {
	t, err := r.target()
	if err != nil {
		return NetMounted{}, err
	}
	if t.Share == "" && t.Kind != "afp" {
		return NetMounted{}, errors.New("choose a share of the server first")
	}
	o := netbrowse.MountOptions{}
	if keep {
		o = netbrowse.MountOptions{Letter: r.Letter, Persistent: r.Persistent}
	}
	m, err := n.mount(ctx, t, n.creds(t, r), o)
	if errors.Is(err, netbrowse.ErrAuth) {
		return NetMounted{NeedAuth: true}, nil
	}
	if err != nil {
		return NetMounted{}, err
	}
	n.remember(t, r)
	if m.Made && !keep {
		n.mu.Lock()
		n.temps = append(n.temps, m.Path)
		n.mu.Unlock()
	}
	if m.Made {
		n.app.emit("system-volumes")
	}
	return NetMounted{Path: m.Path}, nil
}

// Disconnect unmounts (or unmaps) the network share at path.
func (n *Network) Disconnect(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("no share chosen")
	}
	if err := n.unmount(ctx, path); err != nil {
		return err
	}
	n.mu.Lock()
	for i, p := range n.temps {
		if p == path {
			n.temps = append(n.temps[:i], n.temps[i+1:]...)
			break
		}
	}
	n.mu.Unlock()
	n.app.emit("system-volumes")
	return nil
}

// FreeLetters lists the drive letters a share can be mapped to (Windows; empty elsewhere).
func (n *Network) FreeLetters() []string { return netbrowse.FreeLetters() }

// Forget removes a server from the remembered ones, with its saved password.
func (n *Network) Forget(address string) error {
	t, err := netbrowse.ParseTarget(address)
	if err != nil {
		return err
	}
	list := n.recent()
	var keep []recentServer
	for _, r := range list {
		if r.Kind == t.Kind && strings.EqualFold(r.Host, t.Host) {
			if r.User != "" {
				if kc, err := n.openKeychain(); err == nil {
					_ = kc.Delete(netKeyName(netbrowse.Target{Kind: r.Kind, Host: r.Host, Port: r.Port, User: r.User}))
				}
			}
			continue
		}
		keep = append(keep, r)
	}
	return n.saveRecent(keep)
}

// shutdown disconnects the shares Open mounted.
func (n *Network) shutdown() {
	n.mu.Lock()
	temps := n.temps
	n.temps = nil
	n.mu.Unlock()
	for _, p := range temps {
		_ = n.unmount(context.Background(), p)
	}
}

// --- remembered servers and saved passwords ---

// recentServer is one line of network.json.
type recentServer struct {
	netbrowse.Server
	User string `json:"user,omitempty"`
}

const maxRecentServers = 20

func (n *Network) recentFile() string { return filepath.Join(n.dir(), "network.json") }

func (n *Network) recent() []recentServer {
	b, err := os.ReadFile(n.recentFile())
	if err != nil {
		return nil
	}
	var list []recentServer
	if json.Unmarshal(b, &list) != nil {
		return nil
	}
	for i := range list {
		list[i].Source = "recent"
	}
	return list
}

func (n *Network) saveRecent(list []recentServer) error {
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(n.dir(), 0o700); err != nil {
		return err
	}
	return config.WriteFileAtomic(n.recentFile(), b, 0o600)
}

// remember puts t's server first in the remembered list and saves the password when asked.
func (n *Network) remember(t netbrowse.Target, r NetRequest) {
	name := t.Host
	list := n.recent()
	var out []recentServer
	for _, s := range list {
		if s.Kind == t.Kind && strings.EqualFold(s.Host, t.Host) {
			name = s.Name
			continue
		}
		out = append(out, s)
	}
	out = append([]recentServer{{Server: netbrowse.Server{Name: name, Host: t.Host, Port: t.Port, Kind: t.Kind}, User: t.User}}, out...)
	if len(out) > maxRecentServers {
		out = out[:maxRecentServers]
	}
	_ = n.saveRecent(out)
	if r.Save && t.User != "" && r.Password != "" {
		if kc, err := n.openKeychain(); err == nil {
			_ = kc.Set(netKeyName(t), r.Password)
		}
	}
}

// netKeyName is the keychain name of a server's password: "net.smb.pat_nas.local.<hash>". Keychain names take only
// letters, digits, '.', '_' and '-', so the rest becomes '_' and a short hash of the full key keeps names apart.
func netKeyName(t netbrowse.Target) string {
	key := t.Key()
	sum := sha256.Sum256([]byte(key))
	var b strings.Builder
	for _, r := range strings.TrimPrefix(strings.TrimPrefix(key, t.Kind+"://"), "/") {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if len(name) > 80 {
		name = name[:80]
	}
	return "net." + t.Kind + "." + name + "." + hex.EncodeToString(sum[:4])
}

// savedPassword is the password in the keychain for t's server and user, "" when there is none.
func (n *Network) savedPassword(t netbrowse.Target) string {
	if t.User == "" {
		return ""
	}
	kc, err := n.openKeychain()
	if err != nil {
		return ""
	}
	pw, err := kc.Get(netKeyName(t))
	if err != nil {
		return ""
	}
	return pw
}
