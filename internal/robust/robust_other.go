//go:build !windows

package robust

// Other systems don't lock files that are open, so no error is worth a retry.
func isTransient(error) bool { return false }
