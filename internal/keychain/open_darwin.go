package keychain

import "os"

// Open returns the user's login keychain.
func Open() (Keychain, error) {
	if _, err := os.Stat(macSecurity); err != nil {
		return nil, Unavailable("The macOS keychain can't be reached: " + macSecurity + " is missing.")
	}
	return &macKeychain{service: Service, run: execRunner}, nil
}
