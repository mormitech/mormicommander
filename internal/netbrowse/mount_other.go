//go:build !darwin && !linux && !windows

package netbrowse

import (
	"context"
	"errors"
)

func discover(ctx context.Context) ([]Server, string) {
	return nil, "finding servers isn't supported on this system"
}

// Mount is not supported here.
func Mount(ctx context.Context, t Target, c Creds, o MountOptions) (Mounted, error) {
	return Mounted{}, errors.New("connecting shares isn't supported on this system")
}

// Unmount is not supported here.
func Unmount(ctx context.Context, path string) error {
	return errors.New("disconnecting shares isn't supported on this system")
}

// FreeLetters is for Windows only.
func FreeLetters() []string { return nil }
