package keychain

// The parts of the Windows store that are plain data, kept out of the Windows-only file so they are tested
// everywhere: the target name of a secret and the blob the Credential Manager keeps. The blob is the secret as
// UTF-16LE, the form the Credential Manager and other Windows programs use for passwords, so the user sees the real
// password if they look at it there.

import (
	"errors"
	"unicode/utf16"
)

// winTarget is the Credential Manager's target name of a secret: "mormiCommander:<name>".
func winTarget(service, name string) string { return service + ":" + name }

// winBlob encodes secret as UTF-16LE.
func winBlob(secret string) []byte {
	u := utf16.Encode([]rune(secret))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return b
}

// winUnblob decodes a UTF-16LE blob.
func winUnblob(b []byte) (string, error) {
	if len(b)%2 != 0 {
		return "", errors.New("keychain: the saved secret is not UTF-16 text")
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u)), nil
}
