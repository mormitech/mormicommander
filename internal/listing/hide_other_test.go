//go:build !windows

package listing

import "testing"

// markHidden is a no-op off Windows, where the leading dot already hides a file.
func markHidden(t *testing.T, path string) {}
