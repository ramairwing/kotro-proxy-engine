class KortolabsProxy < Formula
  desc "Statically linked edge AI streaming proxy gateway with Zstd cache compression"
  homepage "https://github.com/ramairwing/kotro-proxy-engine"
  version "0.2.6"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/ramairwing/kotro-proxy-engine/releases/download/v0.2.6/korto-proxy-aarch64-apple-darwin.tar.gz"
      sha256 "93b44cf49c0ebf52e6110ee256897133f0704cf48d325901d446a713caf0224b"
    else
      url "https://github.com/ramairwing/kotro-proxy-engine/releases/download/v0.2.6/korto-proxy-x86_64-apple-darwin.tar.gz"
      sha256 "9bf60032153384aa6dc2c1de36b34c5fb8faa4dc48b6fa06755c613ee35af324"
    end
  end

  def install
    asset = Dir["korto-proxy-*"].first
    odie "Expected exactly one korto-proxy binary in the release tarball" if asset.nil?
    bin.install asset => "kortolabs-proxy"
  end

  test do
    assert_match "korto-proxy #{version}", shell_output("#{bin}/kortolabs-proxy --version")
  end
end
