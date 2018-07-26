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

	"github.com/containernetworking/cni/libcni"
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

func conflistAdd(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string) (cnitypes.Result, error) {
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := []string{binDir}
	cniNet := libcni.CNIConfig{Path: binDirs}

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return nil, fmt.Errorf("error in converting the raw bytes to conflist: %v", err)
	}

	result, err := cniNet.AddNetworkList(confList, rt)
	if err != nil {
		return nil, fmt.Errorf("error in getting result from AddNetworkList: %v", err)
	}

	return result, nil
}

func conflistDel(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string) error {
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := []string{binDir}
	cniNet := libcni.CNIConfig{Path: binDirs}

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return fmt.Errorf("error in converting the raw bytes to conflist: %v", err)
	}

	err = cniNet.DelNetworkList(confList, rt)
	if err != nil {
		return fmt.Errorf("error in getting result from DelNetworkList: %v", err)
	}

	return err
}

func delegateAdd(exec invoke.Exec, ifName string, delegate *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string) (cnitypes.Result, error) {
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return nil, fmt.Errorf("Multus: error in setting CNI_IFNAME")
	}

	if err := validateIfName(os.Getenv("CNI_NETNS"), ifName); err != nil {
		return nil, fmt.Errorf("cannot set %q ifname to %q: %v", delegate.Conf.Type, ifName, err)
	}

	if delegate.ConfListPlugin != false {
		result, err := conflistAdd(rt, delegate.Bytes, binDir)
		if err != nil {
			return nil, fmt.Errorf("Multus: error in invoke Conflist add - %q: %v", delegate.ConfList.Name, err)
		}

		return result, nil
	}

	result, err := invoke.DelegateAdd(delegate.Conf.Type, delegate.Bytes, exec)
	if err != nil {
		return nil, fmt.Errorf("Multus: error in invoke Delegate add - %q: %v", delegate.Conf.Type, err)
	}

	return result, nil
}

func delegateDel(exec invoke.Exec, ifName string, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string) error {
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return fmt.Errorf("Multus: error in setting CNI_IFNAME")
	}

	if delegateConf.ConfListPlugin != false {
		err := conflistDel(rt, delegateConf.Bytes, binDir)
		if err != nil {
			return fmt.Errorf("Multus: error in invoke Conflist Del - %q: %v", delegateConf.ConfList.Name, err)
		}

		return err
	}

	if err := invoke.DelegateDel(delegateConf.Conf.Type, delegateConf.Bytes, exec); err != nil {
		return fmt.Errorf("Multus: error in invoke Delegate del - %q: %v", delegateConf.Conf.Type, err)
	}

	return nil
}

func delPlugins(exec invoke.Exec, argIfname string, delegates []*types.DelegateNetConf, lastIdx int, rt *libcni.RuntimeConf, binDir string) error {
	if os.Setenv("CNI_COMMAND", "DEL") != nil {
		return fmt.Errorf("Multus: error in setting CNI_COMMAND to DEL")
	}

	for idx := lastIdx; idx >= 0; idx-- {
		ifName := getIfname(delegates[idx], argIfname, idx)
		rt.IfName = ifName
		if err := delegateDel(exec, ifName, delegates[idx], rt, binDir); err != nil {
			return err
		}
	}

	return nil
}

// Attempts to load Kubernetes-defined delegates and add them to the Multus config.
// Returns the number of Kubernetes-defined delegates added or an error.
func tryLoadK8sDelegates(k8sArgs *types.K8sArgs, conf *types.NetConf, kubeClient k8s.KubeClient) (int, error) {
	var err error

	kubeClient, err = k8s.GetK8sClient(conf.Kubeconfig, kubeClient)
	if err != nil {
		return 0, err
	}

	if kubeClient == nil {
		if len(conf.Delegates) == 0 {
			// No available kube client and no delegates, we can't do anything
			return 0, fmt.Errorf("must have either Kubernetes config or delegates, refer Multus README.md for the usage guide")
		}
		return 0, nil
	}

	delegates, err := k8s.GetK8sNetwork(kubeClient, k8sArgs, conf.ConfDir)
	if err != nil {
		if _, ok := err.(*k8s.NoK8sNetworkError); ok {
			return 0, nil
		}
		return 0, fmt.Errorf("Multus: Err in getting k8s network from pod: %v", err)
	}

	if err = conf.AddDelegates(delegates); err != nil {
		return 0, err
	}

	return len(delegates), nil
}

func cmdAdd(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) (cnitypes.Result, error) {
	n, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return nil, fmt.Errorf("err in loading netconf: %v", err)
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return nil, fmt.Errorf("Multus: Err in getting k8s args: %v", err)
	}

	numK8sDelegates, err := tryLoadK8sDelegates(k8sArgs, n, kubeClient)
	if err != nil {
		return nil, err
	}

	if numK8sDelegates == 0 {
		// cache the multus config if we have only Multus delegates
		if err := saveDelegates(args.ContainerID, n.CNIDir, n.Delegates); err != nil {
			return nil, fmt.Errorf("Multus: Err in saving the delegates: %v", err)
		}
	}

	var result, tmpResult cnitypes.Result
	var rt *libcni.RuntimeConf
	lastIdx := 0
	for idx, delegate := range n.Delegates {
		lastIdx = idx
		ifName := getIfname(delegate, args.IfName, idx)
		rt, _ = types.LoadCNIRuntimeConf(args, k8sArgs, ifName)
		tmpResult, err = delegateAdd(exec, ifName, delegate, rt, n.BinDir)
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
		_ = delPlugins(exec, args.IfName, n.Delegates, lastIdx, rt, n.BinDir)
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
	in, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return fmt.Errorf("Multus: Err in getting k8s args: %v", err)
	}

	numK8sDelegates, err := tryLoadK8sDelegates(k8sArgs, in, kubeClient)
	if err != nil {
		return err
	}

	if numK8sDelegates == 0 {
		// re-read the scratch multus config if we have only Multus delegates
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

	rt, _ := types.LoadCNIRuntimeConf(args, k8sArgs, "")
	return delPlugins(exec, args.IfName, in.Delegates, len(in.Delegates)-1, rt, in.BinDir)
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
