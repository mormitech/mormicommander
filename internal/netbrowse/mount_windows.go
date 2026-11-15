package netbrowse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func discover(ctx context.Context) ([]Server, string) { return discoverNetView(ctx) }

func watch(ctx context.Context, emit func(Event)) string { return watchNetView(ctx, emit) }

var (
	mpr                       = windows.NewLazySystemDLL("mpr.dll")
	procWNetAddConnection2W   = mpr.NewProc("WNetAddConnection2W")
	procWNetCancelConnection2 = mpr.NewProc("WNetCancelConnection2W")
)

const (
	resourcetypeDisk               = 0x1
	connectUpdateProfile           = 0x1
	connectTemporary               = 0x4
	errorSessionCredentialConflict = 1219
	errorAlreadyAssigned           = 85
)

// netResource is NETRESOURCEW.
type netResource struct {
	Scope, Type, DisplayType, Usage uint32
	LocalName, RemoteName           *uint16
	Comment, Provider               *uint16
}

// Mount connects t: with a drive letter it maps the share (reconnected at sign-in when Persistent); without one it
// connects the UNC path, which Explorer and the panels then open directly. Without a user name Windows uses the
// signed-in user's credentials.
func Mount(ctx context.Context, t Target, c Creds, o MountOptions) (Mounted, error) {
	if t.Share == "" {
		return Mounted{}, errors.New("choose a share first")
	}
	unc := t.UNC()
	if t.Kind == "nfs" {
		unc = `\\` + t.Host + strings.ReplaceAll(t.Share, "/", `\`) // the Windows NFS client takes \\host\export
	}
	letter := strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(o.Letter), `\`))
	if letter != "" && (len(letter) != 2 || letter[1] != ':' || letter[0] < 'A' || letter[0] > 'Z') {
		return Mounted{}, fmt.Errorf("%q is not a drive letter", o.Letter)
	}
	if letter == "" && c.User == "" {
		return Mounted{Path: unc}, nil // nothing to connect: Windows signs in by itself when the path is opened
	}
	remote, _ := windows.UTF16PtrFromString(unc)
	nr := netResource{Type: resourcetypeDisk, RemoteName: remote}
	if letter != "" {
		nr.LocalName, _ = windows.UTF16PtrFromString(letter)
	}
	var user, pass *uint16
	if c.User != "" {
		name := c.User
		if c.Domain != "" && !strings.ContainsAny(name, `\@`) {
			name = c.Domain + `\` + name
		}
		user, _ = windows.UTF16PtrFromString(name)
		pass, _ = windows.UTF16PtrFromString(c.Password)
	}
	flags := uint32(connectTemporary)
	if o.Persistent && letter != "" {
		flags = connectUpdateProfile
	}
	r, _, _ := procWNetAddConnection2W.Call(uintptr(unsafe.Pointer(&nr)), uintptr(unsafe.Pointer(pass)), uintptr(unsafe.Pointer(user)), uintptr(flags))
	switch r {
	case 0:
	case uintptr(windows.ERROR_ACCESS_DENIED), uintptr(windows.ERROR_LOGON_FAILURE), uintptr(windows.ERROR_INVALID_PASSWORD):
		return Mounted{}, ErrAuth
	case errorSessionCredentialConflict:
		if letter == "" {
			return Mounted{Path: unc}, nil // already connected with other credentials: the UNC path works as it is
		}
		return Mounted{}, errors.New("Windows is already connected to this server as another user; disconnect it first")
	case errorAlreadyAssigned:
		return Mounted{}, fmt.Errorf("%s is already in use", letter)
	default:
		return Mounted{}, fmt.Errorf("Windows could not connect to %s: %v", unc, syscall.Errno(r))
	}
	if letter != "" {
		return Mounted{Path: letter + `\`, Made: true}, nil
	}
	return Mounted{Path: unc, Made: true}, nil
}

// Unmount disconnects a mapped drive ("Z:\") or a connected UNC path, and forgets it for the next sign-in.
func Unmount(ctx context.Context, path string) error {
	name := strings.TrimRight(path, `\`)
	if len(name) == 2 && name[1] == ':' {
		name = strings.ToUpper(name)
	}
	p, _ := windows.UTF16PtrFromString(name)
	r, _, _ := procWNetCancelConnection2.Call(uintptr(unsafe.Pointer(p)), connectUpdateProfile, 0)
	if r != 0 {
		return fmt.Errorf("%s can't be disconnected: %v", path, syscall.Errno(r))
	}
	return nil
}

// FreeLetters lists the drive letters from D: to Z: that no drive uses, Z: first (Windows maps from the end).
func FreeLetters() []string {
	used, _ := windows.GetLogicalDrives()
	var out []string
	for i := 25; i >= 3; i-- {
		if used&(1<<uint(i)) == 0 {
			out = append(out, string(rune('A'+i))+":")
		}
	}
	return out
}
