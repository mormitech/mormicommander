// This file is the data model behind Total Commander's FTP connection
// manager: a list of saved connections ("sites") arranged in folders, the
// named proxies they can use, and the header of the master password that
// protects their passwords. The model is saved as one JSON file in the
// config folder, written atomically (temporary file plus rename) with
// owner-only permissions, and Save refuses to write a password that is not
// sealed, so a programming mistake cannot leak a password to disk. The
// dialog edits a Store; connecting turns a Site into a Config.
package ftp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Protocol of a saved connection. SFTP sites live in the same manager but
// are opened by the SFTP engine, not by this package.
type Protocol string

const (
	ProtoFTP   Protocol = "ftp"   // plain FTP
	ProtoFTPES Protocol = "ftpes" // explicit TLS
	ProtoFTPS  Protocol = "ftps"  // implicit TLS
	ProtoSFTP  Protocol = "sftp"
)

// Site is one saved connection.
type Site struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Folder   string   `json:"folder,omitempty"` // "Work/Clients"; "" is the top level
	Protocol Protocol `json:"protocol"`
	Host     string   `json:"host"`
	Port     int      `json:"port,omitempty"`
	User     string   `json:"user,omitempty"`

	// Password is always sealed (see SecretBox); empty means "ask when
	// connecting". Use Store.SetPassword / Store.Password.
	Password string `json:"password,omitempty"`

	KeyFile     string    `json:"keyFile,omitempty"` // SFTP private key
	RemoteDir   string    `json:"remoteDir,omitempty"`
	LocalDir    string    `json:"localDir,omitempty"`
	Active      bool      `json:"active,omitempty"`   // active instead of passive mode
	TryTLS      bool      `json:"tryTLS,omitempty"`   // plain FTP: use AUTH TLS when offered
	Transfer    string    `json:"transfer,omitempty"` // "auto", "binary", "ascii"
	ASCIIMasks  []string  `json:"asciiMasks,omitempty"`
	Encoding    string    `json:"encoding,omitempty"` // "auto", "utf-8", "latin1"
	TimeOffset  int       `json:"timeOffsetMinutes,omitempty"`
	Keepalive   int       `json:"keepaliveSeconds,omitempty"`
	ProxyName   string    `json:"proxy,omitempty"`
	ListCommand string    `json:"listCommand,omitempty"`
	NoMLSD      bool      `json:"noMLSD,omitempty"`
	NoEPSV      bool      `json:"noEPSV,omitempty"`
	Comment     string    `json:"comment,omitempty"`
	LastUsed    time.Time `json:"lastUsed,omitzero"`
}

// Path is the site's position in the tree, e.g. "Work/Clients/Acme".
func (s Site) Path() string {
	if s.Folder == "" {
		return s.Name
	}
	return s.Folder + "/" + s.Name
}

// NamedProxy is a proxy definition sites refer to by name.
type NamedProxy struct {
	Name     string `json:"name"`
	Proxy    Proxy  `json:"proxy"`
	Password string `json:"password,omitempty"` // sealed, like site passwords
}

// Store is the whole connection manager content.
type Store struct {
	Version int           `json:"version"`
	Folders []string      `json:"folders,omitempty"` // includes empty folders
	Sites   []Site        `json:"sites"`
	Proxies []NamedProxy  `json:"proxies,omitempty"`
	Crypto  *CryptoHeader `json:"crypto,omitempty"`

	// Keychain says new and changed passwords go to the system keychain, and the list keeps only a reference to
	// them ("keychain:<name>", see KeyRefPrefix). It is off by default; passwords saved before keep their form until
	// the option is switched, which moves them (the UI does that, as it knows the keychain).
	Keychain bool `json:"keychain,omitempty"`
}

const storeVersion = 1

// ErrNoSite is returned for unknown site IDs.
var ErrNoSite = errors.New("ftp: no such saved connection")

// LoadStore reads a connection list; a missing file gives an empty store.
func LoadStore(path string) (*Store, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{Version: storeVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("ftp: %s: %w", path, err)
	}
	if s.Version > storeVersion {
		return nil, fmt.Errorf("ftp: %s was written by a newer version", path)
	}
	s.Version = storeVersion
	return &s, nil
}

// Save writes the store atomically with 0600 permissions. It fails if any
// password is not sealed.
func (s *Store) Save(path string) error {
	for _, site := range s.Sites {
		if site.Password != "" && !IsSealed(site.Password) {
			return fmt.Errorf("ftp: refusing to save plain-text password of %q", site.Name)
		}
	}
	for _, p := range s.Proxies {
		if p.Password != "" && !IsSealed(p.Password) {
			return fmt.Errorf("ftp: refusing to save plain-text password of proxy %q", p.Name)
		}
	}
	s.Version = storeVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ftpsites-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(append(b, '\n'))
	if werr == nil {
		werr = tmp.Chmod(0o600)
	}
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(name, path)
	}
	if werr != nil {
		os.Remove(name)
	}
	return werr
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanFolder(f string) string {
	parts := strings.FieldsFunc(f, func(r rune) bool { return r == '/' || r == '\\' })
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, "/")
}

func (s *Store) index(id string) int {
	for i := range s.Sites {
		if s.Sites[i].ID == id {
			return i
		}
	}
	return -1
}

// Add stores a new site (assigning an ID when empty) and returns the ID.
func (s *Store) Add(site Site) (string, error) {
	if strings.TrimSpace(site.Name) == "" {
		return "", errors.New("ftp: a saved connection needs a name")
	}
	if site.Password != "" && !IsSealed(site.Password) {
		return "", errors.New("ftp: use SetPassword to store a password")
	}
	if site.ID == "" {
		site.ID = newID()
	} else if s.index(site.ID) >= 0 {
		return "", fmt.Errorf("ftp: duplicate site id %q", site.ID)
	}
	if site.Protocol == "" {
		site.Protocol = ProtoFTP
	}
	site.Folder = cleanFolder(site.Folder)
	s.Sites = append(s.Sites, site)
	s.ensureFolder(site.Folder)
	return site.ID, nil
}

// Get returns the site with the given ID.
func (s *Store) Get(id string) (Site, bool) {
	if i := s.index(id); i >= 0 {
		return s.Sites[i], true
	}
	return Site{}, false
}

// Update replaces a stored site (matched by ID). The stored password is
// kept when site.Password is empty; use SetPassword to change or clear it.
func (s *Store) Update(site Site) error {
	i := s.index(site.ID)
	if i < 0 {
		return ErrNoSite
	}
	if site.Password == "" {
		site.Password = s.Sites[i].Password
	} else if !IsSealed(site.Password) {
		return errors.New("ftp: use SetPassword to store a password")
	}
	site.Folder = cleanFolder(site.Folder)
	s.Sites[i] = site
	s.ensureFolder(site.Folder)
	return nil
}

// Remove deletes a site.
func (s *Store) Remove(id string) error {
	i := s.index(id)
	if i < 0 {
		return ErrNoSite
	}
	s.Sites = append(s.Sites[:i], s.Sites[i+1:]...)
	return nil
}

// Duplicate copies a site under a new ID and name ("Name (copy)").
func (s *Store) Duplicate(id string) (string, error) {
	site, ok := s.Get(id)
	if !ok {
		return "", ErrNoSite
	}
	site.ID = ""
	site.Name += " (copy)"
	return s.Add(site)
}

// Move puts a site into another folder (created if needed).
func (s *Store) Move(id, folder string) error {
	i := s.index(id)
	if i < 0 {
		return ErrNoSite
	}
	s.Sites[i].Folder = cleanFolder(folder)
	s.ensureFolder(s.Sites[i].Folder)
	return nil
}

// Find returns the site with the given tree path ("Folder/Name") or, when
// that fails, the first site with that name.
func (s *Store) Find(pathOrName string) (Site, bool) {
	for _, site := range s.Sites {
		if strings.EqualFold(site.Path(), pathOrName) {
			return site, true
		}
	}
	for _, site := range s.Sites {
		if strings.EqualFold(site.Name, pathOrName) {
			return site, true
		}
	}
	return Site{}, false
}

func (s *Store) ensureFolder(f string) {
	if f == "" {
		return
	}
	parts := strings.Split(f, "/")
	for i := 1; i <= len(parts); i++ {
		p := strings.Join(parts[:i], "/")
		found := false
		for _, have := range s.Folders {
			if have == p {
				found = true
				break
			}
		}
		if !found {
			s.Folders = append(s.Folders, p)
		}
	}
	sort.Strings(s.Folders)
}

// AddFolder creates a (possibly nested) folder.
func (s *Store) AddFolder(folder string) error {
	f := cleanFolder(folder)
	if f == "" {
		return errors.New("ftp: empty folder name")
	}
	s.ensureFolder(f)
	return nil
}

func inFolder(f, folder string) bool {
	return f == folder || strings.HasPrefix(f, folder+"/")
}

// RenameFolder renames a folder, moving its subfolders and sites along.
func (s *Store) RenameFolder(from, to string) error {
	from, to = cleanFolder(from), cleanFolder(to)
	if from == "" || to == "" {
		return errors.New("ftp: empty folder name")
	}
	if inFolder(to, from) {
		return errors.New("ftp: cannot move a folder into itself")
	}
	found := false
	for i, f := range s.Folders {
		if inFolder(f, from) {
			s.Folders[i] = to + f[len(from):]
			found = true
		}
	}
	for i := range s.Sites {
		if inFolder(s.Sites[i].Folder, from) {
			s.Sites[i].Folder = to + s.Sites[i].Folder[len(from):]
			found = true
		}
	}
	if !found {
		return fmt.Errorf("ftp: no folder %q", from)
	}
	s.ensureFolder(to)
	s.dedupeFolders()
	return nil
}

func (s *Store) dedupeFolders() {
	sort.Strings(s.Folders)
	out := s.Folders[:0]
	for i, f := range s.Folders {
		if i == 0 || f != s.Folders[i-1] {
			out = append(out, f)
		}
	}
	s.Folders = out
}

// RemoveFolder deletes a folder. Unless recursive, it must be empty (no
// sites and no subfolders); recursive removal deletes the sites too.
func (s *Store) RemoveFolder(folder string, recursive bool) error {
	f := cleanFolder(folder)
	if f == "" {
		return errors.New("ftp: empty folder name")
	}
	var keepSites []Site
	for _, site := range s.Sites {
		if inFolder(site.Folder, f) {
			if !recursive {
				return fmt.Errorf("ftp: folder %q is not empty", f)
			}
			continue
		}
		keepSites = append(keepSites, site)
	}
	var keepFolders []string
	for _, have := range s.Folders {
		if have != f && inFolder(have, f) && !recursive {
			return fmt.Errorf("ftp: folder %q is not empty", f)
		}
		if !inFolder(have, f) {
			keepFolders = append(keepFolders, have)
		}
	}
	s.Sites, s.Folders = keepSites, keepFolders
	return nil
}

// Children lists the direct subfolders (names only) and the sites of a
// folder ("" = top level), both sorted by name, for building the tree.
func (s *Store) Children(folder string) ([]string, []Site) {
	f := cleanFolder(folder)
	var subs []string
	for _, have := range s.Folders {
		rest := have
		if f != "" {
			if !strings.HasPrefix(have, f+"/") {
				continue
			}
			rest = have[len(f)+1:]
		}
		if rest != "" && !strings.Contains(rest, "/") {
			subs = append(subs, rest)
		}
	}
	var sites []Site
	for _, site := range s.Sites {
		if site.Folder == f {
			sites = append(sites, site)
		}
	}
	sort.Strings(subs)
	sort.SliceStable(sites, func(i, j int) bool {
		return strings.ToLower(sites[i].Name) < strings.ToLower(sites[j].Name)
	})
	return subs, sites
}

// HasMasterPassword reports whether a master password was set.
func (s *Store) HasMasterPassword() bool { return s.Crypto != nil }

// SetMasterPassword sets the first master password and returns its key.
// Use ChangeMasterPassword when one is already set.
func (s *Store) SetMasterPassword(password string, iterations int) (*MasterKey, error) {
	if s.Crypto != nil {
		return nil, errors.New("ftp: a master password is already set")
	}
	k, h, err := NewMasterKey(password, iterations)
	if err != nil {
		return nil, err
	}
	s.Crypto = &h
	return k, nil
}

// Unlock checks the master password and returns the key for it.
func (s *Store) Unlock(password string) (*MasterKey, error) {
	if s.Crypto == nil {
		return nil, errors.New("ftp: no master password set")
	}
	return OpenMasterKey(password, *s.Crypto)
}

// ChangeMasterPassword re-seals every stored password with a new master
// password. old must be the key of the current one.
func (s *Store) ChangeMasterPassword(old *MasterKey, password string, iterations int) (*MasterKey, error) {
	k, h, err := NewMasterKey(password, iterations)
	if err != nil {
		return nil, err
	}
	reseal := func(v string) (string, error) {
		if !strings.HasPrefix(v, sealedPrefix) {
			return v, nil // empty, or a keychain reference
		}
		if old == nil {
			return "", ErrLocked
		}
		plain, err := old.Open(v)
		if err != nil {
			return "", err
		}
		return k.Seal(plain)
	}
	sites := append([]Site(nil), s.Sites...)
	proxies := append([]NamedProxy(nil), s.Proxies...)
	for i := range sites {
		if sites[i].Password, err = reseal(sites[i].Password); err != nil {
			return nil, err
		}
	}
	for i := range proxies {
		if proxies[i].Password, err = reseal(proxies[i].Password); err != nil {
			return nil, err
		}
	}
	s.Sites, s.Proxies, s.Crypto = sites, proxies, &h
	return k, nil
}

// SetPassword seals and stores a site's password ("" clears it).
func (s *Store) SetPassword(id, plain string, box SecretBox) error {
	i := s.index(id)
	if i < 0 {
		return ErrNoSite
	}
	if plain == "" {
		s.Sites[i].Password = ""
		return nil
	}
	if box == nil {
		return ErrLocked
	}
	sealed, err := box.Seal(plain)
	if err != nil {
		return err
	}
	s.Sites[i].Password = sealed
	return nil
}

// Password returns a site's plain password ("" when none is saved).
func (s *Store) Password(id string, box SecretBox) (string, error) {
	site, ok := s.Get(id)
	if !ok {
		return "", ErrNoSite
	}
	if site.Password == "" {
		return "", nil
	}
	if box == nil {
		return "", ErrLocked
	}
	return box.Open(site.Password)
}

// SetProxy adds or replaces a named proxy, sealing its password.
func (s *Store) SetProxy(name string, p Proxy, box SecretBox) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("ftp: a proxy needs a name")
	}
	np := NamedProxy{Name: name, Proxy: p}
	if p.Password != "" {
		if box == nil {
			return ErrLocked
		}
		sealed, err := box.Seal(p.Password)
		if err != nil {
			return err
		}
		np.Password = sealed
	}
	np.Proxy.Password = ""
	for i := range s.Proxies {
		if s.Proxies[i].Name == name {
			s.Proxies[i] = np
			return nil
		}
	}
	s.Proxies = append(s.Proxies, np)
	return nil
}

// ProxyByName returns a named proxy with its password opened.
func (s *Store) ProxyByName(name string, box SecretBox) (*Proxy, error) {
	for _, np := range s.Proxies {
		if np.Name != name {
			continue
		}
		p := np.Proxy
		if np.Password != "" {
			if box == nil {
				return nil, ErrLocked
			}
			pw, err := box.Open(np.Password)
			if err != nil {
				return nil, err
			}
			p.Password = pw
		}
		return &p, nil
	}
	return nil, fmt.Errorf("ftp: no proxy named %q", name)
}

// Config builds the connection settings for a saved FTP site. box may be
// nil when neither the site nor its proxy has a saved password; otherwise
// the caller asks for the master password first.
func (s *Store) Config(id string, box SecretBox) (Config, error) {
	site, ok := s.Get(id)
	if !ok {
		return Config{}, ErrNoSite
	}
	cfg, err := site.Config()
	if err != nil {
		return Config{}, err
	}
	if cfg.Password, err = s.Password(id, box); err != nil {
		return Config{}, err
	}
	if site.ProxyName != "" {
		if cfg.Proxy, err = s.ProxyByName(site.ProxyName, box); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// Config converts the site's settings (without password and proxy) into a
// Config. SFTP sites are rejected.
func (site Site) Config() (Config, error) {
	cfg := Config{Host: site.Host, Port: site.Port, User: site.User, InitialDir: site.RemoteDir}
	switch site.Protocol {
	case ProtoFTP, "":
		if site.TryTLS {
			cfg.Security = SecurityExplicitTLSIfAvailable
		}
	case ProtoFTPES:
		cfg.Security = SecurityExplicitTLS
	case ProtoFTPS:
		cfg.Security = SecurityImplicitTLS
	default:
		return Config{}, fmt.Errorf("ftp: %q is a %s connection", site.Name, site.Protocol)
	}
	if site.Active {
		cfg.DataMode = Active
	}
	cfg.TransferMode = ParseTransferMode(site.Transfer)
	cfg.ASCIIMasks = site.ASCIIMasks
	cfg.Encoding = ParseEncoding(site.Encoding)
	cfg.TimeOffset = time.Duration(site.TimeOffset) * time.Minute
	cfg.Keepalive = time.Duration(site.Keepalive) * time.Second
	cfg.ListCommand = site.ListCommand
	cfg.DisableMLSD = site.NoMLSD
	cfg.DisableEPSV = site.NoEPSV
	return cfg, nil
}

// SiteFromURL makes an unsaved site from a quick-connect URL, for "save
// this connection" after connecting by URL. The URL's password is not kept.
func SiteFromURL(name, raw string) (Site, error) {
	cfg, p, err := ParseURL(raw)
	if err != nil {
		return Site{}, err
	}
	site := Site{Name: name, Host: cfg.Host, Port: cfg.Port, RemoteDir: p, Protocol: ProtoFTP}
	if !cfg.IsAnonymous() {
		site.User = cfg.User
	}
	switch cfg.Security {
	case SecurityExplicitTLS:
		site.Protocol = ProtoFTPES
	case SecurityImplicitTLS:
		site.Protocol = ProtoFTPS
	}
	if cfg.DataMode == Active {
		site.Active = true
	}
	if cfg.TransferMode != TransferDefault {
		site.Transfer = cfg.TransferMode.String()
	}
	if site.Name == "" {
		site.Name = cfg.Host
	}
	return site, nil
}
