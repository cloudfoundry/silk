#!/bin/bash

set -e

this_dir="$(cd $(dirname $0) && pwd)"

pushd "$this_dir"

rm -rf out
certstrap init --common-name "ca" --passphrase ""
certstrap request-cert --common-name "client" --passphrase "" --ip "127.0.0.1"
certstrap sign client --CA "ca"

certstrap request-cert --common-name "server" --passphrase "" --ip "127.0.0.1"
certstrap sign server --CA "ca"

mv -f out/* ./
rm -rf out

popd
