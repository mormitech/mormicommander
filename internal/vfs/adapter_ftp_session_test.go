package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/ftp"
)

func TestFTPSessionModeAndQuote(t *testing.T) {
	s := newAuthFTPServer(t)
	s.write("notes.txt", "a\r\nb\r\n")
	r, base := ftpRouter(t, s)
	ctx := context.Background()
	home, err := r.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := r.FTP.TransferMode(home); err != nil || m != ftp.TransferAuto {
		t.Fatalf("default mode %v %v", m, err)
	}
	if err := r.FTP.SetTransferMode(home, ftp.TransferBinary); err != nil {
		t.Fatal(err)
	}
	if m, _ := r.FTP.TransferMode(home + "notes.txt"); m != ftp.TransferBinary {
		t.Fatalf("mode after set: %v", m)
	}
	// binary keeps the CRLF line ends of the text file
	local := filepath.Join(t.TempDir(), "notes.txt")
	if _, err := r.Download(ctx, home+"notes.txt", local, TransferOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(local); string(b) != "a\r\nb\r\n" {
		t.Fatalf("binary download changed the file: %q", b)
	}
	reply, err := r.FTP.Quote(ctx, home, "NOOP")
	if err != nil || !strings.HasPrefix(reply, "200") {
		t.Fatalf("quote: %q %v", reply, err)
	}
	r.Disconnect(home)
	if err := r.FTP.SetTransferMode(home, ftp.TransferASCII); !errors.Is(err, ErrNoSession) {
		t.Fatalf("no session: %v", err)
	}
}
