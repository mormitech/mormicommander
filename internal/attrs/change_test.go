// Tests for planning and applying attribute changes that work on every
// platform: dates (set, now, shift, copy, per-file function), validation,
// filters, ApplyTo, overlap handling, the preview text and cancellation.
package attrs

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

// tree creates a small folder tree and returns its root.
//
//	root/
//	  a.txt
//	  b.log
//	  sub/
//	    c.txt
//	    deep/
//	      d.txt
//	  .git/
//	    config
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"a.txt", "b.log", "sub/c.txt", "sub/deep/d.txt", ".git/config"} {
		write(t, filepath.Join(root, filepath.FromSlash(f)), []byte(f), 0o644)
	}
	return root
}

func relPaths(t *testing.T, root string, p *Plan, changedOnly bool) []string {
	t.Helper()
	var out []string
	for i := range p.Items {
		it := &p.Items[i]
		if changedOnly && !it.Changed() {
			continue
		}
		r, err := filepath.Rel(root, it.Path)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(r))
	}
	return out
}

func mtime(t *testing.T, p string) time.Time {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

func TestValidate(t *testing.T) {
	c := Caps()
	tests := []struct {
		name string
		req  Request
		ok   bool
	}{
		{"empty selection", Request{}, false},
		{"times only", Request{Paths: []string{"x"}, Times: TimeChange{Modified: NowTime()}}, true},
		{"set and clear", Request{Paths: []string{"x"}, Attrs: WinAttrChange{Set: AttrReadOnly, Clear: AttrReadOnly}}, false},
		{"compressed", Request{Paths: []string{"x"}, Attrs: WinAttrChange{Set: AttrCompressed}}, false},
		{"zero date", Request{Paths: []string{"x"}, Times: TimeChange{Modified: TimeValue{Mode: TimeSet}}}, false},
		{"perm", Request{Paths: []string{"x"}, Perm: PermChange{Set: 0o100}}, c.UnixPerms},
		{"bad expr", Request{Paths: []string{"x"}, Perm: PermChange{Expr: "u+?"}}, false},
		{"owner", Request{Paths: []string{"x"}, Owner: OwnerChange{Owner: "0"}}, c.Owner},
		{"created", Request{Paths: []string{"x"}, Times: TimeChange{Created: NowTime()}}, c.CreatedTime},
		{"system attr", Request{Paths: []string{"x"}, Attrs: WinAttrChange{Set: AttrSystem}}, c.NativeAttrs},
		{"read-only", Request{Paths: []string{"x"}, Attrs: WinAttrChange{Set: AttrReadOnly}}, true},
	}
	for _, tt := range tests {
		err := Validate(tt.req)
		if (err == nil) != tt.ok {
			t.Errorf("%s: Validate = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
	if _, err := MakePlan(context.Background(), Request{}); err == nil {
		t.Error("MakePlan accepted an invalid request")
	}
	if !(Request{}).IsZero() || (Request{Times: TimeChange{Accessed: NowTime()}}).IsZero() {
		t.Error("Request.IsZero wrong")
	}
}

func TestPlanWalkFilterApplyTo(t *testing.T) {
	root := tree(t)
	when := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)
	base := Request{Paths: []string{root}, Times: TimeChange{Modified: SetTime(when)}}
	tests := []struct {
		name string
		mod  func(r *Request)
		want []string
	}{
		{"not recursive", func(r *Request) {}, []string{"."}},
		{"recursive all", func(r *Request) { r.Recursive = true },
			[]string{".", ".git", ".git/config", "a.txt", "b.log", "sub", "sub/c.txt", "sub/deep", "sub/deep/d.txt"}},
		{"files only", func(r *Request) { r.Recursive = true; r.ApplyTo = FilesOnly },
			[]string{".git/config", "a.txt", "b.log", "sub/c.txt", "sub/deep/d.txt"}},
		{"folders only", func(r *Request) { r.Recursive = true; r.ApplyTo = FoldersOnly },
			[]string{".", ".git", "sub", "sub/deep"}},
		{"mask *.txt", func(r *Request) { r.Recursive = true; r.Filter = ParseMask("*.txt") },
			[]string{"a.txt", "sub/c.txt", "sub/deep/d.txt"}},
		{"exclude git folder", func(r *Request) { r.Recursive = true; r.Filter = ParseMask(`| .git\`) },
			[]string{".", "a.txt", "b.log", "sub", "sub/c.txt", "sub/deep", "sub/deep/d.txt"}},
		{"exclude deep and logs", func(r *Request) { r.Recursive = true; r.Filter = ParseMask(`| deep/ *.log .git/`) },
			[]string{".", "a.txt", "sub", "sub/c.txt"}},
		{"hook", func(r *Request) {
			r.Recursive = true
			r.Filter = &Filter{Match: func(p string, i Info) bool { return strings.HasPrefix(i.Name, "c") }}
		}, []string{".git/config", "sub/c.txt"}},
	}
	for _, tt := range tests {
		r := base
		tt.mod(&r)
		p, err := MakePlan(context.Background(), r)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := relPaths(t, root, p, false)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%s:\n got  %v\n want %v", tt.name, got, tt.want)
		}
	}
}

func TestPlanOverlapCountsOnce(t *testing.T) {
	root := tree(t)
	r := Request{Paths: []string{root, filepath.Join(root, "a.txt"), root + string(filepath.Separator)},
		Recursive: true, ApplyTo: FilesOnly, Times: TimeChange{Modified: NowTime()}}
	p, err := MakePlan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 5 {
		t.Errorf("items = %v", relPaths(t, root, p, false))
	}
}

func TestApplyTimes(t *testing.T) {
	root := tree(t)
	a := filepath.Join(root, "a.txt")
	c := filepath.Join(root, "sub", "c.txt")
	when := time.Date(2019, 3, 4, 5, 6, 7, 123456000, time.UTC)

	// Set modified and accessed on every file recursively.
	p, err := MakePlan(context.Background(), Request{Paths: []string{root}, Recursive: true, ApplyTo: FilesOnly,
		Times: TimeChange{Modified: SetTime(when), Accessed: SetTime(when.Add(time.Hour))}})
	if err != nil {
		t.Fatal(err)
	}
	if p.ChangedCount() != 5 {
		t.Errorf("changed count = %d", p.ChangedCount())
	}
	var steps int
	res := p.Apply(context.Background(), func(done, total int, it *Item, err error) {
		steps++
		if total != 5 || err != nil {
			t.Errorf("progress %d/%d %v", done, total, err)
		}
	})
	if res.Changed != 5 || len(res.Failed) != 0 || steps != 5 {
		t.Fatalf("result = %+v steps %d", res, steps)
	}
	for _, f := range []string{a, c} {
		if got := mtime(t, f); !got.Equal(when) {
			t.Errorf("%s mtime = %v, want %v", f, got, when)
		}
		info, _ := Stat(f)
		if !info.Accessed.Equal(when.Add(time.Hour)) {
			t.Errorf("%s atime = %v", f, info.Accessed)
		}
	}
	// A second plan with the same values changes nothing.
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{a}, Times: TimeChange{Modified: SetTime(when)}})
	if p2.ChangedCount() != 0 || p2.Items[0].Summary() != a+": no change" {
		t.Errorf("unchanged plan: %d %q", p2.ChangedCount(), p2.Items[0].Summary())
	}

	// Shift by -1 day, copy accessed from modified.
	p3, err := MakePlan(context.Background(), Request{Paths: []string{a},
		Times: TimeChange{Modified: ShiftTime(-24 * time.Hour), Accessed: CopyTime(FieldModified)}})
	if err != nil {
		t.Fatal(err)
	}
	it := p3.Items[0]
	if len(it.Changes) != 2 || !strings.HasPrefix(it.Changes[0], "modified ") || !strings.Contains(it.Changes[0], " → ") {
		t.Errorf("preview = %q", it.Changes)
	}
	p3.Apply(context.Background(), nil)
	if got := mtime(t, a); !got.Equal(when.Add(-24 * time.Hour)) {
		t.Errorf("shifted mtime = %v", got)
	}
	if info, _ := Stat(a); !info.Accessed.Equal(when) {
		t.Errorf("copied atime = %v, want old mtime %v", info.Accessed, when)
	}

	// "Current" stamps every file with the same instant.
	p4, _ := MakePlan(context.Background(), Request{Paths: []string{a, c}, Times: TimeChange{Modified: NowTime()}})
	p4.Apply(context.Background(), nil)
	if ma, mc := mtime(t, a), mtime(t, c); !ma.Equal(mc) || !ma.Equal(p4.Now) && ma.Sub(p4.Now).Abs() > time.Microsecond {
		t.Errorf("now stamps differ: %v %v (now %v)", ma, mc, p4.Now)
	}

	// Per-file function (e.g. date from the name) wins over Modified.
	special := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	p5, _ := MakePlan(context.Background(), Request{Paths: []string{a, c}, Times: TimeChange{
		Modified: SetTime(when),
		Func: func(i Info) (time.Time, bool) {
			return special, i.Name == "a.txt"
		}}})
	p5.Apply(context.Background(), nil)
	if !mtime(t, a).Equal(special) || !mtime(t, c).Equal(when) {
		t.Errorf("func times: a=%v c=%v", mtime(t, a), mtime(t, c))
	}
}

func TestApplyCreatedTime(t *testing.T) {
	if !Caps().CreatedTime {
		t.Skip("creation time cannot be set here")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	write(t, f, []byte("x"), 0o644)
	born := time.Date(2010, 10, 10, 10, 10, 10, 0, time.UTC)
	p, err := MakePlan(context.Background(), Request{Paths: []string{f, dir},
		Times: TimeChange{Created: SetTime(born), Modified: SetTime(born.Add(time.Hour))}})
	if err != nil {
		t.Fatal(err)
	}
	if res := p.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatalf("apply failed: %v", res.Failed)
	}
	for _, x := range []string{f, dir} {
		info, _ := Stat(x)
		if !info.Created.Equal(born) {
			t.Errorf("%s created = %v, want %v", x, info.Created, born)
		}
	}
}

func TestApplyReadOnlyAttribute(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	write(t, f, []byte("x"), 0o664)
	p, err := MakePlan(context.Background(), Request{Paths: []string{f}, Attrs: WinAttrChange{Set: AttrReadOnly}})
	if err != nil {
		t.Fatal(err)
	}
	it := p.Items[0]
	if !it.Changed() || it.After.Win&AttrReadOnly == 0 {
		t.Fatalf("plan = %+v", it)
	}
	if res := p.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	info, _ := Stat(f)
	if info.Win&AttrReadOnly == 0 {
		t.Errorf("not read-only after apply: %v", info.Win)
	}
	if runtime.GOOS != "windows" && info.Perm() != 0o444 {
		t.Errorf("perm after read-only = %v", FormatPerm(info.Perm()))
	}
	// Clearing read-only together with a new date must work even though the
	// file is read-only when the dates are written (Windows ordering).
	when := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{f}, Attrs: WinAttrChange{Clear: AttrReadOnly},
		Times: TimeChange{Modified: SetTime(when)}})
	if res := p2.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	info, _ = Stat(f)
	if info.Win&AttrReadOnly != 0 || !info.Modified.Equal(when) {
		t.Errorf("after clearing: %v %v", info.Win, info.Modified)
	}
	if runtime.GOOS != "windows" && info.Perm() != 0o644 {
		t.Errorf("perm after clearing read-only = %v", FormatPerm(info.Perm()))
	}
}

func TestPlanMissingAndCancel(t *testing.T) {
	root := tree(t)
	missing := filepath.Join(root, "missing")
	p, err := MakePlan(context.Background(), Request{Paths: []string{missing, root}, Times: TimeChange{Modified: NowTime()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Errors()) != 1 || p.Errors()[0].Path != missing || p.Items[0].Changed() {
		t.Errorf("missing entry not reported: %+v", p.Items)
	}
	if !strings.Contains(p.Items[0].Summary(), "missing") {
		t.Errorf("summary = %q", p.Items[0].Summary())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MakePlan(ctx, Request{Paths: []string{root}, Recursive: true, Times: TimeChange{Modified: NowTime()}}); !errors.Is(err, context.Canceled) {
		t.Errorf("plan with canceled ctx: %v", err)
	}
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{root}, Recursive: true, Times: TimeChange{Modified: SetTime(time.Unix(1e9, 0))}})
	res := p2.Apply(ctx, nil)
	if !res.Canceled || res.Changed != 0 {
		t.Errorf("apply with canceled ctx = %+v", res)
	}
}

func TestApplyContinuesAfterFailure(t *testing.T) {
	root := tree(t)
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.log")
	when := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	p, err := MakePlan(context.Background(), Request{Paths: []string{a, b}, Times: TimeChange{Modified: SetTime(when)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(a); err != nil { // vanishes between preview and apply
		t.Fatal(err)
	}
	res := p.Apply(context.Background(), nil)
	if res.Changed != 1 || len(res.Failed) != 1 || res.Failed[0].Path != a {
		t.Errorf("result = %+v", res)
	}
	if !mtime(t, b).Equal(when) {
		t.Error("second file not changed")
	}
}

func TestSymlinkTimesNotFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "t.txt")
	write(t, target, []byte("x"), 0o644)
	link := filepath.Join(dir, "l")
	if err := os.Symlink("t.txt", link); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	before := mtime(t, target)
	when := time.Date(2012, 2, 2, 2, 2, 2, 0, time.UTC)
	p, _ := MakePlan(context.Background(), Request{Paths: []string{link}, Times: TimeChange{Modified: SetTime(when)}})
	if res := p.Apply(context.Background(), nil); len(res.Failed) != 0 {
		t.Fatal(res.Failed)
	}
	if !mtime(t, link).Equal(when) || !mtime(t, target).Equal(before) {
		t.Errorf("link %v target %v", mtime(t, link), mtime(t, target))
	}
	// With FollowLinks the target changes.
	p2, _ := MakePlan(context.Background(), Request{Paths: []string{link}, FollowLinks: true, Times: TimeChange{Modified: SetTime(when)}})
	if p2.Items[0].Info.Kind != KindFile {
		t.Errorf("follow plan describes %v", p2.Items[0].Info.Kind)
	}
	p2.Apply(context.Background(), nil)
	if !mtime(t, target).Equal(when) {
		t.Errorf("target not changed with FollowLinks: %v", mtime(t, target))
	}
}
