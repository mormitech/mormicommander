//go:build !darwin

package dirsize

// isAliasDir: only macOS shows one volume's folders twice (see alias_darwin.go).
func isAliasDir(p, root string) bool { return false }
