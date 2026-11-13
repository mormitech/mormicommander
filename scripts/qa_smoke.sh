#!/bin/sh
# Builds mormiCommander and checks it end to end: go vet, go test, the page's unit tests, scripts/build.sh, then the app
# itself through control mode (scripts/qa_nav.py: panels, navigation, selection, sorting, command line, control mode, in
# fresh temporary folders and in the demo). Nothing touches your files or your settings.
#
#   scripts/qa_smoke.sh            everything
#   scripts/qa_smoke.sh --quick    skip the build; test the app in build/bin as it is
set -e
cd "$(dirname "$0")/.."

if [ "$1" != "--quick" ]; then
  echo "== frontend: install, build, unit tests"
  (cd frontend && npm install --no-audit --no-fund >/dev/null && npm run build >/dev/null && npm test)
  echo "== go vet, go test"
  go vet ./...
  go test ./...
  echo "== build (scripts/build.sh)"
  scripts/build.sh --skip-frontend >/dev/null
fi

case "$(uname)" in
  Darwin) app=build/bin/mormicommander.app ;;
  *) app=build/bin/mormicommander ;;
esac
[ -e "$app" ] || { echo "no app at $app: run without --quick"; exit 1; }
echo "== $("$(dirname "$app")/mormicommander.app/Contents/MacOS/mormicommander" --version 2>/dev/null || "$app" --version)"
echo "== the app, through control mode"
if [ "$(uname)" = "Linux" ] && [ -z "$DISPLAY" ] && command -v xvfb-run >/dev/null; then
  exec xvfb-run -a python3 scripts/qa_nav.py "$app"
fi
exec python3 scripts/qa_nav.py "$app"
