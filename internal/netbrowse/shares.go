package netbrowse

// Listing a server's shares: SMB in the process (go-smb2), NFS through showmount.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
)

// Shares lists the shares of t's server. SMB without a user name logs in as guest; ErrAuth means the server wants a
// user name and password (or the ones given were wrong).
func Shares(ctx context.Context, t Target, c Creds) ([]Share, error) {
	switch t.Kind {
	case "smb":
		return smbShares(ctx, t, c)
	case "nfs":
		return nfsShares(ctx, t)
	case "afp":
		return nil, errors.New("AFP servers can't list their shares here; open the server to choose one in macOS's own window")
	}
	return nil, fmt.Errorf("can't list the shares of a %s server", t.Kind)
}

// smbDial connects and logs in; the TCP connection is closed by the caller through the returned func.
func smbDial(ctx context.Context, t Target, c Creds) (*smb2.Session, func(), error) {
	port := t.Port
	if port == 0 {
		port = 445
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(t.Host, strconv.Itoa(port)))
	if err != nil {
		return nil, nil, fmt.Errorf("can't reach %s: %w", t.Host, plainNetErr(err))
	}
	user := c.User
	if user == "" {
		user = "guest" // go-smb2 has no anonymous login; servers that allow guests take this
	}
	sd := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: c.Password, Domain: c.Domain}}
	s, err := sd.DialContext(ctx, conn)
	if err != nil {
		conn.Close()
		if isAuthErr(err) {
			return nil, nil, ErrAuth
		}
		return nil, nil, fmt.Errorf("%s: %v", t.Host, err)
	}
	return s, func() { s.Logoff(); conn.Close() }, nil
}

// authStatus are the NT status codes of a refused login or access: STATUS_LOGON_FAILURE, ACCESS_DENIED,
// ACCOUNT_RESTRICTION, ACCOUNT_DISABLED, PASSWORD_EXPIRED, ACCOUNT_LOCKED_OUT, WRONG_PASSWORD, BAD_NETWORK_NAME is not
// one (that is a missing share).
var authStatus = map[uint32]bool{0xC000006D: true, 0xC0000022: true, 0xC000006E: true, 0xC0000072: true,
	0xC0000071: true, 0xC0000234: true, 0xC000006A: true, 0xC0000064: true}

// isAuthErr recognises a refused login in go-smb2's errors.
func isAuthErr(err error) bool {
	var re *smb2.ResponseError
	return errors.As(err, &re) && authStatus[re.Code]
}

func smbShares(ctx context.Context, t Target, c Creds) ([]Share, error) {
	s, done, err := smbDial(ctx, t, c)
	if err != nil {
		return nil, err
	}
	defer done()
	names, err := s.ListSharenames()
	if err != nil {
		if isAuthErr(err) {
			return nil, ErrAuth
		}
		return nil, fmt.Errorf("%s doesn't list its shares: %v", t.Host, err)
	}
	var out []Share
	for _, n := range names {
		if visibleShare(n) {
			out = append(out, Share{Name: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// ParseShowmount reads `showmount -e host`: a heading, then one export per line with who may mount it.
func ParseShowmount(out string) []Share {
	var list []Share
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		if !strings.HasPrefix(line, "/") {
			continue // the "Exports list on host:" heading
		}
		path, who := line, ""
		// the path is padded to a column; a run of two or more blanks (or a tab) starts the "who" part
		if i := strings.Index(line, "  "); i > 0 {
			path, who = line[:i], strings.TrimSpace(line[i:])
		} else if i := strings.IndexAny(line, "\t"); i > 0 {
			path, who = line[:i], strings.TrimSpace(line[i:])
		} else if f := strings.Fields(line); len(f) == 2 {
			path, who = f[0], f[1]
		}
		list = append(list, Share{Name: path, Comment: who})
	}
	return list
}

func nfsShares(ctx context.Context, t Target) ([]Share, error) {
	if _, err := exec.LookPath("showmount"); err != nil {
		return nil, errors.New("showmount is missing (on Linux it comes with nfs-common or nfs-utils); type the export instead, for example nfs://server/export")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "showmount", "-e", t.Host)
	hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s doesn't list its exports: %s", t.Host, lastLine(string(out), err))
	}
	return ParseShowmount(string(out)), nil
}

// lastLine is the last non-empty line of a tool's output, or the error.
func lastLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return err.Error()
}

// plainNetErr drops the "dial tcp 1.2.3.4:445:" prefix Go puts before network errors.
func plainNetErr(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Err != nil {
		return op.Err
	}
	return err
}
