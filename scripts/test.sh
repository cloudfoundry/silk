#!/bin/bash

set -eu
set -o pipefail

cd $(dirname $0)/..

go install ./vendor/github.com/onsi/ginkgo/ginkgo

if [ "${1:-""}" = "" ]; then
  ginkgo -r -ldflags="-extldflags=-Wl,--allow-multiple-definition"
else
  ginkgo -r -ldflags="-extldflags=-Wl,--allow-multiple-definition" $1
fi
