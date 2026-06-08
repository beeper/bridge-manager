#!/usr/bin/env bash

set -euo pipefail

# We don't care about the specific Go version, only that Go is available.
# `go build` will then fetch the desired version.
if ! command -v go >/dev/null 2>&1; then
  echo "--- :package: installing go"
  brew install go
fi
go version

echo "--- :hammer_and_wrench: build macOS binaries"
GOOS=darwin GOARCH=amd64 ./build.sh -o bbctl-macos-amd64
GOOS=darwin GOARCH=arm64 ./build.sh -o bbctl-macos-arm64

echo "--- :key: fetch Developer ID cert into the agent keychain"
install_gems
bundle exec fastlane set_up_signing

echo "--- :apple: sign + notarize"
# sing_and_notarize comes from the CI toolkit plugin
sign_and_notarize bbctl-macos-amd64 bbctl-macos-arm64

echo "--- :lock: checksums"
shasum -a 256 bbctl-macos-amd64 bbctl-macos-arm64 > sha256sums.txt
cat sha256sums.txt
