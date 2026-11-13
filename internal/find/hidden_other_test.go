//go:build !windows

package find

import "testing"

// markHidden is a no-op outside Windows: there a leading dot already makes a file hidden.
func markHidden(t *testing.T, path string) {}
