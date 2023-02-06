#!/bin/sh
GOOS=linux GOARCH=amd64 ./build.sh -o bbctl-linux-amd64
GOOS=linux GOARCH=arm64 ./build.sh -o bbctl-linux-arm64
GOOS=darwin GOARCH=amd64 ./build.sh -o bbctl-macos-amd64
GOOS=darwin GOARCH=arm64 ./build.sh -o bbctl-macos-arm64
