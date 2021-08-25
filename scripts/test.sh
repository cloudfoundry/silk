#!/bin/bash

set -eu
set -o pipefail

cd $(dirname $0)/..

BIN_DIR="${PWD}/bin"
mkdir -p "${BIN_DIR}"
export PATH="${PATH}:${BIN_DIR}"

go build -o "$BIN_DIR/ginkgo" github.com/onsi/ginkgo/ginkgo

ginkgo -r -p --race -randomizeAllSpecs -randomizeSuites \
  -ldflags="-extldflags=-Wl,--allow-multiple-definition" \
  ${@}
