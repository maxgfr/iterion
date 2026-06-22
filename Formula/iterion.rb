class Iterion < Formula
  desc "Workflow orchestration engine with a custom DSL (.bot files)"
  homepage "https://github.com/SocialGouv/iterion"
  version "0.16.1"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-darwin-arm64"
      sha256 "7313673915a4527d5da88cff05d7485c4a63af93c7579f6153d50d14fb50fa50"
    end
    on_intel do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-darwin-amd64"
      sha256 "7192d10e75a5ba2a6696c8ee905da9bc1504cfc4d4ab28f7d8d0596d2348449a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-linux-arm64"
      sha256 "a1e8ff510be52e477400b4d6201344fe350b7dd11abe232252c998c575a9a023"
    end
    on_intel do
      url "https://github.com/SocialGouv/iterion/releases/download/v#{version}/iterion-linux-amd64"
      sha256 "76c271e5c31c1c0fa3b8c70fea1a9f0c47bc61824d94e41dd0165c2ceb6e412c"
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
