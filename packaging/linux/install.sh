#!/bin/sh
# Makes mormiCommander show up in the application menu of the Linux desktop (per user, no root needed).
# Run it from the folder the tar.gz was extracted to:  ./install.sh      Undo it with:  ./install.sh --remove
set -e
here=$(cd "$(dirname "$0")" && pwd)
apps="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
desktop="$apps/mormicommander.desktop"

if [ "$1" = "--remove" ]; then
  rm -f "$desktop"
else
  mkdir -p "$apps"
  # the desktop file names the program and the icon without a path; point both to where this copy is
  sed -e "s|^Exec=mormicommander |Exec=$here/mormicommander |" -e "s|^Icon=mormicommander|Icon=$here/appicon.png|" "$here/mormicommander.desktop" > "$desktop"
fi
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps" || true
echo "mormiCommander: done. It is in your applications menu."
