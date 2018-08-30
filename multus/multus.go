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
	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/types"
	"github.com/vishvananda/netlink"
)

func saveScratchNetConf(containerID, dataDir string, netconf []byte) error {
	logging.Debugf("saveScratchNetConf: %s, %s, %s", containerID, dataDir, string(netconf))
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return logging.Errorf("failed to create the multus data directory(%q): %v", dataDir, err)
	}

	path := filepath.Join(dataDir, containerID)

	err := ioutil.WriteFile(path, netconf, 0600)
	if err != nil {
		return logging.Errorf("failed to write container data in the path(%q): %v", path, err)
	}

	return err
}

func consumeScratchNetConf(containerID, dataDir string) ([]byte, error) {
	logging.Debugf("consumeScratchNetConf: %s, %s", containerID, dataDir)
	path := filepath.Join(dataDir, containerID)
	defer os.Remove(path)

	return ioutil.ReadFile(path)
}

func getIfname(delegate *types.DelegateNetConf, argif string, idx int) string {
	logging.Debugf("getIfname: %v, %s, %d", delegate, argif, idx)
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
	logging.Debugf("saveDelegates: %s, %s, %v", containerID, dataDir, delegates)
	delegatesBytes, err := json.Marshal(delegates)
	if err != nil {
		return logging.Errorf("error serializing delegate netconf: %v", err)
	}

	if err = saveScratchNetConf(containerID, dataDir, delegatesBytes); err != nil {
		return logging.Errorf("error in saving the  delegates : %v", err)
	}

	return err
}

func validateIfName(nsname string, ifname string) error {
	logging.Debugf("validateIfName: %s, %s", nsname, ifname)
	podNs, err := ns.GetNS(nsname)
	if err != nil {
		return logging.Errorf("no netns: %v", err)
	}

	err = podNs.Do(func(_ ns.NetNS) error {
		_, err := netlink.LinkByName(ifname)
		if err != nil {
			if err.Error() == "Link not found" {
				return nil
			}
			return err
		}
		return logging.Errorf("ifname %s is already exist", ifname)
	})

	return err
}

func conflistAdd(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string) (cnitypes.Result, error) {
	logging.Debugf("conflistAdd: %v, %s, %s", rt, string(rawnetconflist), binDir)
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := []string{binDir}
	cniNet := libcni.CNIConfig{Path: binDirs}

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return nil, logging.Errorf("error in converting the raw bytes to conflist: %v", err)
	}

	result, err := cniNet.AddNetworkList(confList, rt)
	if err != nil {
		return nil, logging.Errorf("error in getting result from AddNetworkList: %v", err)
	}

	return result, nil
}

func conflistDel(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string) error {
	logging.Debugf("conflistDel: %v, %s, %s", rt, string(rawnetconflist), binDir)
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := []string{binDir}
	cniNet := libcni.CNIConfig{Path: binDirs}

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return logging.Errorf("error in converting the raw bytes to conflist: %v", err)
	}

	err = cniNet.DelNetworkList(confList, rt)
	if err != nil {
		return logging.Errorf("error in getting result from DelNetworkList: %v", err)
	}

	return err
}

func delegateAdd(exec invoke.Exec, ifName string, delegate *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string) (cnitypes.Result, error) {
	logging.Debugf("delegateAdd: %v, %s, %v, %v, %s", exec, ifName, delegate, rt, binDir)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return nil, logging.Errorf("Multus: error in setting CNI_IFNAME")
	}

	if err := validateIfName(os.Getenv("CNI_NETNS"), ifName); err != nil {
		return nil, logging.Errorf("cannot set %q ifname to %q: %v", delegate.Conf.Type, ifName, err)
	}

	if delegate.ConfListPlugin != false {
		result, err := conflistAdd(rt, delegate.Bytes, binDir)
		if err != nil {
			return nil, logging.Errorf("Multus: error in invoke Conflist add - %q: %v", delegate.ConfList.Name, err)
		}

		return result, nil
	}

	result, err := invoke.DelegateAdd(delegate.Conf.Type, delegate.Bytes, exec)
	if err != nil {
		return nil, logging.Errorf("Multus: error in invoke Delegate add - %q: %v", delegate.Conf.Type, err)
	}

	return result, nil
}

func delegateDel(exec invoke.Exec, ifName string, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string) error {
	logging.Debugf("delegateDel: %v, %s, %v, %v, %s", exec, ifName, delegateConf, rt, binDir)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return logging.Errorf("Multus: error in setting CNI_IFNAME")
	}

	if delegateConf.ConfListPlugin != false {
		err := conflistDel(rt, delegateConf.Bytes, binDir)
		if err != nil {
			return logging.Errorf("Multus: error in invoke Conflist Del - %q: %v", delegateConf.ConfList.Name, err)
		}

		return err
	}

	if err := invoke.DelegateDel(delegateConf.Conf.Type, delegateConf.Bytes, exec); err != nil {
		return logging.Errorf("Multus: error in invoke Delegate del - %q: %v", delegateConf.Conf.Type, err)
	}

	return nil
}

func delPlugins(exec invoke.Exec, argIfname string, delegates []*types.DelegateNetConf, lastIdx int, rt *libcni.RuntimeConf, binDir string) error {
	logging.Debugf("delPlugins: %v, %s, %v, %d, %v, %s", exec, argIfname, delegates, lastIdx, rt, binDir)
	if os.Setenv("CNI_COMMAND", "DEL") != nil {
		return logging.Errorf("Multus: error in setting CNI_COMMAND to DEL")
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

func cmdAdd(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) (cnitypes.Result, error) {
	logging.Debugf("cmdAdd: %v, %v, %v", args, exec, kubeClient)
	n, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return nil, logging.Errorf("err in loading netconf: %v", err)
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return nil, logging.Errorf("Multus: Err in getting k8s args: %v", err)
	}

	numK8sDelegates, kc, err := k8s.TryLoadK8sDelegates(k8sArgs, n, kubeClient)
	if err != nil {
		return nil, logging.Errorf("Multus: Err in loading K8s Delegates k8s args: %v", err)
	}

	if numK8sDelegates == 0 {
		// cache the multus config if we have only Multus delegates
		if err := saveDelegates(args.ContainerID, n.CNIDir, n.Delegates); err != nil {
			return nil, logging.Errorf("Multus: Err in saving the delegates: %v", err)
		}
	}

	var result, tmpResult cnitypes.Result
	var netStatus []*types.NetworkStatus
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

		//create the network status, only in case Multus as kubeconfig
		if n.Kubeconfig != "" && kc != nil {
			if kc.Podnamespace != "kube-system" {
				delegateNetStatus, err := types.LoadNetworkStatus(tmpResult, delegate.Conf.Name, delegate.MasterPlugin)
				if err != nil {
					return nil, logging.Errorf("Multus: Err in setting  networks status: %v", err)
				}

				netStatus = append(netStatus, delegateNetStatus)
			}
		}
	}

	if err != nil {
		// Ignore errors; DEL must be idempotent anyway
		_ = delPlugins(exec, args.IfName, n.Delegates, lastIdx, rt, n.BinDir)
		return nil, logging.Errorf("Multus: Err in tearing down failed plugins: %v", err)
	}

	//set the network status annotation in apiserver, only in case Multus as kubeconfig
	if n.Kubeconfig != "" && kc != nil {
		if kc.Podnamespace != "kube-system" {
			err = k8s.SetNetworkStatus(kc, netStatus)
			if err != nil {
				return nil, logging.Errorf("Multus: Err set the networks status: %v", err)
			}
		}
	}

	return result, nil
}

func cmdGet(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) (cnitypes.Result, error) {
	logging.Debugf("cmdGet: %v, %v, %v", args, exec, kubeClient)
	in, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return nil, err
	}

	// FIXME: call all delegates

	return in.PrevResult, nil
}

func cmdDel(args *skel.CmdArgs, exec invoke.Exec, kubeClient k8s.KubeClient) error {
	logging.Debugf("cmdDel: %v, %v, %v", args, exec, kubeClient)
	in, err := types.LoadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if args.Netns == "" {
		return nil
	}
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		//  if NetNs is passed down by the Cloud Orchestration Engine, or if it called multiple times
		// so don't return an error if the device is already removed.
		// https://github.com/kubernetes/kubernetes/issues/43014#issuecomment-287164444
		_, ok := err.(ns.NSPathNotExistErr)
		if ok {
			return nil
		}
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return logging.Errorf("Multus: Err in getting k8s args: %v", err)
	}

	numK8sDelegates, kc, err := k8s.TryLoadK8sDelegates(k8sArgs, in, kubeClient)
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
			return logging.Errorf("Multus: Err in  reading the delegates: %v", err)
		}

		if err := json.Unmarshal(netconfBytes, &in.Delegates); err != nil {
			return logging.Errorf("Multus: failed to load netconf: %v", err)
		}
	}

	//unset the network status annotation in apiserver, only in case Multus as kubeconfig
	if in.Kubeconfig != "" && kc != nil {
		if kc.Podnamespace != "kube-system" {
			err := k8s.SetNetworkStatus(kc, nil)
			if err != nil {
				return logging.Errorf("Multus: Err unset the networks status: %v", err)
			}
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
