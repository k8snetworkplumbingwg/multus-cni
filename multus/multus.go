// Copyright (c) 2017 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This is a "Multi-plugin".The delegate concept refered from CNI project
// It reads other plugin netconf, and then invoke them, e.g.
// flannel or sriov plugin.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	k8s "github.com/intel/multus-cni/k8sclient"
	"github.com/intel/multus-cni/types"
	"github.com/vishvananda/netlink"
)

func saveScratchNetConf(containerID, dataDir string, netconf []byte) error {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create the multus data directory(%q): %v", dataDir, err)
	}

	path := filepath.Join(dataDir, containerID)

	err := ioutil.WriteFile(path, netconf, 0600)
	if err != nil {
		return fmt.Errorf("failed to write container data in the path(%q): %v", path, err)
	}

	return err
}

func consumeScratchNetConf(containerID, dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, containerID)
	defer os.Remove(path)

	return ioutil.ReadFile(path)
}

func getIfname(delegate *types.DelegateNetConf, argif string, idx int) string {
	if delegate.IfnameRequest != "" {
		return delegate.IfnameRequest
	}
	if delegate.MasterPlugin {
		// master plugin always uses the CNI-provided interface name
		return argif
	}

	// Otherwise construct a unique interface name from the delegate's
	// position in the delegate list
	return fmt.Sprintf("net%d", idx)
}

func saveDelegates(containerID, dataDir string, delegates []*types.DelegateNetConf) error {
	delegatesBytes, err := json.Marshal(delegates)
	if err != nil {
		return fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	if err = saveScratchNetConf(containerID, dataDir, delegatesBytes); err != nil {
		return fmt.Errorf("error in saving the  delegates : %v", err)
	}

	return err
}

func validateIfName(nsname string, ifname string) error {
	podNs, err := ns.GetNS(nsname)
	if err != nil {
		return fmt.Errorf("no netns: %v", err)
	}

	err = podNs.Do(func(_ ns.NetNS) error {
		_, err := netlink.LinkByName(ifname)
		if err != nil {
			if err.Error() == "Link not found" {
				return nil
			}
			return err
		}
		return fmt.Errorf("ifname %s is already exist", ifname)
	})

	return err
}

func delegateAdd(exec invoke.Exec, ifName string, delegate *types.DelegateNetConf) (cnitypes.Result, error) {
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return nil, fmt.Errorf("Multus: error in setting CNI_IFNAME")
	}

	if err := validateIfName(os.Getenv("CNI_NETNS"), ifName); err != nil {
		return nil, fmt.Errorf("cannot set %q ifname to %q: %v", delegate.Type, ifName, err)
	}

	result, err := invoke.DelegateAdd(delegate.Type, delegate.Bytes, exec)
	if err != nil {
		return nil, fmt.Errorf("Multus: error in invoke Delegate add - %q: %v", delegate.Type, err)
	}

	return result, nil
}

func delegateDel(exec invoke.Exec, ifName string, delegateConf *types.DelegateNetConf) error {
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return fmt.Errorf("Multus: error in setting CNI_IFNAME")
	}

	if err := invoke.DelegateDel(delegateConf.Type, delegateConf.Bytes, exec); err != nil {
		return fmt.Errorf("Multus: error in invoke Delegate del - %q: %v", delegateConf.Type, err)
	}

	return nil
}

func delPlugins(exec invoke.Exec, argIfname string, delegates []*types.DelegateNetConf, lastIdx int) error {
	if os.Setenv("CNI_COMMAND", "DEL") != nil {
		return fmt.Errorf("Multus: error in setting CNI_COMMAND to DEL")
	}

	for idx := lastIdx; idx >= 0; idx-- {
		ifName := getIfname(delegates[idx], argIfname, idx)
		if err := delegateDel(exec, ifName, delegates[idx]); err != nil {
			return err
		}
	}

	return nil
}

func cmdAdd(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) (cnitypes.Result, error) {
	var nopodnet bool
	n, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return nil, fmt.Errorf("err in loading netconf: %v", err)
	}

	if n.Kubeconfig != "" {
		delegates, err := k8s.GetK8sNetwork(args, n.Kubeconfig, kubeClient, n.ConfDir)
		if err != nil {
			if _, ok := err.(*k8s.NoK8sNetworkError); ok {
				nopodnet = true
			} else {
				return nil, fmt.Errorf("Multus: Err in getting k8s network from pod: %v", err)
			}
		}

		if err = n.AddDelegates(delegates); err != nil {
			return nil, err
		}
	}

	if n.Kubeconfig == "" || nopodnet {
		if err := saveDelegates(args.ContainerID, n.CNIDir, n.Delegates); err != nil {
			return nil, fmt.Errorf("Multus: Err in saving the delegates: %v", err)
		}
	}

	var result, tmpResult cnitypes.Result
	lastIdx := 0
	for idx, delegate := range n.Delegates {
		lastIdx = idx
		ifName := getIfname(delegate, args.IfName, idx)
		tmpResult, err = delegateAdd(exec, ifName, delegate)
		if err != nil {
			break
		}

		// Master plugin result is always used if present
		if delegate.MasterPlugin || result == nil {
			result = tmpResult
		}
	}

	if err != nil {
		// Ignore errors; DEL must be idempotent anyway
		_ = delPlugins(exec, args.IfName, n.Delegates, lastIdx)
		return nil, err
	}

	return result, nil
}

func cmdGet(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) (cnitypes.Result, error) {
	in, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return nil, err
	}

	// FIXME: call all delegates

	return in.PrevResult, nil
}

func cmdDel(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) error {
	var nopodnet bool

	in, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if in.Kubeconfig != "" {
		delegates, err := k8s.GetK8sNetwork(args, in.Kubeconfig, kubeClient, in.ConfDir)
		if err != nil {
			if _, ok := err.(*k8s.NoK8sNetworkError); ok {
				nopodnet = true
			} else {
				return fmt.Errorf("Multus: Err in getting k8s network from pod: %v", err)
			}
		}

		if err = in.AddDelegates(delegates); err != nil {
			return err
		}
	}

	if in.Kubeconfig == "" || nopodnet {
		netconfBytes, err := consumeScratchNetConf(args.ContainerID, in.CNIDir)
		if err != nil {
			if os.IsNotExist(err) {
				// Per spec should ignore error if resources are missing / already removed
				return nil
			}
			return fmt.Errorf("Multus: Err in  reading the delegates: %v", err)
		}

		if err := json.Unmarshal(netconfBytes, &in.Delegates); err != nil {
			return fmt.Errorf("Multus: failed to load netconf: %v", err)
		}
	}

	return delPlugins(exec, args.IfName, in.Delegates, len(in.Delegates)-1)
}

func main() {
	skel.PluginMain(
		func(args *skel.CmdArgs) error {
			result, err := cmdAdd(args, nil, nil)
			if err != nil {
				return err
			}
			return result.Print()
		},
		func(args *skel.CmdArgs) error {
			result, err := cmdGet(args, nil, nil)
			if err != nil {
				return err
			}
			return result.Print()
		},
		func(args *skel.CmdArgs) error { return cmdDel(args, nil, nil) },
		version.All, "meta-plugin that delegates to other CNI plugins")
}
