package main

// The connection manager's list (Ctrl+F, F-377, F-634, F-635): saved servers in folders, kept by internal/ftp's Store
// in ftpsites.json in the settings folder. FTP, SFTP and WebDAV sites share the list (Protocol tells them apart).
// Passwords are never written in plain text: saving one needs TC's master password, which seals it with AES-GCM; the
// master password is asked once per run, when a saved password is first needed. A site without a saved password asks
// for it when connecting.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"mormicommander/internal/ftp"
)

// NetSite is a saved connection as the page edits it.
type NetSite struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Folder       string `json:"folder"`   // "Work/Clients"; "" is the top level
	Protocol     string `json:"protocol"` // ftp, ftpes, ftps, sftp, webdav, webdavs
	Host         string `json:"host"`
	Port         int    `json:"port"` // 0 = the protocol's default
	User         string `json:"user"`
	HasPassword  bool   `json:"hasPassword"`  // a password is saved
	Password     string `json:"password"`     // a new password to save (only sent by the page)
	SavePassword bool   `json:"savePassword"` // keep a password (sealed with the master password); false asks each time
	KeyFile      string `json:"keyFile"`      // SFTP private key
	UseAgent     bool   `json:"useAgent"`     // SFTP: try the keys of the SSH agent
	RemoteDir    string `json:"remoteDir"`
	LocalDir     string `json:"localDir"` // shown in the other panel after connecting
	Passive      bool   `json:"passive"`
	TryTLS       bool   `json:"tryTLS"`   // plain FTP: encrypt when the server offers it
	Transfer     string `json:"transfer"` // auto, binary, ascii
	Encoding     string `json:"encoding"` // auto, utf-8, latin1
	ShowHidden   bool   `json:"showHidden"`
	ListCommand  string `json:"listCommand"`
	TimeOffset   int    `json:"timeOffset"` // minutes added to the server's times
	Keepalive    int    `json:"keepalive"`  // seconds between keep-alive messages, 0 = off
	NoMLSD       bool   `json:"noMLSD"`
	Comment      string `json:"comment"`
	LastUsed     int64  `json:"lastUsed"` // Unix milliseconds, 0 = never
}

// NetSiteList is the whole connection list.
type NetSiteList struct {
	Folders   []string  `json:"folders"`
	Sites     []NetSite `json:"sites"`
	HasMaster bool      `json:"hasMaster"` // a master password is set
	Unlocked  bool      `json:"unlocked"`  // and typed in this run
}

// netSiteExtra holds what ftp.Site has no field for (settings key "netSites", by site ID).
type netSiteExtra struct {
	NoAgent bool `json:"noAgent,omitempty"`
}

// loadStore reads the connection list once.
func (n *Net) loadStore() (*ftp.Store, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.loadStoreLocked()
}

func (n *Net) loadStoreLocked() (*ftp.Store, error) {
	if n.store != nil {
		return n.store, nil
	}
	if n.storePath == "" {
		n.store = &ftp.Store{Version: 1}
		return n.store, nil
	}
	st, err := ftp.LoadStore(n.storePath)
	if err != nil {
		return nil, errors.New("The list of saved connections can't be read: " + strings.TrimPrefix(err.Error(), "ftp: "))
	}
	n.store = st
	return st, nil
}

func (n *Net) saveStoreLocked() error {
	if n.store == nil || n.storePath == "" {
		return nil
	}
	if err := n.store.Save(n.storePath); err != nil {
		return errors.New("The list of saved connections can't be saved: " + strings.TrimPrefix(err.Error(), "ftp: "))
	}
	return nil
}

// netStoreErr turns a store error into a sentence.
func netStoreErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ftp.ErrWrongMasterPass):
		return errors.New("The master password is wrong.")
	case errors.Is(err, ftp.ErrLocked):
		return errors.New("Saved passwords are locked: the master password is needed.")
	case errors.Is(err, ftp.ErrNoSite):
		return errors.New("This saved connection is not there any more.")
	}
	s := strings.TrimPrefix(err.Error(), "ftp: ")
	return errors.New(strings.ToUpper(s[:1]) + s[1:] + ".")
}

func netSiteIndex(st *ftp.Store, id string) int {
	for i := range st.Sites {
		if st.Sites[i].ID == id {
			return i
		}
	}
	return -1
}

// unlockIfNeeded returns the master password's key, asking for it once per run. Without a master password it returns
// nil (no password can be saved then).
func (n *Net) unlockIfNeeded() (ftp.SecretBox, error) {
	st, err := n.loadStore()
	if err != nil {
		return nil, err
	}
	n.mu.Lock()
	k := n.key
	n.mu.Unlock()
	if k != nil {
		return k, nil
	}
	if !st.HasMasterPassword() {
		return nil, nil
	}
	msg := "Saved passwords are protected by the master password."
	for attempt := 0; attempt < 3; attempt++ {
		pw, ok := n.askText("master", "Master password", msg, true)
		if !ok {
			return nil, errors.New("The saved password can't be used without the master password.")
		}
		key, err := st.Unlock(pw)
		if err == nil {
			n.mu.Lock()
			n.key = key
			n.mu.Unlock()
			return key, nil
		}
		if !errors.Is(err, ftp.ErrWrongMasterPass) {
			return nil, netStoreErr(err)
		}
		msg = "That was not the master password. Try again."
	}
	return nil, errors.New("The master password is wrong.")
}

// boxForSaving returns a key to seal a password with: the master password, set up first when there is none.
func (n *Net) boxForSaving() (ftp.SecretBox, error) {
	st, err := n.loadStore()
	if err != nil {
		return nil, err
	}
	if st.HasMasterPassword() {
		return n.unlockIfNeeded()
	}
	pw, ok := n.askText("master-new", "Choose a master password",
		"Saved passwords are encrypted with a master password. You type it once each time you start mormiCommander and use a saved password.", true)
	if !ok || pw == "" {
		return nil, errors.New("The password was not saved: it needs a master password.")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	key, err := st.SetMasterPassword(pw, ftp.DefaultKDFIterations)
	if err != nil {
		return nil, netStoreErr(err)
	}
	n.key = key
	if err := n.saveStoreLocked(); err != nil {
		return nil, err
	}
	return key, nil
}

// ChangeMasterPassword sets or changes the master password (F-635). Saved passwords are sealed again with the new one.
func (n *Net) ChangeMasterPassword() error {
	st, err := n.loadStore()
	if err != nil {
		return err
	}
	if !st.HasMasterPassword() {
		_, err := n.boxForSaving()
		return err
	}
	box, err := n.unlockIfNeeded()
	if err != nil {
		return err
	}
	pw, ok := n.askText("master-new", "New master password", "Type the new master password.", true)
	if !ok || pw == "" {
		return errNetCancelled
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	old, _ := box.(*ftp.MasterKey)
	key, err := st.ChangeMasterPassword(old, pw, ftp.DefaultKDFIterations)
	if err != nil {
		return netStoreErr(err)
	}
	n.key = key
	return n.saveStoreLocked()
}

// siteExtra reads the extra settings of a site.
func (n *Net) siteExtra(id string) netSiteExtra {
	var all map[string]netSiteExtra
	if n.app != nil && n.app.settings != nil {
		if v := n.app.settings.Get("netSites"); v != nil {
			if b, err := json.Marshal(v); err == nil {
				json.Unmarshal(b, &all)
			}
		}
	}
	return all[id]
}

func (n *Net) setSiteExtra(id string, e *netSiteExtra) {
	if n.app == nil || n.app.settings == nil {
		return
	}
	all := map[string]netSiteExtra{}
	if v := n.app.settings.Get("netSites"); v != nil {
		if b, err := json.Marshal(v); err == nil {
			json.Unmarshal(b, &all)
		}
	}
	if e == nil || *e == (netSiteExtra{}) {
		delete(all, id)
	} else {
		all[id] = *e
	}
	n.app.settings.put("netSites", all)
}

// listHasHidden says whether a list command asks for hidden files ("LIST -a", "LIST -la").
func listHasHidden(lc string) bool {
	f := strings.Fields(lc)
	for _, a := range f[min(1, len(f)):] {
		if strings.HasPrefix(a, "-") && strings.ContainsRune(a, 'a') {
			return true
		}
	}
	return false
}

func (n *Net) toNetSite(s ftp.Site) NetSite {
	ns := NetSite{ID: s.ID, Name: s.Name, Folder: s.Folder, Protocol: string(s.Protocol), Host: s.Host, Port: s.Port, User: s.User,
		HasPassword: s.Password != "", SavePassword: s.Password != "", KeyFile: s.KeyFile, RemoteDir: s.RemoteDir, LocalDir: s.LocalDir,
		Passive: !s.Active, TryTLS: s.TryTLS, Transfer: s.Transfer, Encoding: s.Encoding, ShowHidden: listHasHidden(s.ListCommand),
		ListCommand: s.ListCommand, TimeOffset: s.TimeOffset, Keepalive: s.Keepalive, NoMLSD: s.NoMLSD, Comment: s.Comment,
		UseAgent: !n.siteExtra(s.ID).NoAgent}
	if ns.Protocol == "" {
		ns.Protocol = "ftp"
	}
	if strings.EqualFold(strings.TrimSpace(s.ListCommand), "LIST -a") {
		ns.ListCommand = ""
	}
	if !s.LastUsed.IsZero() {
		ns.LastUsed = s.LastUsed.UnixMilli()
	}
	return ns
}

// Sites returns the saved connections and their folders.
func (n *Net) Sites() (NetSiteList, error) {
	st, err := n.loadStore()
	if err != nil {
		return NetSiteList{}, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	out := NetSiteList{Folders: append([]string{}, st.Folders...), Sites: []NetSite{}, HasMaster: st.HasMasterPassword(), Unlocked: n.key != nil}
	for _, s := range st.Sites {
		out.Sites = append(out.Sites, n.toNetSite(s))
	}
	sort.SliceStable(out.Sites, func(i, j int) bool {
		if out.Sites[i].Folder != out.Sites[j].Folder {
			return out.Sites[i].Folder < out.Sites[j].Folder
		}
		return strings.ToLower(out.Sites[i].Name) < strings.ToLower(out.Sites[j].Name)
	})
	sort.Strings(out.Folders)
	return out, nil
}

// SaveSite adds a new saved connection (empty ID) or changes one, and returns its ID.
func (n *Net) SaveSite(ns NetSite) (string, error) {
	ns.Name, ns.Host = strings.TrimSpace(ns.Name), strings.TrimSpace(ns.Host)
	if ns.Name == "" {
		return "", errors.New("Give the connection a name.")
	}
	if ns.Host == "" {
		return "", errors.New("Type the server's name or address.")
	}
	if netSchemes[ns.Protocol] == "" || ns.Protocol == "scp" || ns.Protocol == "dav" || ns.Protocol == "davs" {
		ns.Protocol = "ftp"
	}
	if ns.Port < 0 || ns.Port > 65535 {
		return "", errors.New("The port must be a number from 1 to 65535.")
	}
	// a host typed as a URL ("ftp://example.com:2121/pub") fills in the other fields
	if strings.Contains(ns.Host, "://") || strings.ContainsAny(ns.Host, "/@") {
		u, err := netParseURL(ns.Host)
		if err != nil {
			return "", err
		}
		ns.Protocol, ns.Host = u.Scheme, u.Host
		if u.Port != 0 && ns.Port == 0 {
			ns.Port = u.Port
		}
		if u.User != "" && ns.User == "" && !(u.Scheme == "sftp" && u.User == netLocalUser()) {
			ns.User = u.User
		}
		if u.Path != "" && u.Path != "/" && ns.RemoteDir == "" {
			ns.RemoteDir = u.Path
		}
		if u.Password != "" && ns.Password == "" {
			ns.Password = u.Password
		}
	}
	// "host:port" (but not a bare IPv6 address) gives the port too
	if h, p, ok := strings.Cut(ns.Host, ":"); ok && !strings.Contains(p, ":") {
		port, err := strconv.Atoi(p)
		if err != nil || port <= 0 || port > 65535 {
			return "", fmt.Errorf("%s is not a port number (1 to 65535).", p)
		}
		ns.Host = h
		if ns.Port == 0 {
			ns.Port = port
		}
	}
	ns.Host = strings.Trim(ns.Host, "[]")
	lc := strings.TrimSpace(ns.ListCommand)
	if ns.ShowHidden && !listHasHidden(lc) {
		if lc == "" {
			lc = "LIST -a"
		} else {
			lc += " -a"
		}
	}
	site := ftp.Site{ID: ns.ID, Name: ns.Name, Folder: ns.Folder, Protocol: ftp.Protocol(ns.Protocol), Host: ns.Host, Port: ns.Port,
		User: strings.TrimSpace(ns.User), KeyFile: strings.TrimSpace(ns.KeyFile), RemoteDir: strings.TrimSpace(ns.RemoteDir),
		LocalDir: strings.TrimSpace(ns.LocalDir), Active: !ns.Passive, TryTLS: ns.TryTLS, Transfer: ns.Transfer, Encoding: ns.Encoding,
		ListCommand: lc, TimeOffset: ns.TimeOffset, Keepalive: ns.Keepalive, NoMLSD: ns.NoMLSD, Comment: ns.Comment}
	if site.Transfer == "auto" {
		site.Transfer = ""
	}
	if site.Encoding == "auto" {
		site.Encoding = ""
	}
	var box ftp.SecretBox
	if ns.SavePassword && ns.Password != "" {
		var err error
		if box, err = n.boxForSaving(); err != nil {
			return "", err
		}
	}
	st, err := n.loadStore()
	if err != nil {
		return "", err
	}
	id, err := n.saveSiteLocked(st, site, ns, box)
	if err != nil {
		return "", err
	}
	if ns.Protocol == "sftp" && !ns.UseAgent {
		n.setSiteExtra(id, &netSiteExtra{NoAgent: true})
	} else {
		n.setSiteExtra(id, nil)
	}
	return id, nil
}

// saveSiteLocked stores the site and its password and writes the list.
func (n *Net) saveSiteLocked(st *ftp.Store, site ftp.Site, ns NetSite, box ftp.SecretBox) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	id := site.ID
	var err error
	if id == "" {
		if id, err = st.Add(site); err != nil {
			return "", netStoreErr(err)
		}
	} else {
		i := netSiteIndex(st, id)
		if i < 0 {
			return "", netStoreErr(ftp.ErrNoSite)
		}
		site.LastUsed = st.Sites[i].LastUsed
		if err := st.Update(site); err != nil {
			return "", netStoreErr(err)
		}
	}
	switch {
	case !ns.SavePassword:
		st.SetPassword(id, "", nil)
	case ns.Password != "":
		if err := st.SetPassword(id, ns.Password, box); err != nil {
			return "", netStoreErr(err)
		}
	}
	return id, n.saveStoreLocked()
}

// DeleteSite removes a saved connection.
func (n *Net) DeleteSite(id string) error {
	st, err := n.loadStore()
	if err != nil {
		return err
	}
	n.mu.Lock()
	if err := st.Remove(id); err != nil {
		n.mu.Unlock()
		return netStoreErr(err)
	}
	err = n.saveStoreLocked()
	n.mu.Unlock()
	n.setSiteExtra(id, nil)
	return err
}

// CopySite duplicates a saved connection ("Name (copy)") and returns the new ID.
func (n *Net) CopySite(id string) (string, error) {
	st, err := n.loadStore()
	if err != nil {
		return "", err
	}
	n.mu.Lock()
	nid, err := st.Duplicate(id)
	if err != nil {
		n.mu.Unlock()
		return "", netStoreErr(err)
	}
	err = n.saveStoreLocked()
	n.mu.Unlock()
	if e := n.siteExtra(id); e != (netSiteExtra{}) {
		n.setSiteExtra(nid, &e)
	}
	return nid, err
}

// MoveSite puts a saved connection into another folder ("" = top level).
func (n *Net) MoveSite(id, folder string) error {
	return n.storeOp(func(st *ftp.Store) error { return st.Move(id, folder) })
}

// AddFolder makes a folder in the connection list ("Work/Clients" makes both).
func (n *Net) AddFolder(folder string) error {
	return n.storeOp(func(st *ftp.Store) error { return st.AddFolder(folder) })
}

// RenameFolder renames a folder of the connection list, with what is in it.
func (n *Net) RenameFolder(from, to string) error {
	return n.storeOp(func(st *ftp.Store) error { return st.RenameFolder(from, to) })
}

// DeleteFolder removes a folder of the connection list with the connections in it.
func (n *Net) DeleteFolder(folder string) error {
	return n.storeOp(func(st *ftp.Store) error { return st.RemoveFolder(folder, true) })
}

func (n *Net) storeOp(f func(st *ftp.Store) error) error {
	st, err := n.loadStore()
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := f(st); err != nil {
		return netStoreErr(err)
	}
	return n.saveStoreLocked()
}
