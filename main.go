package main

import (
	"encoding/json"
	"log"

	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
)

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}

type NetConf struct {
	types.NetConf
}

func cmdAdd(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		log.Fatal(err)
	}
	result, err := ipam.ExecAdd(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		log.Fatal(err)
	}

	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		log.Fatal(err)
	}

	currentNS, err := ns.GetCurrentNS()
	if err != nil {
		log.Fatal(err)
	}

	creator := &veth.Creator{}

	hostVeth, containerVeth, err := creator.Pair(args.IfName, 1500, currentNS.Path(), args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	cniResult.Interfaces = append(cniResult.Interfaces,
		&current.Interface{
			Name: hostVeth.Attrs().Name,
			Mac:  hostVeth.Attrs().HardwareAddr.String(),
		},
		&current.Interface{
			Name:    containerVeth.Attrs().Name,
			Mac:     containerVeth.Attrs().HardwareAddr.String(),
			Sandbox: args.Netns,
		},
	)

	cniResult.IPs[0].Interface = -1

	return types.PrintResult(cniResult, netConf.CNIVersion)

}

func cmdDel(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		log.Fatal(err)
	}

	err = ipam.ExecDel(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		log.Fatal(err)
	}

	destroyer := &veth.Destroyer{}
	err = destroyer.Destroy(args.IfName, args.Netns)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
