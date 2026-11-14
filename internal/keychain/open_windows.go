package keychain

// The Windows store: the Credential Manager ("Windows Credentials", generic credentials), through CredWriteW,
// CredReadW and CredDeleteW of advapi32. The secret goes to the system call in memory; no program is started, so it
// never appears on a command line. A credential is kept on this computer only (CRED_PERSIST_LOCAL_MACHINE), not
// roamed with the user's profile, and Windows protects it with the user's login.

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine = 2 // CRED_PERSIST_LOCAL_MACHINE
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type winKeychain struct{ service string }

// Open returns the Credential Manager.
func Open() (Keychain, error) {
	if err := procCredReadW.Find(); err != nil {
		return nil, Unavailable("The Windows Credential Manager can't be reached.")
	}
	return &winKeychain{service: Service}, nil
}

func (k *winKeychain) Get(name string) (string, error) {
	if err := ValidName(name); err != nil {
		return "", err
	}
	target, err := windows.UTF16PtrFromString(winTarget(k.service, name))
	if err != nil {
		return "", err
	}
	var c *credential
	r, _, e := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		if errors.Is(e, windows.ERROR_NOT_FOUND) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain: %v", e)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(c)))
	var blob []byte
	if c.CredentialBlobSize > 0 && c.CredentialBlob != nil {
		blob = append([]byte(nil), unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)...)
	}
	s, err := winUnblob(blob)
	clear(blob)
	return s, err
}

func (k *winKeychain) Set(name, secret string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	if err := checkSecret(secret); err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(winTarget(k.service, name))
	if err != nil {
		return err
	}
	user, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	blob := winBlob(secret)
	defer clear(blob)
	c := credential{Type: credTypeGeneric, TargetName: target, UserName: user, Persist: credPersistLocalMachine,
		CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0]}
	if r, _, e := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return fmt.Errorf("keychain: %v", e)
	}
	return nil
}

func (k *winKeychain) Delete(name string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(winTarget(k.service, name))
	if err != nil {
		return err
	}
	if r, _, e := procCredDeleteW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0); r == 0 && !errors.Is(e, windows.ERROR_NOT_FOUND) {
		return fmt.Errorf("keychain: %v", e)
	}
	return nil
}
