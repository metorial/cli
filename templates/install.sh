#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${METORIAL_CLI_BASE_URL:-https://cli.metorial.com}"
REQUESTED_VERSION="${METORIAL_CLI_VERSION:-${VERSION:-}}"
RELEASE_ROOT="${BASE_URL%/}/metorial-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
SPINNER_PID=''
SPINNER_MESSAGE=''

log() {
  printf '%s\n' "$*"
}

fail() {
  stop_spinner
  printf '\n[error] %s\n' "$*" >&2
  exit 1
}

warn() {
  printf '[warning] %s\n' "$*"
}

info() {
  printf '[info] %s\n' "$*"
}

start_spinner() {
  SPINNER_MESSAGE="$1"
  (
    while :; do
      printf '\r%s [|]' "$SPINNER_MESSAGE"
      sleep 0.1
      printf '\r%s [/]' "$SPINNER_MESSAGE"
      sleep 0.1
      printf '\r%s [-]' "$SPINNER_MESSAGE"
      sleep 0.1
      printf '\r%s [\\]' "$SPINNER_MESSAGE"
      sleep 0.1
    done
  ) &
  SPINNER_PID=$!
}

stop_spinner() {
  if [ -n "${SPINNER_PID:-}" ]; then
    kill "$SPINNER_PID" >/dev/null 2>&1 || true
    wait "$SPINNER_PID" 2>/dev/null || true
    SPINNER_PID=''
  fi
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required command: $1"
}

resolve_shell_rc() {
  current_shell=''

  current_shell="${SHELL##*/}"

  case "$current_shell" in
    bash) printf '%s/.bashrc' "$HOME" ;;
    zsh) printf '%s/.zshrc' "$HOME" ;;
    *)
      if [ -f "${HOME}/.bashrc" ]; then
        printf '%s/.bashrc' "$HOME"
        return
      fi

      if [ -f "${HOME}/.zshrc" ]; then
        printf '%s/.zshrc' "$HOME"
        return
      fi

      printf '%s/.profile' "$HOME"
      ;;
  esac
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
    printf '%s' "v${REQUESTED_VERSION#v}"
    return
  fi

  need curl
  curl -fsSL "${RELEASE_ROOT}/latest" | tr -d '\r' | head -n 1
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

resolve_managed_bin_path() {
  printf '%s/.metorial/cli/metorial' "$HOME"
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

ensure_path_in_shell_rc() {
  install_dir="$1"
  rc_file=''
  export_line=''

  case ":$PATH:" in
    *":${install_dir}:"*) return ;;
  esac

  rc_file="$(resolve_shell_rc)"
  export_line="export PATH=\"${install_dir}:\$PATH\""

  mkdir -p "$(dirname "$rc_file")"
  touch "$rc_file"

  if grep -Fq "$export_line" "$rc_file"; then
    warn "Your shell config already includes ${install_dir}, but your current shell has not loaded it yet."
    info "Run 'source ${rc_file}' or open a new terminal before using 'metorial'."
    return
  fi

  {
    printf '\n'
    printf '# Added by Metorial CLI installer\n'
    printf '%s\n' "$export_line"
  } >> "$rc_file"

  warn "Added ${install_dir} to PATH in ${rc_file}."
  info "Run 'source ${rc_file}' or open a new terminal before using 'metorial'."
}

main() {
  need curl

  printf 'Welcome to the \033[1;34mMetorial CLI\033[0m!\n'

  os=''
  os="$(detect_os)"

  arch=''
  arch="$(detect_arch)"

  version=''
  version="$(resolve_version)"
  [ -n "$version" ] || fail 'Unable to resolve the latest CLI version'

  normalized_version="${version#v}"
  archive_name="metorial_${normalized_version}_${os}_${arch}.tar.gz"
  release_base="${RELEASE_ROOT}/${version}"
  archive_path="${TMP_DIR}/${archive_name}"
  checksum_path="${TMP_DIR}/checksums.txt"
  extract_dir="${TMP_DIR}/extract"
  install_dir=''
  install_dir="$(resolve_install_dir)"
  managed_bin_path=''
  managed_bin_path="$(resolve_managed_bin_path)"
  symlink_path="${install_dir}/metorial"

  start_spinner "Downloading version ${version}"
  curl -fsSL "${release_base}/${archive_name}" -o "$archive_path"
  curl -fsSL "${release_base}/checksums.txt" -o "$checksum_path"

  verify_checksum "$checksum_path" "$archive_name" "$archive_path"
  extract_archive "$archive_path" "$extract_dir"

  mkdir -p "$(dirname "$managed_bin_path")"
  mkdir -p "$install_dir"
  install "$extract_dir/metorial" "$managed_bin_path"
  ln -sfn "$managed_bin_path" "$symlink_path"
  stop_spinner

  printf '\rSuccessfully installed \033[1;34mMetorial CLI\033[0m (%s)\n' "$version"
  printf "Get started by running 'metorial'\n"
  info "Managed binary: ${managed_bin_path}"
  info "Command symlink: ${symlink_path}"
  ensure_path_in_shell_rc "$install_dir"
}

main "$@"
