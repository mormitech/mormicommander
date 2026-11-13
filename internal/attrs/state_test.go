// Tests for the three-state checkbox summary of a multiple selection and the
// conversion of final checkbox states back into change requests.
package attrs

import (
	"io/fs"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	t1 := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	infos := []Info{
		{Kind: KindFile, Mode: 0o755, Win: AttrArchive | AttrReadOnly, Owner: "ann", Group: "staff", Modified: t1, Created: t1},
		{Kind: KindDir, Mode: fs.ModeDir | 0o750, Win: AttrArchive, Owner: "ann", Group: "wheel", Modified: t1, Created: t2, HiddenFlag: true},
		{Kind: KindSymlink, Mode: fs.ModeSymlink | 0o711, Win: AttrArchive, Owner: "ann", Group: "staff", Modified: t1, Created: t1},
	}
	s := Summarize(infos)
	checks := []struct {
		name string
		got  Tri
		want Tri
	}{
		{"owner read", s.PermBit(0o400), On},
		{"group read", s.PermBit(0o040), Mixed},
		{"others read", s.PermBit(0o004), Mixed},
		{"others write", s.PermBit(0o002), Off},
		{"group write", s.PermBit(0o020), Off},
		{"owner exec", s.PermBit(0o100), On},
		{"archive", s.Attr(AttrArchive), On},
		{"read-only", s.Attr(AttrReadOnly), Mixed},
		{"system", s.Attr(AttrSystem), Off},
		{"hidden flag", s.HiddenFlag(), Mixed},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.Owner != "ann" || s.OwnerMixed || s.Group != "" || !s.GroupMixed {
		t.Errorf("owner/group = %q %v %q %v", s.Owner, s.OwnerMixed, s.Group, s.GroupMixed)
	}
	if !s.Modified.Equal(t1) || s.ModifiedMixed || !s.Created.IsZero() || !s.CreatedMixed {
		t.Errorf("dates wrong: %+v", s)
	}
	if s.Files != 1 || s.Folders != 1 || s.Links != 1 || s.Count != 3 {
		t.Errorf("counts wrong: %+v", s)
	}
	if _, ok := s.CommonPerm(); ok {
		t.Error("CommonPerm should be mixed")
	}
	one := Summarize(infos[:1])
	if m, ok := one.CommonPerm(); !ok || m != 0o755 {
		t.Errorf("CommonPerm single = %v %v", m, ok)
	}
	empty := Summarize(nil)
	if empty.PermBit(0o400) != Off || empty.Attr(AttrArchive) != Off || empty.HiddenFlag() != Off {
		t.Error("empty selection should be all off")
	}
	if On.String() != "on" || Mixed.String() != "mixed" || Off.String() != "off" {
		t.Error("Tri.String")
	}
}

func TestFromTri(t *testing.T) {
	pc := PermChangeFromTri(map[fs.FileMode]Tri{0o400: On, 0o002: Off, 0o040: Mixed, fs.ModeSetuid: On})
	if pc.Set != 0o400|fs.ModeSetuid || pc.Clear != 0o002 {
		t.Errorf("PermChangeFromTri = %+v", pc)
	}
	wc := WinAttrChangeFromTri(map[WinAttr]Tri{AttrReadOnly: On, AttrHidden: Off, AttrSystem: Mixed})
	if wc.Set != AttrReadOnly || wc.Clear != AttrHidden {
		t.Errorf("WinAttrChangeFromTri = %+v", wc)
	}
	initial := map[WinAttr]Tri{AttrReadOnly: On, AttrHidden: Mixed, AttrArchive: Off}
	final := map[WinAttr]Tri{AttrReadOnly: On, AttrHidden: Off, AttrArchive: Off, AttrSystem: On}
	got := ChangedOnly(initial, final)
	if len(got) != 2 || got[AttrHidden] != Off || got[AttrSystem] != On {
		t.Errorf("ChangedOnly = %v", got)
	}
}
