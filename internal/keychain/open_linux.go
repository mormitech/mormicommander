package keychain

import "os/exec"

// Open returns the Secret Service through secret-tool, when it is installed.
func Open() (Keychain, error) {
	p, err := exec.LookPath("secret-tool")
	if err != nil {
		return nil, Unavailable("The system keychain needs secret-tool (the libsecret-tools or libsecret package) and a Secret Service such as GNOME Keyring or KWallet.")
	}
	return &secretToolKeychain{service: Service, path: p, run: execRunner}, nil
}
