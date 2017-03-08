#!/bin/bash

set -eu
set -o pipefail

go build -o /tmp/silk

pushd vendor/github.com/containernetworking/cni/plugins/ipam/host-local > /dev/null
  go build -o /tmp/host-local
popd > /dev/null

echo '
{
  "cniVersion": "0.3.0",
  "name": "my-silk-network",
  "type": "silk",
  "ipam": {
      "type": "host-local",
      "subnet": "10.255.30.0/24",
      "routes": [ { "dst": "0.0.0.0/0" } ],
      "dataDir": "/tmp/cni/data"
   }
}
' > /tmp/silk.conf

export CNI_IFNAME=eth0
export CNI_NETNS=apricot
export CNI_PATH=/tmp

CNI_COMMAND=ADD /tmp/silk < /tmp/silk.conf
CNI_COMMAND=DEL /tmp/silk < /tmp/silk.conf
