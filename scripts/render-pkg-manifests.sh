#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 4 ]; then
  echo "Usage: $0 <version> <sha256sums-file> <out-homebrew-rb> <out-scoop-json>" >&2
  exit 2
fi

VERSION="$1"
SUMS_FILE="$2"
OUT_RB="$3"
OUT_SCOOP="$4"
BASE_URL="https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v${VERSION}"

lookup_sha() {
  local archive="$1"
  grep -E "^[0-9a-fA-F]{64}[[:space:]]+(\./)?${archive}$" "$SUMS_FILE" | awk '{print tolower($1)}' | head -n 1
}

WIN_ARCH="dtkit_${VERSION}_windows_amd64.zip"
LIN_ARCH="dtkit_${VERSION}_linux_amd64.tar.gz"
DAR_ARCH="dtkit_${VERSION}_darwin_arm64.tar.gz"

WIN_SHA="$(lookup_sha "$WIN_ARCH")"
LIN_SHA="$(lookup_sha "$LIN_ARCH")"
DAR_SHA="$(lookup_sha "$DAR_ARCH")"

for pair in "windows:$WIN_SHA" "linux:$LIN_SHA" "darwin:$DAR_SHA"; do
  name="${pair%%:*}"
  val="${pair#*:}"
  if [ -z "$val" ]; then
    echo "Missing SHA256 entry for $name in $SUMS_FILE" >&2
    exit 1
  fi
done

mkdir -p "$(dirname "$OUT_RB")" "$(dirname "$OUT_SCOOP")"

cat > "$OUT_RB" <<EOF
class Dtkit < Formula
  desc "Portable AI engineering kits, guardrails, and context engine for coding agents"
  homepage "https://github.com/truong-newbie/Agent_Kit_Impl"
  version "${VERSION}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "${BASE_URL}/${DAR_ARCH}"
      sha256 "${DAR_SHA}"
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      url "${BASE_URL}/${LIN_ARCH}"
      sha256 "${LIN_SHA}"
    end
  end

  def install
    bin.install "dtkit"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dtkit version --json")
  end
end
EOF

cat > "$OUT_SCOOP" <<EOF
{
  "version": "${VERSION}",
  "description": "Portable AI engineering kits, guardrails, and context engine for coding agents",
  "homepage": "https://github.com/truong-newbie/Agent_Kit_Impl",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "${BASE_URL}/${WIN_ARCH}",
      "hash": "${WIN_SHA}"
    }
  },
  "bin": "dtkit.exe",
  "checkver": "github",
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v\$version/dtkit_\$version_windows_amd64.zip"
      }
    },
    "hash": {
      "url": "\$baseurl/SHA256SUMS"
    }
  }
}
EOF
