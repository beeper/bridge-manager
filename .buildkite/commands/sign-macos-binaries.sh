#!/usr/bin/env bash

set -euo pipefail

# Go isn't guaranteed on the macOS signing image, which exists primarily for the
# Xcode signing toolchain. Install via Homebrew if absent; `go build` then
# auto-fetches the toolchain pinned in go.mod (toolchain go1.26.0).
if ! command -v go >/dev/null 2>&1; then
  echo "--- :package: installing go"
  brew install go
fi
go version

echo "--- :hammer_and_wrench: build macOS binaries"
# Same names as the published release assets, so the signed binaries are drop-in.
GOOS=darwin GOARCH=amd64 ./build.sh -o bbctl-macos-amd64
GOOS=darwin GOARCH=arm64 ./build.sh -o bbctl-macos-arm64

echo "--- :key: fetch Developer ID cert into the agent keychain"
install_gems
bundle exec fastlane set_up_signing

echo "--- :apple: sign + notarize"
# Plain CLI under hardened runtime — no entitlements. The toolkit command
# resolves the Developer ID identity from the keychain by team id.
sign_and_notarize bbctl-macos-amd64 bbctl-macos-arm64

echo "--- :lock: checksums"
shasum -a 256 bbctl-macos-amd64 bbctl-macos-arm64 > sha256sums.txt
cat sha256sums.txt
