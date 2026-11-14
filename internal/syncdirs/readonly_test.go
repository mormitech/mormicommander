// Tests for read-only sides (FEATURES.md F-555): a side that can only be read,
// such as a RAR archive, keeps every row's action away from it, also after
// Reset, an explicit action or a turned arrow, so Plan never has to write
// there and Execute never meets ErrReadOnly.
package syncdirs

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestReadOnlySideGetsNoActions(t *testing.T) {
	l, r := trees(t, map[string]spec{
		"same.txt":  file("s", 0),
		"lnew.txt":  file("new", time.Hour),
		"lonly.txt": file("l", 0),
	}, map[string]spec{
		"same.txt":  file("s", 0),
		"lnew.txt":  file("old", 0),
		"ronly.txt": file("r", 0),
		"rnew.txt":  file("rn", time.Hour),
	})
	build(t, l, map[string]spec{"rnew.txt": file("ln", 0)})
	// The left side is read-only: an io/fs view of the folder, as an archive would be.
	res, err := Compare(context.Background(), FromFS(os.DirFS(l)), DirFS(r), DefaultOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Find("ronly.txt").Action; got != CopyRightToLeft {
		t.Fatalf("before SetReadOnly ronly.txt has %v", got)
	}
	res.SetReadOnly(LeftSide, true)
	if !res.ReadOnly(LeftSide) || res.ReadOnly(RightSide) {
		t.Fatal("ReadOnly does not report the marked side")
	}
	want := map[string]Action{"lnew.txt": CopyLeftToRight, "lonly.txt": CopyLeftToRight, "ronly.txt": NoAction, "rnew.txt": NoAction, "same.txt": NoAction}
	for p, a := range want {
		if got := res.Find(p).Action; got != a {
			t.Errorf("%s: action %v, want %v", p, got, a)
		}
	}
	if res.SetAction("ronly.txt", CopyRightToLeft) == nil || res.SetAction("same.txt", DeleteLeft) == nil || res.SetAction("same.txt", DeleteBoth) == nil {
		t.Fatal("an action writing to the read-only side was accepted")
	}
	if res.SetAction("ronly.txt", DeleteRight) != nil {
		t.Fatal("deleting on the writable side was refused")
	}
	if res.ReverseAction("lnew.txt") == nil {
		t.Fatal("the arrow turned towards the read-only side")
	}
	if n, _ := res.SetActionTree("", CopyRightToLeft); n != 0 {
		t.Fatalf("SetActionTree changed %d rows towards the read-only side", n)
	}
	res.ResetActions()
	if res.Find("ronly.txt").Action != NoAction {
		t.Fatal("Reset gave back an action towards the read-only side")
	}
	p := res.Plan(AllOps())
	if p.NeedsWriteLeft || p.FilesLR != 2 {
		t.Fatalf("plan %+v", p)
	}
	rep, err := Execute(context.Background(), p, res.left, res.right, ExecOptions{})
	if err != nil || len(rep.Errors) != 0 || rep.Done != 2 {
		t.Fatalf("execute: %+v %v", rep, err)
	}
	// Asymmetric mode with a read-only right side has nothing it may do.
	res2, err := Compare(context.Background(), DirFS(l), FromFS(os.DirFS(r)), Options{Subdirs: true, Asymmetric: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res2.SetReadOnly(RightSide, true)
	if p := res2.Plan(AllOps()); len(p.Ops) != 0 {
		t.Fatalf("asymmetric plan onto a read-only side: %+v", p.Ops)
	}
}
