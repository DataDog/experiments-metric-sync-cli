#!/bin/sh
# Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026 Datadog, Inc.

set -eu

REPO="${REPO:-DataDog/experiments-metric-sync-cli}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="metric-sync"

fail() {
  echo "metric-sync install failed: $*" >&2
  exit 1
}

info() {
  echo "metric-sync install: $*"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo "amd64" ;;
    arm64 | aarch64) echo "arm64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

download() {
  url="$1"
  dest="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
    return
  fi

  fail "curl or wget is required"
}

sha256() {
  file="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi

  fail "sha256sum or shasum is required"
}

install_binary() {
  src="$1"
  dest="$INSTALL_DIR/$BINARY_NAME"

  mkdir -p "$INSTALL_DIR" 2>/dev/null || true

  if [ -d "$INSTALL_DIR" ] && [ -w "$INSTALL_DIR" ]; then
    install -m 0755 "$src" "$dest"
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    info "installing to $INSTALL_DIR with sudo"
    sudo mkdir -p "$INSTALL_DIR"
    sudo install -m 0755 "$src" "$dest"
    return
  fi

  fail "$INSTALL_DIR is not writable; rerun with INSTALL_DIR set to a writable directory"
}

os="$(detect_os)"
arch="$(detect_arch)"
asset="$BINARY_NAME-$os-$arch.tar.gz"

if [ "$VERSION" = "latest" ]; then
  release_url="https://github.com/$REPO/releases/latest/download"
else
  release_url="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

asset_path="$tmp_dir/$asset"
checksums_path="$tmp_dir/checksums.txt"

info "downloading $asset from $REPO ($VERSION)"
download "$release_url/$asset" "$asset_path"
download "$release_url/checksums.txt" "$checksums_path"

expected="$(awk -v asset="$asset" '$2 == asset {print $1}' "$checksums_path")"
[ -n "$expected" ] || fail "checksum entry for $asset was not found"

actual="$(sha256 "$asset_path")"
if [ "$actual" != "$expected" ]; then
  fail "checksum mismatch for $asset"
fi

info "checksum verified"
tar -xzf "$asset_path" -C "$tmp_dir"
[ -x "$tmp_dir/$BINARY_NAME" ] || fail "$BINARY_NAME was not found in $asset"

install_binary "$tmp_dir/$BINARY_NAME"

info "installed $BINARY_NAME to $INSTALL_DIR"
"$INSTALL_DIR/$BINARY_NAME" version
