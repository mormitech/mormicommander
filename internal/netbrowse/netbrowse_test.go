package netbrowse

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in   string
		want Target
	}{
		{"nas", Target{Kind: "smb", Host: "nas"}},
		{"nas/media", Target{Kind: "smb", Host: "nas", Share: "media"}},
		{`\\NAS\Media\sub`, Target{Kind: "smb", Host: "NAS", Share: "Media"}},
		{"//nas/media", Target{Kind: "smb", Host: "nas", Share: "media"}},
		{"smb://pat@nas.local:4455/media/x", Target{Kind: "smb", Host: "nas.local", Port: 4455, User: "pat", Share: "media"}},
		{"cifs://nas/a", Target{Kind: "smb", Host: "nas", Share: "a"}},
		{"nfs://nas/volume1/media", Target{Kind: "nfs", Host: "nas", Share: "/volume1/media"}},
		{"afp://mac.local/Share", Target{Kind: "afp", Host: "mac.local", Share: "Share"}},
		{"smb://[fe80::1]/s", Target{Kind: "smb", Host: "fe80::1", Share: "s"}},
	}
	for _, c := range cases {
		got, err := ParseTarget(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseTarget(%q) = %+v, %v; want %+v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "ftp://x", `\\`, "smb://nas:99999/a", "smb://na;s/a", "smb:///share"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("ParseTarget(%q) accepted", bad)
		}
	}
}

func TestTargetForms(t *testing.T) {
	tg := Target{Kind: "smb", Host: "nas", User: "pat", Share: "my media"}
	if got := tg.URL(); got != "smb://pat@nas/my%20media" {
		t.Errorf("URL = %q", got)
	}
	if got := tg.UNC(); got != `\\nas\my media` {
		t.Errorf("UNC = %q", got)
	}
	if got := (Target{Kind: "nfs", Host: "nas", User: "x", Share: "/v/m"}).URL(); got != "nfs://nas/v/m" {
		t.Errorf("nfs URL = %q", got)
	}
	if got := (Target{Kind: "smb", Host: "NAS", User: "pat", Share: "a"}).Key(); got != "smb://pat@nas" {
		t.Errorf("Key = %q", got)
	}
}

func TestParseDNSSD(t *testing.T) {
	browse := `Browsing for _smb._tcp.local.
DATE: ---Sat 14 Nov 2026---
19:20:01.123  ...STARTING...
Timestamp     A/R    Flags  if Domain               Service Type         Instance Name
19:20:01.456  Add        3   4 local.               _smb._tcp.           My NAS
19:20:01.457  Add        3   4 local.               _smb._tcp.           Office PC
19:20:01.458  Add        2   6 local.               _smb._tcp.           My NAS
19:20:02.000  Rmv        0   4 local.               _smb._tcp.           Office PC
`
	if got := ParseDNSSDBrowse(browse); !reflect.DeepEqual(got, []string{"My NAS"}) {
		t.Errorf("browse = %q", got)
	}
	host, port := ParseDNSSDLookup("19:20:02.100  My\\032NAS._smb._tcp.local. can be reached at my-nas.local.:445 (interface 4)\n")
	if host != "my-nas.local" || port != 445 {
		t.Errorf("lookup = %q %d", host, port)
	}
}

func TestParseAvahi(t *testing.T) {
	out := `+;eth0;IPv4;My\032NAS;_smb._tcp;local
=;eth0;IPv4;My\032NAS;_smb._tcp;local;my-nas.local;192.168.1.5;445;""
=;eth0;IPv6;My\032NAS;_smb._tcp;local;my-nas.local;fe80::1;445;""
=;eth0;IPv4;exports;_nfs._tcp;local;nfs.local;192.168.1.6;2049;"path=/srv"
=;eth0;IPv4;printer;_ipp._tcp;local;p.local;192.168.1.7;631;""
=;eth0;IPv4;odd\.name;_afpovertcp._tcp;local;mac.local;192.168.1.8;10548;""
`
	got := Dedup(ParseAvahi(out))
	want := []Server{
		{Name: "exports", Host: "nfs.local", Kind: "nfs", Source: "avahi"},
		{Name: "My NAS", Host: "my-nas.local", Kind: "smb", Source: "avahi"},
		{Name: "odd.name", Host: "mac.local", Port: 10548, Kind: "afp", Source: "avahi"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("avahi = %+v", got)
	}
}

func TestParseNetView(t *testing.T) {
	out := "Server Name            Remark\r\n\r\n-------------------------------------------------------------------------------\r\n\\\\NAS                  \r\n\\\\PC1                  My PC\r\nThe command completed successfully.\r\n"
	got := ParseNetView(out)
	if len(got) != 2 || got[0].Host != "NAS" || got[1].Name != "PC1" {
		t.Errorf("net view = %+v", got)
	}
}

func TestParseShowmount(t *testing.T) {
	out := "Exports list on nas.local:\n/volume1/media                     192.168.1.0/24\n/export/my files                   (everyone)\n/bare\n"
	got := ParseShowmount(out)
	want := []Share{{Name: "/volume1/media", Comment: "192.168.1.0/24"}, {Name: "/export/my files", Comment: "(everyone)"}, {Name: "/bare"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("showmount = %+v", got)
	}
}

func TestMatchMount(t *testing.T) {
	table := ParseMountTable(`/dev/disk3s1s1 on / (apfs, sealed, local, read-only, journaled)
//pat@My-NAS._smb._tcp.local/my%20media on /Volumes/my media (smbfs, nodev, nosuid, mounted by pat)
nas.local:/volume1/backup on /Volumes/backup (nfs, nodev, nosuid, mounted by pat)
`)
	if got := matchMount(table, Target{Kind: "smb", Host: "my-nas.local", Share: "My Media"}); got != "/Volumes/my media" {
		t.Errorf("smb match = %q", got)
	}
	if got := matchMount(table, Target{Kind: "nfs", Host: "nas", Share: "/volume1/backup/"}); got != "/Volumes/backup" {
		t.Errorf("nfs match = %q", got)
	}
	if got := matchMount(table, Target{Kind: "smb", Host: "other", Share: "my media"}); got != "" {
		t.Errorf("wrong host matched: %q", got)
	}
}

func TestMountScriptQuotes(t *testing.T) {
	s := MountScript(Target{Kind: "smb", Host: "nas", User: "ignored", Share: "media"}, Creds{User: `pat`, Password: `a"b\c`})
	if s != `mount volume "smb://nas/media" as user name "pat" with password "a\"b\\c"`+"\n" {
		t.Errorf("script = %q", s)
	}
}

func TestGVfs(t *testing.T) {
	if got := GVfsURI("smb-share:server=nas,share=my%20media,user=pat"); got != "smb://pat@nas/my%20media" {
		t.Errorf("smb = %q", got)
	}
	if got := GVfsURI("nfs:host=nas,prefix=%2Fvolume1%2Fmedia"); got != "nfs://nas/volume1/media" {
		t.Errorf("nfs = %q", got)
	}
	if GVfsURI("mtp:host=phone") != "" || GVfsURI("plain") != "" {
		t.Error("an unknown GVfs folder gave a location")
	}
	if got := GVfsName("smb-share:server=nas,share=media"); got != "media on nas" {
		t.Errorf("name = %q", got)
	}
	if got := ParseGioLocalPath("display name: media on nas\nlocal path: /run/user/1000/gvfs/smb-share:server=nas,share=media\nuri: smb://nas/media/\n"); !strings.HasSuffix(got, "share=media") {
		t.Errorf("local path = %q", got)
	}
}

func TestVisibleShare(t *testing.T) {
	if visibleShare("IPC$") || visibleShare("C$") || !visibleShare("media") {
		t.Error("hidden shares")
	}
}
