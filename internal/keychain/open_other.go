//go:build !darwin && !linux && !windows

package keychain

// Open says there is no system keychain the app can use on this system.
func Open() (Keychain, error) {
	return nil, Unavailable("mormiCommander can't use a system keychain on this system.")
}
