# internal/keychain

Named secrets in the operating system's own secret store, for anything the
app must keep but never write to its own files: saved server passwords (when
"Keep saved passwords in the system keychain" is on), and later an AI
provider's API key.

```go
kc, err := keychain.Open()          // errors.Is(err, keychain.ErrUnavailable): say err.Error() to the user
err = kc.Set("site.4f2a", "s3cret") // replaces what was there
pw, err := kc.Get("site.4f2a")      // keychain.ErrNotFound when there is none
err = kc.Delete("site.4f2a")        // no error when there was none
```

Every secret is filed under the service `mormiCommander`. Names are letters,
digits, `.`, `_` and `-` (up to 128); secrets are 1 to 1024 bytes.

| System  | Store | How the secret travels |
|---------|-------|------------------------|
| macOS   | login keychain, generic password (service, account = name) | `/usr/bin/security -i`, the add command on stdin with the secret as `-X <hex>`; read back with `find-generic-password -g` |
| Windows | Credential Manager, generic credential `mormiCommander:<name>`, kept on this computer | `CredWriteW` / `CredReadW` / `CredDeleteW` in memory, the secret as UTF-16LE |
| Linux   | Secret Service through `secret-tool` (attributes `service`, `id`) | `secret-tool store` reads it on stdin; unavailable without secret-tool |

The secret is never an argument of a program, so it never shows in the
process list. `Fake` is an in-memory `Keychain` for tests.

Tests: `go test ./internal/keychain` checks the commands with a fake runner.
`MC_KEYCHAIN_INTEGRATION=1 go test ./internal/keychain -run Security` (macOS)
runs `security` for real against a keychain file the test creates and deletes.
