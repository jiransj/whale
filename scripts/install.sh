#!/bin/sh

set -eu

REPO_SLUG="${REPO_SLUG:-usewhale/whale}"
OWNER="${OWNER:-}"
REPO="${REPO:-}"
VERSION="${VERSION:-latest}"
BIN_DIR="${BIN_DIR:-}"

if [ -n "$OWNER" ] && [ -n "$REPO" ]; then
  REPO_SLUG="$OWNER/$REPO"
fi

detect_os() {
  case "$(uname -s)" in
    Darwin) printf '%s\n' "darwin" ;;
    Linux) printf '%s\n' "linux" ;;
    *)
      printf '%s\n' "unsupported"
      return 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf '%s\n' "amd64" ;;
    arm64|aarch64) printf '%s\n' "arm64" ;;
    *)
      printf '%s\n' "unsupported"
      return 1
      ;;
  esac
}

sha256_cmd() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s\n' "sha256sum"
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    printf '%s\n' "shasum -a 256"
    return 0
  fi
  return 1
}

resolve_version() {
  if [ "$VERSION" != "latest" ]; then
    printf '%s\n' "$VERSION"
    return 0
  fi
  api_url="https://api.github.com/repos/$REPO_SLUG/releases/latest"
  release_json="$(curl -fsSL "$api_url")"
  tag="$(printf '%s\n' "$release_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$tag" ]; then
    printf '%s\n' "failed to resolve latest release tag from $api_url" >&2
    return 1
  fi
  printf '%s\n' "$tag"
}

default_bin_dir() {
  if [ -w /usr/local/bin ]; then
    printf '%s\n' "/usr/local/bin"
    return 0
  fi
  printf '%s\n' "$HOME/.local/bin"
}

verify_checksum() {
  file="$1"
  expected="$2"
  cmd="$(sha256_cmd)" || {
    printf '%s\n' "whale install: need sha256sum or shasum to verify downloads" >&2
    return 1
  }
  actual="$(sh -c "$cmd \"$file\"" | awk '{print $1}')"
  if [ "$actual" != "$expected" ]; then
    printf '%s\n' "whale install: checksum mismatch for $(basename "$file")" >&2
    printf '%s\n' "expected: $expected" >&2
    printf '%s\n' "actual:   $actual" >&2
    return 1
  fi
}

install_binary() {
  src="$1"
  dst="$2"
  mkdir -p "$dst"
  target="$dst/whale"
  cp "$src" "$target"
  chmod 0755 "$target"
  printf '%s\n' "$target"
}

OS="$(detect_os)" || {
  printf '%s\n' "whale install: unsupported OS: $(uname -s)" >&2
  exit 1
}

ARCH="$(detect_arch)" || {
  printf '%s\n' "whale install: unsupported architecture: $(uname -m)" >&2
  exit 1
}

if [ -z "$BIN_DIR" ]; then
  BIN_DIR="$(default_bin_dir)"
fi

if [ "$VERSION" = "latest" ]; then
  printf '%s\n' "Resolving latest whale release..."
fi
RESOLVED_VERSION="$(resolve_version)"
ASSET_NAME="whale-$OS-$ARCH"
BASE_URL="https://github.com/$REPO_SLUG/releases/download/$RESOLVED_VERSION"
TMPDIR="$(mktemp -d 2>/dev/null || mktemp -d -t whale-install)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

ASSET_PATH="$TMPDIR/$ASSET_NAME"
CHECKSUMS_PATH="$TMPDIR/checksums.txt"

printf '%s\n' "Installing whale $RESOLVED_VERSION for $OS/$ARCH"
printf '%s\n' "Downloading $ASSET_NAME..."
curl -fsSL "$BASE_URL/$ASSET_NAME" -o "$ASSET_PATH"
printf '%s\n' "Downloading checksums.txt..."
curl -fsSL "$BASE_URL/checksums.txt" -o "$CHECKSUMS_PATH"

EXPECTED_SUM="$(awk -v asset="$ASSET_NAME" '$2 == asset || $2 ~ "/"asset"$" {print $1}' "$CHECKSUMS_PATH")"
if [ -z "$EXPECTED_SUM" ]; then
  printf '%s\n' "whale install: could not find checksum for $ASSET_NAME" >&2
  exit 1
fi

printf '%s\n' "Verifying checksum..."
verify_checksum "$ASSET_PATH" "$EXPECTED_SUM"
printf '%s\n' "Installing to $BIN_DIR/whale..."
TARGET="$(install_binary "$ASSET_PATH" "$BIN_DIR")"

printf '%s\n' "Installed whale $RESOLVED_VERSION to $TARGET"
"$TARGET" --version

case ":$PATH:" in
  *:"$BIN_DIR":*) ;;
  *)
    printf '\n%s\n' "Add $BIN_DIR to your PATH to run 'whale' directly."
    ;;
esac
