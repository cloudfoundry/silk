#!/bin/bash

set -eu
set -o pipefail

cd $(dirname $0)/..

go install ./vendor/github.com/onsi/ginkgo/ginkgo

ginkgo acceptance
