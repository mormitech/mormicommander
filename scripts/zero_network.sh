#!/usr/bin/env bash
# The zero-network test of the built app (board issue #14, FEATURES.md F-875; docs/privacy.md). Linux only.
#
#   scripts/zero_network.sh --selftest [--out DIR]                 the detector reports a planted connection (run first)
#   scripts/zero_network.sh [--out DIR] [--keep] [path/to/binary]  the session (default binary: build/bin/mormicommander)
#
# Starts the app under a virtual display and a private D-Bus session, in control mode, with a temporary HOME and
# settings folder, drives a scripted session (scripts/zero_network.py) and watches the app and every process it
# starts (WebKitGTK's web and network processes too) with strace and ss. Fails with a report of every connection to
# another machine, DNS lookup or socket open to the network. Loopback and Unix sockets are allowed.
#
# Needs: python3, strace, ss (iproute2); for the session also xvfb-run (xvfb, xauth) and dbus-run-session (dbus), and
# the runtime libraries of GTK 4 and WebKitGTK 6.0. Nothing touches your files or your settings.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ "$(uname -s)" != Linux ]; then
  echo "zero_network.sh runs on Linux only (strace, ss, /proc)" >&2
  exit 2
fi

selftest=no
app=
args=()
while [ $# -gt 0 ]; do
  case "$1" in
    --selftest) selftest=yes; args+=("$1") ;;
    --out) args+=("$1" "$2"); shift ;;
    --keep) args+=("$1") ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    -*) echo "unknown option $1" >&2; exit 2 ;;
    *) app=$1 ;;
  esac
  shift
done

need=(python3 strace ss)
[ "$selftest" = yes ] || need+=(xvfb-run xauth dbus-run-session)
missing=()
for t in "${need[@]}"; do command -v "$t" >/dev/null || missing+=("$t"); done
if [ ${#missing[@]} -gt 0 ]; then
  echo "missing tools: ${missing[*]} (Ubuntu: sudo apt-get install strace iproute2 xvfb xauth dbus)" >&2
  exit 2
fi

if [ "$selftest" = yes ]; then
  exec python3 scripts/zero_network.py "${args[@]}"
fi

app=${app:-build/bin/mormicommander}
[ -x "$app" ] || { echo "no app at $app: build it first (scripts/build.sh)" >&2; exit 2; }

# No GPU on a virtual display: GTK draws with cairo, WebKit without DMA-BUF. CI runners do not allow the user
# namespaces bubblewrap needs for WebKitGTK's web process sandbox, so it is off for this test only (never in the app);
# strace follows the web process either way.
export GSK_RENDERER="${GSK_RENDERER:-cairo}"
export WEBKIT_DISABLE_DMABUF_RENDERER="${WEBKIT_DISABLE_DMABUF_RENDERER:-1}"
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS="${WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS:-1}"
export LIBGL_ALWAYS_SOFTWARE="${LIBGL_ALWAYS_SOFTWARE:-1}"
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY all_proxy ALL_PROXY no_proxy NO_PROXY

exec python3 scripts/zero_network.py "${args[@]}" "$app"
