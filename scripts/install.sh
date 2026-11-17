#!/usr/bin/env bash
set -euo pipefail

VERSION="0.4.0-rc.1"
BIN_DIR="${HOME}/.local/bin"
BASE_URL=""
DRY_RUN=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --bin-dir)
      BIN_DIR="${2:-}"
      shift 2
      ;;
    --base-url)
      BASE_URL="${2:-}"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: install.sh [--version <semver>] [--bin-dir <dir>] [--base-url <url>] [--dry-run]
EOF
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

SEMVER_RE='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if ! printf '%s\n' "$VERSION" | grep -Eq "$SEMVER_RE"; then
  echo "invalid semver version '$VERSION'" >&2
  exit 2
fi

if [ -z "$BIN_DIR" ] || [ "$BIN_DIR" = "/" ]; then
  echo "invalid destination bin directory: '$BIN_DIR'" >&2
  exit 2
fi

OS_RAW="${DTKIT_TEST_OS:-$(uname -s)}"
ARCH_RAW="${DTKIT_TEST_ARCH:-$(uname -m)}"

case "$OS_RAW" in
  Linux) OS_NAME="linux" ;;
  Darwin) OS_NAME="darwin" ;;
  *)
    echo "unsupported OS '$OS_RAW'. Supported release targets: linux/amd64, darwin/arm64, windows/amd64. Build from source with Go 1.27.4+ instead." >&2
    exit 1
    ;;
esac

case "$ARCH_RAW" in
  x86_64|amd64) ARCH_NAME="amd64" ;;
  arm64|aarch64) ARCH_NAME="arm64" ;;
  *)
    echo "unsupported architecture '$ARCH_RAW' on $OS_NAME. Supported Unix release targets: linux/amd64 and darwin/arm64. Build from source with Go 1.27.4+ instead." >&2
    exit 1
    ;;
esac

if [ "$OS_NAME" = "linux" ] && [ "$ARCH_NAME" != "amd64" ]; then
  echo "unsupported target ${OS_NAME}/${ARCH_NAME}: no prebuilt Linux ARM64 release asset is published yet; build from source with 'go build ./cmd/dtkit'." >&2
  exit 1
fi
if [ "$OS_NAME" = "darwin" ] && [ "$ARCH_NAME" != "arm64" ]; then
  echo "unsupported target ${OS_NAME}/${ARCH_NAME}: no prebuilt Intel macOS release asset is published yet; build from source with 'go build ./cmd/dtkit'." >&2
  exit 1
fi

ARCHIVE="dtkit_${VERSION}_${OS_NAME}_${ARCH_NAME}.tar.gz"
if [ -z "$BASE_URL" ]; then
  BASE_URL="https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v${VERSION}"
fi
BASE_URL="${BASE_URL%/}"

if command -v sha256sum >/dev/null 2>&1; then
  SHA_TOOL="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA_TOOL="shasum"
else
  echo "neither sha256sum nor shasum is installed; cannot verify archive checksum safely." >&2
  exit 1
fi

if [ "$DRY_RUN" -eq 1 ]; then
  printf 'dry-run: would verify and install %s from %s into %s/dtkit\n' "$ARCHIVE" "$BASE_URL" "$BIN_DIR"
  exit 0
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dtkit-install.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

curl -fsSL "${BASE_URL}/${ARCHIVE}" -o "${TMP_DIR}/${ARCHIVE}"
curl -fsSL "${BASE_URL}/SHA256SUMS" -o "${TMP_DIR}/SHA256SUMS"

MATCH_COUNT="$(grep -Ec "^[0-9a-fA-F]{64}[[:space:]]+(\./)?${ARCHIVE}$" "${TMP_DIR}/SHA256SUMS" || true)"
if [ "$MATCH_COUNT" -ne 1 ]; then
  echo "expected exactly 1 SHA256SUMS entry for ${ARCHIVE}, found ${MATCH_COUNT}; aborting." >&2
  exit 1
fi

EXPECTED_LINE="$(grep -E "^[0-9a-fA-F]{64}[[:space:]]+(\./)?${ARCHIVE}$" "${TMP_DIR}/SHA256SUMS")"
EXPECTED_HASH="$(printf '%s' "$EXPECTED_LINE" | awk '{print tolower($1)}')"

if [ "$SHA_TOOL" = "sha256sum" ]; then
  ACTUAL_HASH="$(sha256sum "${TMP_DIR}/${ARCHIVE}" | awk '{print tolower($1)}')"
else
  ACTUAL_HASH="$(shasum -a 256 "${TMP_DIR}/${ARCHIVE}" | awk '{print tolower($1)}')"
fi

if [ -z "$ACTUAL_HASH" ] || [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
  echo "SHA256 checksum mismatch for ${ARCHIVE}: expected ${EXPECTED_HASH}, got ${ACTUAL_HASH}; aborting before extraction." >&2
  exit 1
fi

mkdir -p "${TMP_DIR}/extracted"
tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "${TMP_DIR}/extracted"

EXTRACTED_BIN="${TMP_DIR}/extracted/dtkit"
if [ ! -f "$EXTRACTED_BIN" ]; then
  echo "archive ${ARCHIVE} did not contain expected 'dtkit' binary." >&2
  exit 1
fi
chmod 0755 "$EXTRACTED_BIN"

VER_OUT="$("$EXTRACTED_BIN" version --json)"
if ! printf '%s' "$VER_OUT" | grep -q "\"version\":\"${VERSION}\""; then
  echo "extracted binary version verification failed: ${VER_OUT}" >&2
  exit 1
fi

mkdir -p "$BIN_DIR"
DEST_BIN="${BIN_DIR%/}/dtkit"
TMP_DEST="${DEST_BIN}.tmp.$$"
cp "$EXTRACTED_BIN" "$TMP_DEST"
chmod 0755 "$TMP_DEST"

BACKUP_BIN=""
if [ -e "$DEST_BIN" ]; then
  BACKUP_BIN="${DEST_BIN}.bak.$$"
  cp "$DEST_BIN" "$BACKUP_BIN"
fi

if ! mv -f "$TMP_DEST" "$DEST_BIN"; then
  rm -f "$TMP_DEST"
  if [ -n "$BACKUP_BIN" ] && [ -e "$BACKUP_BIN" ] && [ ! -e "$DEST_BIN" ]; then
    mv -f "$BACKUP_BIN" "$DEST_BIN" || true
  fi
  echo "failed to install dtkit into ${DEST_BIN}" >&2
  exit 1
fi
rm -f "$BACKUP_BIN"

printf 'Installed %s -> %s\n' "$ARCHIVE" "$DEST_BIN"
