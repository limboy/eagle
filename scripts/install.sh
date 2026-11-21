#!/bin/sh
# Builds Eagle for this Mac and installs it in /Applications (or
# ~/Applications when /Applications can't be written), replacing any copy
# already there, then opens it.
#
#   scripts/install.sh              build, install and open
#   scripts/install.sh --no-open    build and install only
#   INSTALL_DIR=~/Apps scripts/install.sh
set -eu

cd "$(dirname "$0")/.."

open_after=1
for arg in "$@"; do
  case "$arg" in
    --no-open) open_after=0 ;;
    -h|--help) sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "Unknown option: $arg" >&2; exit 2 ;;
  esac
done

if [ "$(uname -s)" != "Darwin" ]; then
  echo "Eagle runs on macOS only." >&2
  exit 1
fi

# The name and bundle identifier come from the configuration, so this keeps
# working if either changes.
config_value() {
  sed -n "s/^[[:space:]]*$1:[[:space:]]*\"\([^\"]*\)\".*/\1/p" mygo.config.ts | head -n 1
}
name=$(config_value name)
identifier=$(config_value identifier)
if [ -z "$name" ] || [ -z "$identifier" ]; then
  echo "Couldn't read name and identifier from mygo.config.ts." >&2
  exit 1
fi

if [ ! -d node_modules ]; then
  echo "==> Installing dependencies"
  bun install
fi

echo "==> Building $name"
# A local install needs neither the disk image nor notarization.
bun run build -skip-dmg -skip-notarize

arch=$(uname -m)
case "$arch" in
  arm64) platform=darwin-arm64 ;;
  x86_64) platform=darwin-amd64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac
app="build/$platform/$name.app"
if [ ! -d "$app" ]; then
  echo "The build didn't produce $app." >&2
  exit 1
fi

if [ -z "${INSTALL_DIR:-}" ]; then
  if [ -w /Applications ]; then
    INSTALL_DIR=/Applications
  else
    INSTALL_DIR="$HOME/Applications"
  fi
fi
mkdir -p "$INSTALL_DIR"
target="$INSTALL_DIR/$name.app"

# Quit a running copy first, so the new one replaces it cleanly.
if osascript -e "application id \"$identifier\" is running" 2>/dev/null | grep -q true; then
  echo "==> Quitting the running $name"
  osascript -e "tell application id \"$identifier\" to quit" || true
  i=0
  while osascript -e "application id \"$identifier\" is running" 2>/dev/null | grep -q true; do
    i=$((i + 1))
    if [ "$i" -gt 50 ]; then
      echo "$name didn't quit; quit it and run this again." >&2
      exit 1
    fi
    sleep 0.2
  done
fi

echo "==> Installing to $target"
rm -rf "$target"
ditto "$app" "$target"
# Tell Launch Services about the new copy, so Finder and Spotlight show its
# current icon and version.
lsregister=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
[ -x "$lsregister" ] && "$lsregister" -f "$target" || true

echo "Installed $name in $INSTALL_DIR."
if [ "$open_after" = 1 ]; then
  open "$target"
fi
