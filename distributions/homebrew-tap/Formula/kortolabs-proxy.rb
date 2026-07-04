class KortolabsProxy < Formula
  desc "Statically linked edge AI streaming proxy gateway with Zstd cache compression"
  homepage "https://github.com/ramairwing/kotro-proxy-engine"
  version "0.2.4"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/ramairwing/kotro-proxy-engine/releases/download/v0.2.4/korto-proxy-aarch64-apple-darwin.tar.gz"
      sha256 "a06ec0c3ed9a73025015978c20c5a06af7024c146a3f1e83525ec44deb9dd2e4"
    else
      url "https://github.com/ramairwing/kotro-proxy-engine/releases/download/v0.2.4/korto-proxy-x86_64-apple-darwin.tar.gz"
      sha256 "65c1159fd1549a5a58541d891f6db578733a8d7ee9d72ac41474bd5e4683d46c"
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
