package main

// The Network panel (#148, like Total Commander's Network Neighborhood): network:// is a folder a panel shows.
//
//   - network:// lists the servers: the ones found on the local network so far and the ones used before. The search
//     starts the first time the folder is shown and then keeps running in the background while the app runs, so the
//     list fills as servers answer and is there at once when the panel comes back. Each change is sent to the page
//     ("network-servers"), which rereads a panel showing the folder.
//   - network://<server> lists the server's shares (asking for a user name and password through the page's
//     "network-auth" event when the server wants them).
//   - network://<server>/<share> mounts the share (Open: disconnected again when the app ends) and shows the real
//     folder, whose ".." leads back to the shares.
//
// smb://, nfs:// and afp:// typed in the path bar work the same way: smb://nas lists the shares of nas, smb://nas/media
// opens the share.

import (
	"context"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/netbrowse"
	"mormicommander/internal/vfs"
)

const networkRoot = "network://"

func init() {
	for _, s := range []string{"network", "smb", "nfs", "afp"} {
		vfs.Register(s, networkFS{})
	}
}

// netPanel is the background search and what it found.
type netPanel struct {
	once    sync.Once
	mu      sync.Mutex
	found   map[string]netbrowse.Server // kind + "\x00" + lower name → server
	started time.Time
	note    string
	stop    context.CancelFunc
	pending *time.Timer

	sessions map[string]netbrowse.Creds            // Target.Key() → credentials typed this session
	users    map[string]string                     // kind + "\x00" + lower host → the user name last given
	names    map[string]netbrowse.Server           // the root folder's entry names → servers
	shares   map[string]map[string]netbrowse.Share // server path → entry name → share
}

// NetSearch tells the page how the search is going.
type NetSearch struct {
	Searching bool   `json:"searching"` // the first seconds of the search: show that it's looking
	Count     int    `json:"count"`     // servers found so far
	Note      string `json:"note"`      // why nothing can be found, when it can't search
}

// netSearchPhase is how long the page shows "Searching…" after the search starts; servers that answer later still
// appear, the search never stops while the app runs.
const netSearchPhase = 12 * time.Second

func (n *Network) watchState() *netPanel {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.w == nil {
		n.w = &netPanel{found: map[string]netbrowse.Server{}, sessions: map[string]netbrowse.Creds{}, users: map[string]string{},
			names: map[string]netbrowse.Server{}, shares: map[string]map[string]netbrowse.Share{}}
	}
	return n.w
}

// startWatch starts the background search once.
func (n *Network) startWatch() {
	w := n.watchState()
	w.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		w.mu.Lock()
		w.started, w.stop = time.Now(), cancel
		w.mu.Unlock()
		go func() {
			note := n.watch(ctx, func(ev netbrowse.Event) {
				key := ev.Server.Kind + "\x00" + strings.ToLower(ev.Server.Name)
				w.mu.Lock()
				if ev.Gone {
					delete(w.found, key)
				} else {
					w.found[key] = ev.Server
				}
				w.mu.Unlock()
				n.changed()
			})
			if note != "" {
				w.mu.Lock()
				w.note = note
				w.mu.Unlock()
				n.changed()
			}
		}()
		n.changed() // the page shows "Searching…" at once
		// the end of the "Searching…" phase is a change too
		time.AfterFunc(netSearchPhase, n.changed)
	})
}

// changed tells the page, at most four times a second, that the servers changed.
func (n *Network) changed() {
	w := n.watchState()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != nil {
		return
	}
	w.pending = time.AfterFunc(250*time.Millisecond, func() {
		w.mu.Lock()
		w.pending = nil
		w.mu.Unlock()
		n.app.emit("network-servers", n.Status())
	})
}

// Status says whether the Network panel's search is still in its first phase, and how many servers it found.
func (n *Network) Status() NetSearch {
	w := n.watchState()
	w.mu.Lock()
	defer w.mu.Unlock()
	return NetSearch{Searching: !w.started.IsZero() && time.Since(w.started) < netSearchPhase && w.note == "", Count: len(w.found), Note: w.note}
}

// SetCreds keeps a user name and password for a server for this session (and in the keychain when Save is ticked,
// once they work); the page then shows the folder again.
func (n *Network) SetCreds(r NetRequest) error {
	t, err := netbrowse.ParseTarget(r.Address)
	if err != nil {
		return err
	}
	t.User, t.Share = r.User, ""
	w := n.watchState()
	w.mu.Lock()
	w.sessions[t.Key()] = netbrowse.Creds{User: r.User, Password: r.Password, Domain: r.Domain}
	w.users[t.Kind+"\x00"+strings.ToLower(t.Host)] = r.User
	w.mu.Unlock()
	if r.Save && r.User != "" && r.Password != "" {
		if kc, err := n.openKeychain(); err == nil {
			_ = kc.Set(netKeyName(t), r.Password)
		}
	}
	return nil
}

// panelCreds are the credentials for t: typed this session, else the user used before with a saved password.
func (n *Network) panelCreds(t netbrowse.Target) (netbrowse.Target, netbrowse.Creds) {
	w := n.watchState()
	w.mu.Lock()
	if t.User == "" {
		t.User = w.users[t.Kind+"\x00"+strings.ToLower(t.Host)]
	}
	c, ok := w.sessions[netbrowse.Target{Kind: t.Kind, Host: t.Host, Port: t.Port, User: t.User}.Key()]
	w.mu.Unlock()
	if ok {
		return t, c
	}
	if t.User == "" {
		for _, r := range n.recent() {
			if r.Kind == t.Kind && strings.EqualFold(r.Host, t.Host) {
				t.User = r.User
			}
		}
	}
	return t, netbrowse.Creds{User: t.User, Password: n.savedPassword(t)}
}

// errNetAuth is what the panel shows while the page asks for a user name and password.
type errNetAuth struct{ server string }

func (e errNetAuth) Error() string { return e.server + " wants a user name and password." }

// askAuth tells the page to ask for credentials for t, to show path again afterwards.
func (n *Network) askAuth(t netbrowse.Target, path string) error {
	n.app.emit("network-auth", map[string]string{"address": netbrowse.Target{Kind: t.Kind, Host: t.Host, Port: t.Port}.URL(), "user": t.User, "path": path, "server": t.Host})
	return errNetAuth{t.Host}
}

// serverName is a server's entry in the root folder: its name, with the protocol when it isn't SMB, and no "/".
func serverName(s netbrowse.Server) string {
	name := s.Name
	if name == "" {
		name = s.Host
	}
	switch s.Kind {
	case "afp":
		name += " (AFP)"
	case "nfs":
		name += " (NFS)"
	}
	return strings.ReplaceAll(name, "/", "∕")
}

// shareName is a share's entry name: NFS exports are paths, so their "/" becomes "∕".
func shareName(s netbrowse.Share) string {
	return strings.ReplaceAll(strings.TrimPrefix(s.Name, "/"), "/", "∕")
}

// rootServers lists the root folder: found servers and the ones used before, one entry each. An AFP server that also
// answers SMB is left out (SMB is what macOS uses today).
func (n *Network) rootServers() map[string]netbrowse.Server {
	w := n.watchState()
	w.mu.Lock()
	var list []netbrowse.Server
	for _, s := range w.found {
		list = append(list, s)
	}
	w.mu.Unlock()
	smb := map[string]bool{}
	for _, s := range list {
		if s.Kind == "smb" {
			smb[strings.ToLower(s.Name)] = true
			smb[strings.ToLower(s.Host)] = true
		}
	}
	hosts := map[string]bool{}
	out := map[string]netbrowse.Server{}
	for _, s := range list {
		if s.Kind == "afp" && (smb[strings.ToLower(s.Name)] || smb[strings.ToLower(s.Host)]) {
			continue
		}
		out[serverName(s)] = s
		hosts[s.Kind+"\x00"+strings.ToLower(s.Host)] = true
	}
	for _, r := range n.recent() {
		if hosts[r.Kind+"\x00"+strings.ToLower(r.Host)] {
			continue
		}
		name := serverName(r.Server)
		if _, taken := out[name]; taken {
			continue
		}
		out[name] = r.Server
	}
	w.mu.Lock()
	w.names = out
	w.mu.Unlock()
	return out
}

// resolve reads a network path: the server (from the root's names, or a typed smb://host), and the share part.
func (n *Network) resolve(ctx context.Context, p string) (t netbrowse.Target, server string, share string, err error) {
	if strings.HasPrefix(p, networkRoot) {
		rest := strings.TrimPrefix(p, networkRoot)
		name, share, _ := strings.Cut(rest, "/")
		w := n.watchState()
		w.mu.Lock()
		s, ok := w.names[name]
		w.mu.Unlock()
		if !ok {
			s, ok = n.rootServers()[name]
		}
		if !ok {
			s, ok = n.awaitServer(ctx, name)
		}
		if !ok {
			return t, "", "", &fs.PathError{Op: "list", Path: p, Err: fs.ErrNotExist}
		}
		return netbrowse.Target{Kind: s.Kind, Host: s.Host, Port: s.Port}, networkRoot + name, share, nil
	}
	t, err = netbrowse.ParseTarget(p)
	if err != nil {
		return t, "", "", err
	}
	share = t.Share
	if t.Kind == "nfs" {
		share = strings.TrimPrefix(share, "/")
	}
	t.Share = ""
	return t, t.URL(), share, nil
}

// serverWait is how long a server name the search hasn't found yet is waited for: a button, a bookmark or a typed
// network://Name right after the start comes before the first answers.
var serverWait = 8 * time.Second

// awaitServer starts the search (when it isn't running) and waits for a server of that name to answer.
func (n *Network) awaitServer(ctx context.Context, name string) (netbrowse.Server, bool) {
	n.startWatch()
	end := time.Now().Add(serverWait)
	for time.Now().Before(end) {
		select {
		case <-ctx.Done():
			return netbrowse.Server{}, false
		case <-time.After(200 * time.Millisecond):
		}
		if s, ok := n.rootServers()[name]; ok {
			return s, true
		}
	}
	return netbrowse.Server{}, false
}

// listNetwork is the Network panel's List.
func (n *Network) listNetwork(ctx context.Context, p string) (vfs.Listing, error) {
	p = networkClean(p)
	dirEntry := func(name string) vfs.Entry {
		return vfs.Entry{Name: name, Flags: vfs.FlagDir | vfs.FlagNoAttr, Mode: uint32(fs.ModeDir | 0o755)}
	}
	if p == networkRoot {
		n.startWatch()
		var names []string
		for name := range n.rootServers() {
			names = append(names, name)
		}
		sort.Strings(names)
		l := vfs.Listing{Path: networkRoot}
		for _, name := range names {
			l.Entries = append(l.Entries, dirEntry(name))
		}
		return l, nil
	}
	t, server, share, err := n.resolve(ctx, p)
	if err != nil {
		return vfs.Listing{}, err
	}
	t, c := n.panelCreds(t)
	if share == "" && t.Kind != "afp" {
		list, err := n.shares(ctx, t, c)
		if errors.Is(err, netbrowse.ErrAuth) {
			return vfs.Listing{}, n.askAuth(t, p)
		}
		if err != nil {
			return vfs.Listing{}, err
		}
		n.remember(t, NetRequest{})
		w := n.watchState()
		byName := map[string]netbrowse.Share{}
		l := vfs.Listing{Path: p, Parent: networkRoot}
		for _, s := range list {
			byName[shareName(s)] = s
			l.Entries = append(l.Entries, dirEntry(shareName(s)))
		}
		w.mu.Lock()
		w.shares[server] = byName
		w.mu.Unlock()
		return l, nil
	}
	// a share: mount it and show the real folder
	w := n.watchState()
	w.mu.Lock()
	if s, ok := w.shares[server][share]; ok {
		t.Share = s.Name
	} else {
		t.Share = strings.ReplaceAll(share, "∕", "/")
		if t.Kind == "nfs" {
			t.Share = "/" + t.Share
		}
	}
	w.mu.Unlock()
	m, err := n.mount(ctx, t, c, netbrowse.MountOptions{})
	if errors.Is(err, netbrowse.ErrAuth) {
		return vfs.Listing{}, n.askAuth(t, p)
	}
	if err != nil {
		return vfs.Listing{}, err
	}
	n.remember(t, NetRequest{})
	if m.Made {
		n.mu.Lock()
		n.temps = append(n.temps, m.Path)
		n.mu.Unlock()
		n.app.emit("system-volumes")
	}
	l, err := n.app.fs.List(ctx, m.Path)
	if err != nil {
		return l, err
	}
	if t.Kind != "afp" {
		l.Parent = server // ".." goes back to the shares
	} else {
		l.Parent = networkRoot
	}
	return l, nil
}

// networkPath says whether p is a folder of the Network panel (the servers, or a server's shares).
func networkPath(p string) bool {
	l := strings.ToLower(p)
	for _, s := range []string{"network://", "smb://", "nfs://", "afp://"} {
		if strings.HasPrefix(l, s) {
			return true
		}
	}
	return false
}

// networkClean drops a trailing "/" (but keeps "network://").
func networkClean(p string) string {
	if p == "network:" || p == networkRoot {
		return networkRoot
	}
	return strings.TrimRight(p, "/")
}

// networkFS is the vfs.FS of network://, smb://, nfs:// and afp://.
type networkFS struct{}

func (networkFS) List(ctx context.Context, p string) (vfs.Listing, error) {
	if networkFeature == nil {
		return vfs.Listing{}, errors.New("the network neighbourhood isn't ready")
	}
	return networkFeature.listNetwork(ctx, p)
}

func (networkFS) Stat(p string) (vfs.Entry, error) {
	name := p[strings.LastIndex(networkClean(p), "/")+1:]
	return vfs.Entry{Name: name, Flags: vfs.FlagDir | vfs.FlagNoAttr, Mode: uint32(fs.ModeDir | 0o755)}, nil
}

func (networkFS) Clean(p string) string { return networkClean(p) }

func (networkFS) Parent(p string) string {
	p = networkClean(p)
	if p == networkRoot {
		return ""
	}
	i := strings.LastIndex(p, "/")
	if i < len(p[:strings.Index(p, "://")+3]) {
		return networkRoot // smb://host, network://server
	}
	return p[:i]
}

func (networkFS) Root(p string) string { return networkRoot }

func (networkFS) Join(dir, name string) string {
	if strings.HasSuffix(dir, "://") {
		return dir + name
	}
	return strings.TrimRight(dir, "/") + "/" + name
}
