#!/usr/bin/env bash
# Builds mormiCommander for the system it runs on (or for GOOS/GOARCH when they are set): first the page (frontend/,
# Vite), which the Go program embeds, then the program itself with Wails v3. There is no Wails CLI in the way, only go
# and npm.
#
#   macOS    build/bin/mormicommander.app (Info.plist with the version from wails.json, the icon, ad-hoc signed)
#   Windows  build/bin/mormicommander.exe (icon, manifest and file properties linked in; needs bash, as Git for Windows
#            has it)
#   Linux    build/bin/mormicommander (GTK 4 and WebKitGTK 6.0: libgtk-4-dev and libwebkitgtk-6.0-dev to build)
#
# wails.json is the one place of the version; it is read here for the bundle and the .exe, and by version.go for the
# About box, --version and control mode. Options: --skip-frontend uses frontend/dist as it is; EXTRA_TAGS adds Go build
# tags; DEV=1 leaves out the production tag (the page's developer tools stay on); WAILS3 names a
# prebuilt wails3 v3.1.0 for the Windows .syso step instead of go run.
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD

skip_frontend=
for a in "$@"; do
  case $a in
    --skip-frontend) skip_frontend=1 ;;
    *) echo "build.sh: unknown option $a" >&2; exit 2 ;;
  esac
done

# info KEY: a field of wails.json's "info" (productVersion, productName, bundleIdentifier, copyright, comments,
# companyName).
info() { node -p "require('./wails.json').info.$1"; }
version=$(info productVersion)
goos=${GOOS:-$(go env GOOS)}
goarch=${GOARCH:-$(go env GOARCH)}
tags=production
[ -n "${DEV:-}" ] && tags=
[ -n "${EXTRA_TAGS:-}" ] && tags="${tags:+$tags,}$EXTRA_TAGS"
flags=(-trimpath -buildvcs=false)
[ -n "$tags" ] && flags+=(-tags "$tags")

echo "mormiCommander $version for $goos/$goarch"

if [ -z "$skip_frontend" ]; then
  (cd frontend && { [ -d node_modules ] || npm ci; } && npm run build)
fi
[ -f frontend/dist/index.html ] || { echo "build.sh: frontend/dist is missing (run without --skip-frontend)" >&2; exit 1; }
mkdir -p build/bin

# fill FILE: the file with @VERSION@ and the other wails.json values put in, on stdout.
fill() {
  sed -e "s|@VERSION@|$version|g" -e "s|@BUNDLE_ID@|$(info bundleIdentifier)|g" -e "s|@PRODUCT_NAME@|$(info productName)|g" -e "s|@COPYRIGHT@|$(info copyright)|g" \
    -e "s|@COMMENTS@|$(info comments)|g" "$1"
}

case $goos in
darwin)
  export CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=13.0
  export CGO_CFLAGS="-mmacosx-version-min=13.0 ${CGO_CFLAGS:-}" CGO_LDFLAGS="-mmacosx-version-min=13.0 ${CGO_LDFLAGS:-}"
  app=build/bin/mormicommander.app
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  GOOS=$goos GOARCH=$goarch go build "${flags[@]}" -ldflags "-s -w" -o "$tmp/mormicommander" .
  # The bundle is put together next to the old one and swapped in at the end, so a failed build leaves the last
  # working app in place.
  new="$tmp/mormicommander.app"
  mkdir -p "$new/Contents/MacOS" "$new/Contents/Resources"
  mv "$tmp/mormicommander" "$new/Contents/MacOS/mormicommander"
  fill build/darwin/Info.plist > "$new/Contents/Info.plist"
  # the icon, in every size macOS asks for, made from build/appicon.png (1024 x 1024)
  set=$tmp/iconfile.iconset
  mkdir -p "$set"
  for s in 16 32 128 256 512; do
    sips -z $s $s build/appicon.png --out "$set/icon_${s}x${s}.png" >/dev/null
    sips -z $((s * 2)) $((s * 2)) build/appicon.png --out "$set/icon_${s}x${s}@2x.png" >/dev/null
  done
  iconutil -c icns "$set" -o "$new/Contents/Resources/iconfile.icns"
  # the `mormicommander` command (the Homebrew cask links to it), see packaging/macos
  mkdir -p "$new/Contents/Resources/bin"
  install -m 755 packaging/macos/mormicommander "$new/Contents/Resources/bin/mormicommander"
  # an ad-hoc signature, so the bundle is at least internally consistent; it is not notarized
  codesign --force --deep -s - "$new" >/dev/null 2>&1 || echo "build.sh: codesign failed (the app still runs from here)" >&2
  rm -rf "$app"
  mv "$new" "$app"
  echo "built $app"
  ;;
windows)
  # The icon, the manifest and the file properties go into the .exe through a .syso file that go build links in;
  # Wails' own tool makes it (the same version as the library, fetched by go run).
  winfo=$(mktemp)
  node -e '
    const i = require("./wails.json").info;
    process.stdout.write(JSON.stringify({ fixed: { file_version: i.productVersion }, info: { "0000": {
      ProductVersion: i.productVersion, CompanyName: i.companyName, FileDescription: i.productName,
      LegalCopyright: i.copyright, ProductName: i.productName, Comments: i.comments } } }, null, 2));' > "$winfo"
  manifest=$(mktemp)
  fill build/windows/wails.exe.manifest > "$manifest"
  syso="$root/rsrc_windows_$goarch.syso"
  trap 'rm -f "$winfo" "$manifest" "$syso"' EXIT
  # WAILS3 may name an already-built wails3 v3.1.0 (CI keeps one in its cache) so it isn't compiled again each time;
  # without it, go run fetches and builds the tool as before.
  if [ -n "${WAILS3:-}" ]; then
    wails3=("$WAILS3")
  else
    wails3=(go run github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0)
  fi
  "${wails3[@]}" generate syso -arch "$goarch" -icon build/windows/icon.ico \
    -manifest "$manifest" -info "$winfo" -out "$syso"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build "${flags[@]}" -ldflags "-s -w -H windowsgui" -o build/bin/mormicommander.exe .
  echo "built build/bin/mormicommander.exe"
  ;;
linux)
  CGO_ENABLED=${CGO_ENABLED:-1} GOOS=$goos GOARCH=$goarch go build "${flags[@]}" -ldflags "-s -w" -o build/bin/mormicommander .
  echo "built build/bin/mormicommander"
  ;;
*)
  echo "build.sh: $goos is not a system mormiCommander builds for" >&2
  exit 1
  ;;
esac
