class KotroProxy < Formula
  desc "Local security and efficiency layer for MCP-native agentic AI — injection scanning, secret redaction, semantic cache, agent loop protection"
  homepage "https://github.com/kotro-labs/kotro-proxy-engine"
  version "0.4.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kotro-labs/kotro-proxy-engine/releases/download/v0.4.0/kotro-proxy-aarch64-apple-darwin.tar.gz"
      sha256 "c4a34af4bd9a9d2fa02589f4a8453e9b3b3ff9503a55d35126769e3916609929"
    else
      url "https://github.com/kotro-labs/kotro-proxy-engine/releases/download/v0.4.0/kotro-proxy-x86_64-apple-darwin.tar.gz"
      sha256 "26899568c9e0cfe86ec12a39254df59192ca6653832ccd3b762dfc855f097ebc"
    end
  end

  def install
    asset = Dir["kotro-proxy-*"].first
    odie "Expected exactly one kotro-proxy binary in the release tarball" if asset.nil?
    bin.install asset => "kotro-proxy"
  end

  test do
    assert_match "kotro-proxy #{version}", shell_output("#{bin}/kotro-proxy --version")
  end
end
