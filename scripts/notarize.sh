#!/usr/bin/env bash
# Notarizes a Developer ID signed mormicommander.app with Apple and staples the ticket to it, so Gatekeeper opens it
# without a warning, also offline (board issue #44; the Homebrew cask list drops apps that fail Gatekeeper). The app
# must have been built with MACOS_SIGN_IDENTITY set (scripts/build.sh signs it with the hardened runtime then).
#
#   scripts/notarize.sh [path/to/mormicommander.app]     (default build/bin/mormicommander.app)
#
# Apple is reached with an App Store Connect API key, given by three environment variables:
#   APPLE_API_KEY_ID      the key's ID
#   APPLE_API_ISSUER      the issuer ID of the team
#   APPLE_API_KEY_P8      the path of the downloaded AuthKey_<ID>.p8 file
set -euo pipefail
cd "$(dirname "$0")/.."

app=${1:-build/bin/mormicommander.app}
[ -d "$app" ] || { echo "notarize.sh: no app at $app" >&2; exit 1; }
for v in APPLE_API_KEY_ID APPLE_API_ISSUER APPLE_API_KEY_P8; do
  [ -n "${!v:-}" ] || { echo "notarize.sh: $v is not set" >&2; exit 1; }
done
# an ad-hoc signed app would only be refused by Apple after the upload, so say it here
if codesign -dv "$app" 2>&1 | grep -q 'Signature=adhoc'; then
  echo "notarize.sh: $app has only an ad-hoc signature; build it with MACOS_SIGN_IDENTITY set" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# notarytool takes a zip of the app; the ticket is stapled to the app itself afterwards
ditto -c -k --keepParent "$app" "$tmp/upload.zip"
xcrun notarytool submit "$tmp/upload.zip" --key "$APPLE_API_KEY_P8" --key-id "$APPLE_API_KEY_ID" \
  --issuer "$APPLE_API_ISSUER" --wait --timeout 30m
xcrun stapler staple "$app"
xcrun stapler validate "$app"
spctl --assess --type execute --verbose=2 "$app"
echo "notarized and stapled $app"
