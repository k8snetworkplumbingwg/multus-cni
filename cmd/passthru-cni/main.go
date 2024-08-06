// Package: passthru-cni
package main

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	cniVersion "github.com/containernetworking/cni/pkg/version"
)

// NetConf is a CNI configuration structure
type NetConf struct {
	cniTypes.NetConf
}

func main() {
	skel.PluginMain(
		cmdAdd,
		nil,
		cmdDel,
		cniVersion.PluginSupports("0.3.0", "0.3.1", "0.4.0", "1.0.0", "1.1.0"),
		"Passthrough CNI Plugin v1.0",
	)
}

func cmdAdd(args *skel.CmdArgs) error {
	n, err := loadNetConf(args.StdinData)
	if err != nil {
		return fmt.Errorf("passthru cni: error parsing CNI configuration: %s", err)
	}

	// Create an empty but valid CNI result
	result := &current.Result{
		CNIVersion: n.CNIVersion,
		Interfaces: []*current.Interface{},
		IPs:        []*current.IPConfig{},
		Routes:     []*cniTypes.Route{},
		DNS:        cniTypes.DNS{},
	}

	return cniTypes.PrintResult(result, n.CNIVersion)
}

func cmdDel(_ *skel.CmdArgs) error {
	// Nothing to do for DEL command, just return nil
	return nil
}

func loadNetConf(bytes []byte) (*NetConf, error) {
	n := &NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("passthru cni: failed to load netconf: %s", err)
	}
	return n, nil
}
