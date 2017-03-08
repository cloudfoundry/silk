#!/bin/bash

set -eu
set -o pipefail

cd $(dirname $0)/..

go get github.com/tools/godep
go get github.com/onsi/ginkgo
go install github.com/onsi/ginkgo/ginkgo

ginkgo acceptance
