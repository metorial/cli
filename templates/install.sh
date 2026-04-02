#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${METORIAL_CLI_BASE_URL:-https://cli.metorial.com}"
REQUESTED_VERSION="${METORIAL_CLI_VERSION:-${VERSION:-}}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required command: $1"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *)
      fail 'Automatic install is currently supported on macOS and Linux. Use npm, Homebrew, Scoop, or Chocolatey on other platforms.'
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) fail "Unsupported architecture: $(uname -m)" ;;
  esac
}

resolve_version() {
  if [ -n "$REQUESTED_VERSION" ]; then
    printf '%s' "${REQUESTED_VERSION#v}"
    return
  fi

  need curl
  curl -fsSL "$BASE_URL/releases/latest.json" | sed -n 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
}

resolve_install_dir() {
  if [ -n "${METORIAL_CLI_BIN_DIR:-}" ]; then
    printf '%s' "$METORIAL_CLI_BIN_DIR"
    return
  fi

  if [ "$(id -u)" -eq 0 ]; then
    printf '/usr/local/bin'
    return
  fi

  if [ -w /usr/local/bin ]; then
    printf '/usr/local/bin'
    return
  fi

  printf '%s/.local/bin' "$HOME"
}

verify_checksum() {
  checksum_file="$1"
  archive_name="$2"
  archive_path="$3"
  expected=''
  expected="$(awk -v target="$archive_name" '$2 == target { print $1 }' "$checksum_file" | head -n 1)"
  [ -n "$expected" ] || fail "Checksum for ${archive_name} not found"

  if command -v sha256sum >/dev/null 2>&1; then
    actual=''
    actual="$(sha256sum "$archive_path" | awk '{print $1}')"
    [ "$actual" = "$expected" ] || fail 'Checksum verification failed'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    actual=''
    actual="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
    [ "$actual" = "$expected" ] || fail 'Checksum verification failed'
    return
  fi

  fail 'Neither sha256sum nor shasum is available to verify the download'
}

extract_archive() {
  archive_path="$1"
  destination="$2"
  need tar
  mkdir -p "$destination"
  tar -xzf "$archive_path" -C "$destination"
}

main() {
  need curl

  os=''
  os="$(detect_os)"

  arch=''
  arch="$(detect_arch)"

  version=''
  version="$(resolve_version)"
  [ -n "$version" ] || fail 'Unable to resolve the latest CLI version'

  archive_name="metorial_${version}_${os}_${arch}.tar.gz"
  release_base="${BASE_URL}/releases/download/v${version}"
  archive_path="${TMP_DIR}/${archive_name}"
  checksum_path="${TMP_DIR}/checksums.txt"
  extract_dir="${TMP_DIR}/extract"
  install_dir=''
  install_dir="$(resolve_install_dir)"

  log "Downloading Metorial CLI ${version} for ${os}/${arch}"
  curl -fsSL "${release_base}/${archive_name}" -o "$archive_path"
  curl -fsSL "${release_base}/checksums.txt" -o "$checksum_path"

  verify_checksum "$checksum_path" "$archive_name" "$archive_path"
  extract_archive "$archive_path" "$extract_dir"

  mkdir -p "$install_dir"
  install "$extract_dir/metorial" "$install_dir/metorial"

  log "Installed metorial to ${install_dir}/metorial"

  case ":$PATH:" in
    *":${install_dir}:"*) ;;
    *)
      log "Add ${install_dir} to your PATH if it is not already available."
      ;;
  esac

  "${install_dir}/metorial" version
}

main "$@"
