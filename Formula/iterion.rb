class Iterion < Formula
  desc "Workflow orchestration engine with a custom DSL (.iter files)"
  homepage "https://github.com/SocialGouv/iterion"
  version "0.11.4"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-darwin-arm64"
      sha256 "d0e3a78cb3764b137022a9726082f7754d32e68329bb2dfabc14b1d72643b139"
    end
    on_intel do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-darwin-amd64"
      sha256 "d3ae11cdba0e60bcb88af4a8287424aa47c6839a0fa3dd95544e70ced83c4194"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-linux-arm64"
      sha256 "ec0fe97df3b631e3d9bd6f97fdd5b4f440271360bb8304da42971207e59cb072"
    end
    on_intel do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-linux-amd64"
      sha256 "16a0ffacad776d06c45d0452c5589e54a00958f027bb4b16ca22ab8d07b17977"
    end
  end

  def install
    bin.install Dir["iterion-*"].first => "iterion"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/iterion version")
  end

  livecheck do
    url :stable
    strategy :github_latest
  end
end
