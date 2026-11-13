// This file tests the connection manager model: adding, editing, moving and
// deleting saved connections and folders, saving and loading the JSON file,
// the guarantee that passwords never reach the disk in plain text, the
// master password (right, wrong, changed), turning a site into a working
// Config, and importing Total Commander's wcx_ftp.ini.
package ftp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testIter = 1000 // keep PBKDF2 fast in tests

func TestStoreCRUDAndFolders(t *testing.T) {
	s := &Store{}
	id1, err := s.Add(Site{Name: "Alpha", Folder: "Work/Clients", Host: "a.example"})
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := s.Add(Site{Name: "beta", Host: "b.example", Protocol: ProtoFTPS})
	id3, _ := s.Add(Site{Name: "Gamma", Folder: `Work\Clients\`, Host: "g.example"})
	if _, err := s.Add(Site{Host: "x"}); err == nil {
		t.Error("site without name accepted")
	}
	if _, err := s.Add(Site{Name: "x", Password: "plain"}); err == nil {
		t.Error("plain password accepted by Add")
	}
	if got := strings.Join(s.Folders, "|"); got != "Work|Work/Clients" {
		t.Fatalf("folders = %s", got)
	}
	subs, sites := s.Children("")
	if strings.Join(subs, "|") != "Work" || len(sites) != 1 || sites[0].ID != id2 {
		t.Fatalf("top level = %v %v", subs, sites)
	}
	subs, sites = s.Children("Work/Clients")
	if len(subs) != 0 || len(sites) != 2 || sites[0].Name != "Alpha" || sites[1].Name != "Gamma" {
		t.Fatalf("Work/Clients = %v %v", subs, sites)
	}
	if site, ok := s.Find("work/clients/gamma"); !ok || site.ID != id3 {
		t.Error("Find by path failed")
	}
	if site, ok := s.Find("BETA"); !ok || site.ID != id2 {
		t.Error("Find by name failed")
	}

	site, _ := s.Get(id1)
	site.Host = "new.example"
	if err := s.Update(site); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(id1); got.Host != "new.example" {
		t.Error("Update lost")
	}
	if err := s.Move(id2, "Private"); err != nil {
		t.Fatal(err)
	}
	dup, err := s.Duplicate(id2)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Get(dup); d.Name != "beta (copy)" || d.Folder != "Private" || d.ID == id2 {
		t.Errorf("duplicate = %+v", d)
	}

	if err := s.RenameFolder("Work", "Job"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(id3); got.Folder != "Job/Clients" {
		t.Errorf("site folder after rename = %q", got.Folder)
	}
	if strings.Join(s.Folders, "|") != "Job|Job/Clients|Private" {
		t.Errorf("folders after rename = %v", s.Folders)
	}
	if err := s.RenameFolder("Job", "Job/Sub"); err == nil {
		t.Error("folder moved into itself")
	}
	if err := s.RemoveFolder("Job", false); err == nil {
		t.Error("non-empty folder removed")
	}
	s.AddFolder("Empty/Nested")
	if err := s.RemoveFolder("Empty", false); err == nil {
		t.Error("folder with subfolder removed without recursive")
	}
	if err := s.RemoveFolder("Empty", true); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveFolder("Job", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(id1); ok {
		t.Error("recursive folder removal kept a site")
	}
	if err := s.Remove(id2); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(id2); !errors.Is(err, ErrNoSite) {
		t.Errorf("double remove: %v", err)
	}
	if len(s.Sites) != 1 || strings.Join(s.Folders, "|") != "Private" {
		t.Errorf("left: %+v %v", s.Sites, s.Folders)
	}
}

func TestStorePasswordsAndPersistence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cfg", "ftp.json")
	s, err := LoadStore(file)
	if err != nil || len(s.Sites) != 0 {
		t.Fatalf("LoadStore missing file = %+v, %v", s, err)
	}
	id, _ := s.Add(Site{Name: "Srv", Host: "h", User: "peter"})
	if err := s.SetPassword(id, "hunter2", nil); !errors.Is(err, ErrLocked) {
		t.Fatalf("SetPassword without box: %v", err)
	}
	key, err := s.SetMasterPassword("master!", testIter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetMasterPassword("again", testIter); err == nil {
		t.Error("second SetMasterPassword accepted")
	}
	if err := s.SetPassword(id, "hunter2", key); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProxy("corp", Proxy{Type: ProxySOCKS5, Host: "proxy", User: "pu", Password: "proxysecret"}, key); err != nil {
		t.Fatal(err)
	}
	site, _ := s.Get(id)
	site.ProxyName = "corp"
	s.Update(site)
	if err := s.Save(file); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	for _, secret := range []string{"hunter2", "proxysecret", "master!"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("%q stored in plain text:\n%s", secret, raw)
		}
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(file); fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v", fi.Mode())
		}
	}

	s2, err := LoadStore(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Unlock("wrong"); !errors.Is(err, ErrWrongMasterPass) {
		t.Fatalf("Unlock wrong: %v", err)
	}
	key2, err := s2.Unlock("master!")
	if err != nil {
		t.Fatal(err)
	}
	if pw, err := s2.Password(id, key2); err != nil || pw != "hunter2" {
		t.Fatalf("Password = %q, %v", pw, err)
	}
	if _, err := s2.Password(id, nil); !errors.Is(err, ErrLocked) {
		t.Fatalf("Password locked: %v", err)
	}
	cfg, err := s2.Config(id, key2)
	if err != nil || cfg.Password != "hunter2" || cfg.Proxy == nil || cfg.Proxy.Password != "proxysecret" || cfg.User != "peter" {
		t.Fatalf("Config = %+v, %v", cfg, err)
	}

	// Changing the master password re-seals everything.
	key3, err := s2.ChangeMasterPassword(key2, "new master", testIter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Unlock("master!"); !errors.Is(err, ErrWrongMasterPass) {
		t.Error("old master password still works")
	}
	if pw, _ := s2.Password(id, key3); pw != "hunter2" {
		t.Errorf("password after change = %q", pw)
	}
	if _, err := s2.Password(id, key2); err == nil {
		t.Error("old key still opens passwords")
	}

	// A plain password can never be saved, even if forced into the struct.
	s2.Sites[0].Password = "oops"
	if err := s2.Save(file); err == nil {
		t.Fatal("plain password saved")
	}
	s2.Sites[0].Password = KeyRefPrefix + "mormicommander/Srv"
	if err := s2.Save(file); err != nil {
		t.Fatalf("keychain reference rejected: %v", err)
	}
	// Clearing a password.
	if err := s2.SetPassword(id, "", nil); err != nil {
		t.Fatal(err)
	}
	if pw, err := s2.Password(id, nil); err != nil || pw != "" {
		t.Errorf("cleared password = %q, %v", pw, err)
	}
}

func TestLoadStoreErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o600)
	if _, err := LoadStore(bad); err == nil {
		t.Error("broken JSON accepted")
	}
	newer := filepath.Join(dir, "newer.json")
	os.WriteFile(newer, []byte(`{"version": 99, "sites": []}`), 0o600)
	if _, err := LoadStore(newer); err == nil {
		t.Error("newer version accepted")
	}
}

func TestSiteConfig(t *testing.T) {
	tests := []struct {
		site Site
		ok   bool
		sec  Security
		port int
	}{
		{Site{Name: "a", Host: "h"}, true, SecurityNone, 21},
		{Site{Name: "a", Host: "h", TryTLS: true}, true, SecurityExplicitTLSIfAvailable, 21},
		{Site{Name: "a", Host: "h", Protocol: ProtoFTPES}, true, SecurityExplicitTLS, 21},
		{Site{Name: "a", Host: "h", Protocol: ProtoFTPS}, true, SecurityImplicitTLS, 990},
		{Site{Name: "a", Host: "h", Protocol: ProtoFTPS, Port: 9990}, true, SecurityImplicitTLS, 9990},
		{Site{Name: "a", Host: "h", Protocol: ProtoSFTP}, false, 0, 0},
	}
	for _, tt := range tests {
		cfg, err := tt.site.Config()
		if (err == nil) != tt.ok {
			t.Errorf("%+v: err = %v", tt.site, err)
			continue
		}
		if tt.ok && (cfg.Security != tt.sec || cfg.EffectivePort() != tt.port) {
			t.Errorf("%+v: got %v port %d", tt.site, cfg.Security, cfg.EffectivePort())
		}
	}
	site := Site{Name: "x", Host: "h", Active: true, Transfer: "ascii", Encoding: "latin1", TimeOffset: -90,
		Keepalive: 30, ListCommand: "LIST -a", NoMLSD: true, NoEPSV: true, RemoteDir: "/start"}
	cfg, _ := site.Config()
	if cfg.DataMode != Active || cfg.TransferMode != TransferASCII || cfg.Encoding != EncodingLatin1 ||
		cfg.TimeOffset != -90*time.Minute || cfg.Keepalive != 30*time.Second || cfg.ListCommand != "LIST -a" ||
		!cfg.DisableMLSD || !cfg.DisableEPSV || cfg.InitialDir != "/start" {
		t.Errorf("Config = %+v", cfg)
	}

	s2, err := SiteFromURL("", "ftpes://peter:pw@host:2121/dir?mode=active")
	if err != nil || s2.Protocol != ProtoFTPES || s2.User != "peter" || s2.Password != "" || s2.Port != 2121 ||
		s2.RemoteDir != "/dir" || !s2.Active || s2.Name != "host" {
		t.Errorf("SiteFromURL = %+v, %v", s2, err)
	}
}

func TestStoreConfigConnects(t *testing.T) {
	srv := newTestServer(t, func(s *testServer) { s.user, s.pass = "peter", "pw" })
	srv.write("home/f", "1")
	s := &Store{}
	key, _ := s.SetMasterPassword("m", testIter)
	id, _ := s.Add(Site{Name: "local", Host: "127.0.0.1", Port: srv.port, User: "peter", RemoteDir: "/home"})
	s.SetPassword(id, "pw", key)
	cfg, err := s.Config(id, key)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Timeout = 5 * time.Second
	c := dialT(t, cfg)
	if p, _ := c.Pwd(context.Background()); p != "/home" {
		t.Fatalf("Pwd = %q", p)
	}
}

func TestImportWcxFtpINI(t *testing.T) {
	ini := "\ufeff; Total Commander FTP connections\r\n" +
		"[connections]\r\n" +
		"2=Second\r\n" +
		"1=Group\\First\r\n" +
		"3=Missing\r\n" +
		"[default]\r\n" +
		"pasvmode=1\r\n" +
		"[Group\\First]\r\n" +
		"host=ftp.first.example:2121\r\n" +
		"username=peter\r\n" +
		"password=0123456789ABCDEF\r\n" +
		"directory=/pub\r\n" +
		"localdir=C:\\Downloads\r\n" +
		"pasvmode=0\r\n" +
		"transfermode=A\r\n" +
		"utf8=1\r\n" +
		"[second]\r\n" +
		"host=ftps://secure.example\r\n" +
		"username=anonymous\r\n" +
		"transfermode=I\r\n" +
		"keepalive=60\r\n"
	sites, err := ImportWcxFtpINI(strings.NewReader(ini))
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 {
		t.Fatalf("got %d sites: %+v", len(sites), sites)
	}
	a := sites[0]
	if a.Name != "First" || a.Folder != "Group" || a.Host != "ftp.first.example" || a.Port != 2121 || a.User != "peter" ||
		a.RemoteDir != "/pub" || a.LocalDir != `C:\Downloads` || !a.Active || a.Transfer != "ascii" || a.Encoding != "utf-8" ||
		a.Password != "" || a.Protocol != ProtoFTP {
		t.Errorf("first = %+v", a)
	}
	b := sites[1]
	if b.Name != "Second" || b.Protocol != ProtoFTPS || b.Host != "secure.example" || b.User != "" || b.Transfer != "binary" ||
		b.Keepalive != 60 || b.Active {
		t.Errorf("second = %+v", b)
	}
	s := &Store{}
	for _, site := range sites {
		if _, err := s.Add(site); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Folders) != 1 || s.Folders[0] != "Group" {
		t.Errorf("folders = %v", s.Folders)
	}
	for _, tc := range []struct {
		in   string
		host string
		port int
		ok   bool
	}{{"h:21", "h", 21, true}, {"[::1]:2121", "::1", 2121, true}, {"::1", "::1", 0, false}, {"h", "h", 0, false}} {
		h, p, ok := cutPort(tc.in)
		if h != tc.host || p != tc.port || ok != tc.ok {
			t.Errorf("cutPort(%q) = %q %d %v", tc.in, h, p, ok)
		}
	}
}

func TestMasterKeyDirect(t *testing.T) {
	k, h, err := NewMasterKey("pw", testIter)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := k.Seal("same")
	b, _ := k.Seal("same")
	if a == b {
		t.Error("sealing is deterministic; nonce reuse")
	}
	if !IsSealed(a) || IsSealed("plain") {
		t.Error("IsSealed wrong")
	}
	if got, err := k.Open(a); err != nil || got != "same" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	tampered := a[:len(a)-4] + "AAAA"
	if _, err := k.Open(tampered); err == nil {
		t.Error("tampered value opened")
	}
	if _, err := k.Open("plain"); err == nil {
		t.Error("unsealed value opened")
	}
	if _, _, err := NewMasterKey("", testIter); err == nil {
		t.Error("empty master password accepted")
	}
	h.KDF = "rot13"
	if _, err := OpenMasterKey("pw", h); err == nil {
		t.Error("unknown KDF accepted")
	}
}
