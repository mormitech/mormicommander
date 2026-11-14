# Signing the releases

CI builds every release unsigned (macOS: ad hoc) until the signing secrets are added to the repository. Once they are
there, the same workflow signs (and on macOS notarizes) the builds by itself; nothing else changes. Secrets go in
GitHub: repository Settings → Secrets and variables → Actions → New repository secret.

## macOS: Developer ID and notarization (board issue #44)

Needs a paid Apple Developer Program membership.

1. **Certificate.** In Xcode (Settings → Accounts → Manage Certificates) or on developer.apple.com, create a
   *Developer ID Application* certificate. In Keychain Access, export it with its private key as a `.p12` file with
   a password.
2. **API key for notarization.** On App Store Connect → Users and Access → Integrations → App Store Connect API, create
   a key with the *Developer* role and download its `AuthKey_<ID>.p8` (it can be downloaded only once). Note the key
   ID and the issuer ID shown on that page.
3. **Secrets:**

   | Secret | Value |
   |---|---|
   | `MACOS_CERT_P12` | the `.p12` file in base64: `base64 -i cert.p12 \| pbcopy` |
   | `MACOS_CERT_PASSWORD` | the `.p12` password |
   | `APPLE_API_KEY_ID` | the key ID |
   | `APPLE_API_ISSUER` | the issuer ID |
   | `APPLE_API_KEY_P8` | the whole text of `AuthKey_<ID>.p8` |

With the certificate secrets, CI signs the app with the hardened runtime (`scripts/build.sh`, `MACOS_SIGN_IDENTITY`);
with the API key as well, it notarizes and staples it (`scripts/notarize.sh`). The macOS build is universal (Apple
silicon and Intel) either way.

Locally the same works: `MACOS_SIGN_IDENTITY="Developer ID Application: …" GOARCH=universal scripts/build.sh`, then
`APPLE_API_KEY_ID=… APPLE_API_ISSUER=… APPLE_API_KEY_P8=path/to/AuthKey.p8 scripts/notarize.sh`.

Once releases are notarized, the Homebrew cask (`scripts/make-cask.mjs`) no longer needs to clear the quarantine flag.
