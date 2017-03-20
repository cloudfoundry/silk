package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/cloudfoundry-incubator/silk/veth"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
)

var hostNSPath string

func main() {
	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		log.Fatal(err)
	}
	hostNSPath = hostNS.Path()

	skel.PluginMain(cmdAdd, cmdDel, version.All)
}

type NetConf struct {
	types.NetConf
}

func cmdAdd(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		return err // impossible, skel package asserts JSON is valid
	}
	result, err := ipam.ExecAdd(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		return &types.Error{
			Code:    100,
			Msg:     "ipam plugin failed",
			Details: err.Error(),
		}
	}

	vethManager := veth.NewManager(hostNSPath, args.Netns)

	cniResult, err := current.NewResultFromResult(result)
	if err != nil {
		return fmt.Errorf("unable to convert result to current CNI version: %s", err) // not tested
	}
	cniResult.IPs[0].Address.Mask = net.IPv4Mask(255, 255, 255, 255)

	vethPair, err := vethManager.CreatePair(args.IfName, 1500)
	if err != nil {
		return &types.Error{
			Code:    100,
			Msg:     "creation of veth pair failed",
			Details: err.Error(),
		}
	}

	err = vethManager.DisableIPv6(vethPair)
	if err != nil {
		return fmt.Errorf("unable to disable IPv6: %s", err) // not tested
	}

	err = vethManager.AssignIP(vethPair, &cniResult.IPs[0].Address)
	if err != nil {
		return fmt.Errorf("unable to assign ip: %s", err) // not tested
	}

	cniResult.Interfaces = append(cniResult.Interfaces,
		&current.Interface{
			Name: vethPair.Host.Link.Name,
			Mac:  vethPair.Host.Link.HardwareAddr.String(),
		},
		&current.Interface{
			Name:    vethPair.Container.Link.Name,
			Mac:     vethPair.Container.Link.HardwareAddr.String(),
			Sandbox: args.Netns,
		},
	)

	cniResult.IPs[0].Interface = 1

	return types.PrintResult(cniResult, netConf.CNIVersion)

}

func cmdDel(args *skel.CmdArgs) error {
	var netConf NetConf
	err := json.Unmarshal(args.StdinData, &netConf)
	if err != nil {
		return err // impossible, skel package asserts JSON is valid
	}

	err = ipam.ExecDel(netConf.IPAM.Type, args.StdinData)
	if err != nil {
		return &types.Error{
			Code:    100,
			Msg:     "ipam plugin failed",
			Details: err.Error(),
		}
	}

	vethManager := veth.NewManager(hostNSPath, args.Netns)

	err = vethManager.Destroy(args.IfName)
	if err != nil {
		return &types.Error{
			Code:    100,
			Msg:     "deletion of veth pair failed",
			Details: err.Error(),
		}
	}

	return nil
}
