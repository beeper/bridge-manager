#!/bin/sh
GOOS=linux GOARCH=amd64 ./build.sh -o bbctl-linux-amd64
GOOS=linux GOARCH=arm64 ./build.sh -o bbctl-linux-arm64
GOOS=darwin GOARCH=amd64 ./build.sh -o bbctl-macos-amd64
GOOS=darwin GOARCH=arm64 ./build.sh -o bbctl-macos-arm64

# TODO
# * add WhiteBox Packages to CI environment?
# * use sed to set version in pkgproj xml
# * add code signing
if [[ true ]]; then
    mkdir ref

    lipo -create -output ./ref/bbctl bbctl-macos-amd64 bbctl-macos-arm64

    /usr/local/bin/packagesbuild \
    --reference-folder ./ref \
    --project ./bbctl.pkgproj \
    --build-folder "$(pwd)"

    rm -r ./ref
fi
