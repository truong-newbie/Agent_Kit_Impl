# Template Homebrew formula for DTKit.
# Release builds generate a ready-to-publish dtkit.rb populated from SHA256SUMS
# via scripts/render-pkg-manifests.sh and attach it to each GitHub Release.
class Dtkit < Formula
  desc "Portable AI engineering kits, guardrails, and context engine for coding agents"
  homepage "https://github.com/truong-newbie/Agent_Kit_Impl"
  version "0.7.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v0.7.0/dtkit_0.7.0_darwin_arm64.tar.gz"
      sha256 "<SHA256_FROM_RELEASE_SHA256SUMS_DARWIN_ARM64>"
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      url "https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v0.7.0/dtkit_0.7.0_linux_amd64.tar.gz"
      sha256 "<SHA256_FROM_RELEASE_SHA256SUMS_LINUX_AMD64>"
    end
  end

  def install
    bin.install "dtkit"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dtkit version --json")
  end
end
