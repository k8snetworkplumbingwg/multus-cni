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

package multus

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	cnicurrent "github.com/containernetworking/cni/pkg/types/current"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	k8s "gopkg.in/intel/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/intel/multus-cni.v3/pkg/logging"
	"gopkg.in/intel/multus-cni.v3/pkg/netutils"
	"gopkg.in/intel/multus-cni.v3/pkg/types"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	version = "master@git"
	commit  = "unknown commit"
	date    = "unknown date"
)

var (
	pollDuration = 1000 * time.Millisecond
	pollTimeout  = 45 * time.Second
)

func PrintVersionString() string {
	return fmt.Sprintf("multus-cni version:%s, commit:%s, date:%s",
		version, commit, date)
}

func saveScratchNetConf(containerID, dataDir string, netconf []byte) error {
	logging.Debugf("saveScratchNetConf: %s, %s, %s", containerID, dataDir, string(netconf))
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return logging.Errorf("saveScratchNetConf: failed to create the multus data directory(%q): %v", dataDir, err)
	}

	path := filepath.Join(dataDir, containerID)

	err := ioutil.WriteFile(path, netconf, 0600)
	if err != nil {
		return logging.Errorf("saveScratchNetConf: failed to write container data in the path(%q): %v", path, err)
	}

	return err
}

func consumeScratchNetConf(containerID, dataDir string) ([]byte, string, error) {
	logging.Debugf("consumeScratchNetConf: %s, %s", containerID, dataDir)
	path := filepath.Join(dataDir, containerID)

	b, err := ioutil.ReadFile(path)
	return b, path, err
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

func getDelegateDeviceInfo(delegate *types.DelegateNetConf, runtimeConf *libcni.RuntimeConf) (*nettypes.DeviceInfo, error) {
	// If the DPDeviceInfoFile was created, it was copied to the CNIDeviceInfoFile.
	// If the DPDeviceInfoFile was not created, CNI might have created it. So
	// either way, load CNIDeviceInfoFile.
	if info, ok := runtimeConf.CapabilityArgs["CNIDeviceInfoFile"]; ok {
		if infostr, ok := info.(string); ok {
			return nadutils.LoadDeviceInfoFromCNI(infostr)
		}
	} else {
		logging.Debugf("getDelegateDeviceInfo(): No CapArgs - info=%v ok=%v", info, ok)
	}
	return nil, nil
}

func saveDelegates(containerID, dataDir string, delegates []*types.DelegateNetConf) error {
	logging.Debugf("saveDelegates: %s, %s, %v", containerID, dataDir, delegates)
	delegatesBytes, err := json.Marshal(delegates)
	if err != nil {
		return logging.Errorf("saveDelegates: error serializing delegate netconf: %v", err)
	}

	if err = saveScratchNetConf(containerID, dataDir, delegatesBytes); err != nil {
		return logging.Errorf("saveDelegates: error in saving the delegates : %v", err)
	}

	return err
}

func deleteDelegates(containerID, dataDir string) error {
	logging.Debugf("deleteDelegates: %s, %s", containerID, dataDir)

	path := filepath.Join(dataDir, containerID)
	if err := os.Remove(path); err != nil {
		return logging.Errorf("deleteDelegates: error in deleting the delegates : %v", err)
	}

	return nil
}

func validateIfName(nsname string, ifname string) error {
	logging.Debugf("validateIfName: %s, %s", nsname, ifname)
	podNs, err := ns.GetNS(nsname)
	if err != nil {
		return logging.Errorf("validateIfName: no net namespace %s found: %v", nsname, err)
	}

	err = podNs.Do(func(_ ns.NetNS) error {
		_, err := netlink.LinkByName(ifname)
		if err != nil {
			if err.Error() == "Link not found" {
				return nil
			}
			return err
		}
		return logging.Errorf("validateIfName: interface name %s already exists", ifname)
	})

	return err
}

func confAdd(rt *libcni.RuntimeConf, rawNetconf []byte, binDir string, exec invoke.Exec) (cnitypes.Result, error) {
	logging.Debugf("confAdd: %v, %s, %s", rt, string(rawNetconf), binDir)
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{binDir}, binDirs...)
	cniNet := libcni.NewCNIConfig(binDirs, exec)

	conf, err := libcni.ConfFromBytes(rawNetconf)
	if err != nil {
		return nil, logging.Errorf("error in converting the raw bytes to conf: %v", err)
	}

	result, err := cniNet.AddNetwork(context.Background(), conf, rt)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func confCheck(rt *libcni.RuntimeConf, rawNetconf []byte, binDir string, exec invoke.Exec) error {
	logging.Debugf("confCheck: %v, %s, %s", rt, string(rawNetconf), binDir)

	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{binDir}, binDirs...)
	cniNet := libcni.NewCNIConfig(binDirs, exec)

	conf, err := libcni.ConfFromBytes(rawNetconf)
	if err != nil {
		return logging.Errorf("error in converting the raw bytes to conf: %v", err)
	}

	err = cniNet.CheckNetwork(context.Background(), conf, rt)
	if err != nil {
		return logging.Errorf("error in getting result from DelNetwork: %v", err)
	}

	return err
}

func confDel(rt *libcni.RuntimeConf, rawNetconf []byte, binDir string, exec invoke.Exec) error {
	logging.Debugf("conflistDel: %v, %s, %s", rt, string(rawNetconf), binDir)
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{binDir}, binDirs...)
	cniNet := libcni.NewCNIConfig(binDirs, exec)

	conf, err := libcni.ConfFromBytes(rawNetconf)
	if err != nil {
		return logging.Errorf("error in converting the raw bytes to conf: %v", err)
	}

	err = cniNet.DelNetwork(context.Background(), conf, rt)
	if err != nil {
		return logging.Errorf("error in getting result from DelNetwork: %v", err)
	}

	return err
}

func conflistAdd(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string, exec invoke.Exec) (cnitypes.Result, error) {
	logging.Debugf("conflistAdd: %v, %s, %s", rt, string(rawnetconflist), binDir)
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{binDir}, binDirs...)
	cniNet := libcni.NewCNIConfig(binDirs, exec)

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return nil, logging.Errorf("conflistAdd: error converting the raw bytes into a conflist: %v", err)
	}

	result, err := cniNet.AddNetworkList(context.Background(), confList, rt)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func conflistCheck(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string, exec invoke.Exec) error {
	logging.Debugf("conflistCheck: %v, %s, %s", rt, string(rawnetconflist), binDir)

	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{binDir}, binDirs...)
	cniNet := libcni.NewCNIConfig(binDirs, exec)

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return logging.Errorf("conflistCheck: error converting the raw bytes into a conflist: %v", err)
	}

	err = cniNet.CheckNetworkList(context.Background(), confList, rt)
	if err != nil {
		return logging.Errorf("conflistCheck: error in getting result from CheckNetworkList: %v", err)
	}

	return err
}

func conflistDel(rt *libcni.RuntimeConf, rawnetconflist []byte, binDir string, exec invoke.Exec) error {
	logging.Debugf("conflistDel: %v, %s, %s", rt, string(rawnetconflist), binDir)
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{binDir}, binDirs...)
	cniNet := libcni.NewCNIConfig(binDirs, exec)

	confList, err := libcni.ConfListFromBytes(rawnetconflist)
	if err != nil {
		return logging.Errorf("conflistDel: error converting the raw bytes into a conflist: %v", err)
	}

	err = cniNet.DelNetworkList(context.Background(), confList, rt)
	if err != nil {
		return logging.Errorf("conflistDel: error in getting result from DelNetworkList: %v", err)
	}

	return err
}

func delegateAdd(exec invoke.Exec, kubeClient *k8s.ClientInfo, pod *v1.Pod, ifName string, delegate *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string, cniArgs string) (cnitypes.Result, error) {
	logging.Debugf("delegateAdd: %v, %s, %v, %v, %s", exec, ifName, delegate, rt, binDir)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return nil, logging.Errorf("delegateAdd: error setting envionment variable CNI_IFNAME")
	}

	if err := validateIfName(os.Getenv("CNI_NETNS"), ifName); err != nil {
		return nil, logging.Errorf("delegateAdd: cannot set %q interface name to %q: %v", delegate.Conf.Type, ifName, err)
	}

	// Deprecated in ver 3.5.
	if delegate.MacRequest != "" || delegate.IPRequest != nil {
		if cniArgs != "" {
			cniArgs = fmt.Sprintf("%s;IgnoreUnknown=true", cniArgs)
		} else {
			cniArgs = "IgnoreUnknown=true"
		}
		if delegate.MacRequest != "" {
			// validate Mac address
			_, err := net.ParseMAC(delegate.MacRequest)
			if err != nil {
				return nil, logging.Errorf("delegateAdd: failed to parse mac address %q", delegate.MacRequest)
			}

			cniArgs = fmt.Sprintf("%s;MAC=%s", cniArgs, delegate.MacRequest)
			logging.Debugf("delegateAdd: set MAC address %q to %q", delegate.MacRequest, ifName)
			rt.Args = append(rt.Args, [2]string{"MAC", delegate.MacRequest})
		}

		if delegate.IPRequest != nil {
			// validate IP address
			for _, ip := range delegate.IPRequest {
				if strings.Contains(ip, "/") {
					_, _, err := net.ParseCIDR(ip)
					if err != nil {
						return nil, logging.Errorf("delegateAdd: failed to parse IP address %q", ip)
					}
				} else if net.ParseIP(ip) == nil {
					return nil, logging.Errorf("delegateAdd: failed to parse IP address %q", ip)
				}
			}

			ips := strings.Join(delegate.IPRequest, ",")
			cniArgs = fmt.Sprintf("%s;IP=%s", cniArgs, ips)
			logging.Debugf("delegateAdd: set IP address %q to %q", ips, ifName)
			rt.Args = append(rt.Args, [2]string{"IP", ips})
		}
	}

	var result cnitypes.Result
	var err error
	if delegate.ConfListPlugin {
		result, err = conflistAdd(rt, delegate.Bytes, binDir, exec)
		if err != nil {
			return nil, err
		}
	} else {
		result, err = confAdd(rt, delegate.Bytes, binDir, exec)
		if err != nil {
			return nil, err
		}
	}

	if logging.GetLoggingLevel() >= logging.VerboseLevel {
		data, _ := json.Marshal(result)
		var cniConfName string
		if delegate.ConfListPlugin {
			cniConfName = delegate.ConfList.Name
		} else {
			cniConfName = delegate.Conf.Name
		}

		podUID := "unknownUID"
		if pod != nil {
			podUID = string(pod.ObjectMeta.UID)
		}
		logging.Verbosef("Add: %s:%s:%s:%s(%s):%s %s", rt.Args[1][1], rt.Args[2][1], podUID, delegate.Name, cniConfName, rt.IfName, string(data))
	}

	// get IP addresses from result
	ips := []string{}
	res, err := cnicurrent.NewResultFromResult(result)
	if err != nil {
		logging.Errorf("delegateAdd: error converting result: %v", err)
		return result, nil
	}
	for _, ip := range res.IPs {
		ips = append(ips, ip.Address.String())
	}

	if pod != nil {
		// send kubernetes events
		if delegate.Name != "" {
			kubeClient.Eventf(pod, v1.EventTypeNormal, "AddedInterface", "Add %s %v from %s", rt.IfName, ips, delegate.Name)
		} else {
			kubeClient.Eventf(pod, v1.EventTypeNormal, "AddedInterface", "Add %s %v", rt.IfName, ips)
		}
	} else {
		// for further debug https://github.com/intel/multus-cni/issues/481
		logging.Errorf("delegateAdd: pod nil pointer: namespace: %s, name: %s, container id: %s, pod: %v", rt.Args[1][1], rt.Args[2][1], rt.Args[3][1], pod)
	}

	return result, nil
}

func delegateCheck(exec invoke.Exec, ifName string, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string) error {
	logging.Debugf("delegateCheck: %v, %s, %v, %v, %s", exec, ifName, delegateConf, rt, binDir)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return logging.Errorf("delegateCheck: error setting envionment variable CNI_IFNAME")
	}

	if logging.GetLoggingLevel() >= logging.VerboseLevel {
		var cniConfName string
		if delegateConf.ConfListPlugin {
			cniConfName = delegateConf.ConfList.Name
		} else {
			cniConfName = delegateConf.Conf.Name
		}
		logging.Verbosef("Check: %s:%s:%s(%s):%s %s", rt.Args[1][1], rt.Args[2][1], delegateConf.Name, cniConfName, rt.IfName, string(delegateConf.Bytes))
	}

	var err error
	if delegateConf.ConfListPlugin {
		err = conflistCheck(rt, delegateConf.Bytes, binDir, exec)
		if err != nil {
			return logging.Errorf("delegateCheck: error invoking ConflistCheck - %q: %v", delegateConf.ConfList.Name, err)
		}
	} else {
		err = confCheck(rt, delegateConf.Bytes, binDir, exec)
		if err != nil {
			return logging.Errorf("delegateCheck: error invoking DelegateCheck - %q: %v", delegateConf.Conf.Type, err)
		}
	}

	return err
}

func delegateDel(exec invoke.Exec, pod *v1.Pod, ifName string, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, binDir string) error {
	logging.Debugf("delegateDel: %v, %v, %s, %v, %v, %s", exec, pod, ifName, delegateConf, rt, binDir)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return logging.Errorf("delegateDel: error setting envionment variable CNI_IFNAME")
	}

	if logging.GetLoggingLevel() >= logging.VerboseLevel {
		var confName string
		if delegateConf.ConfListPlugin {
			confName = delegateConf.ConfList.Name
		} else {
			confName = delegateConf.Conf.Name
		}
		podUID := "unknownUID"
		if pod != nil {
			podUID = string(pod.ObjectMeta.UID)
		}
		logging.Verbosef("Del: %s:%s:%s:%s:%s %s", rt.Args[1][1], rt.Args[2][1], podUID, confName, rt.IfName, string(delegateConf.Bytes))
	}

	var err error
	if delegateConf.ConfListPlugin {
		err = conflistDel(rt, delegateConf.Bytes, binDir, exec)
		if err != nil {
			return logging.Errorf("delegateDel: error invoking ConflistDel - %q: %v", delegateConf.ConfList.Name, err)
		}
	} else {
		err = confDel(rt, delegateConf.Bytes, binDir, exec)
		if err != nil {
			return logging.Errorf("delegateDel: error invoking DelegateDel - %q: %v", delegateConf.Conf.Type, err)
		}
	}

	return err
}

// delPlugins deletes plugins in reverse order from lastdIdx
// Uses netRt as base RuntimeConf (coming from NetConf) but merges it
// with each of the delegates' configuration
func delPlugins(exec invoke.Exec, pod *v1.Pod, args *skel.CmdArgs, k8sArgs *types.K8sArgs, delegates []*types.DelegateNetConf, lastIdx int, netRt *types.RuntimeConfig, binDir string) error {
	logging.Debugf("delPlugins: %v, %v, %v, %v, %v, %d, %v, %s", exec, pod, args, k8sArgs, delegates, lastIdx, netRt, binDir)
	if os.Setenv("CNI_COMMAND", "DEL") != nil {
		return logging.Errorf("delPlugins: error setting environment variable CNI_COMMAND to a value of DEL")
	}

	var errorstrings []string
	for idx := lastIdx; idx >= 0; idx-- {
		ifName := getIfname(delegates[idx], args.IfName, idx)
		rt, cniDeviceInfoPath := types.CreateCNIRuntimeConf(args, k8sArgs, ifName, netRt, delegates[idx])
		// Attempt to delete all but do not error out, instead, collect all errors.
		if err := delegateDel(exec, pod, ifName, delegates[idx], rt, binDir); err != nil {
			errorstrings = append(errorstrings, err.Error())
		}
		if cniDeviceInfoPath != "" {
			err := nadutils.CleanDeviceInfoForCNI(cniDeviceInfoPath)
			// Even if the filename is set, file may not be present. Ignore error,
			// but log and in the future may need to filter on specific errors.
			if err != nil {
				logging.Debugf("delPlugins: CleanDeviceInfoForCNI returned an error - err=%v", err)
			}
		}
	}

	// Check if we had any errors, and send them all back.
	if len(errorstrings) > 0 {
		return fmt.Errorf(strings.Join(errorstrings, " / "))
	}

	return nil
}

func cmdErr(k8sArgs *types.K8sArgs, format string, args ...interface{}) error {
	prefix := "Multus: "
	if k8sArgs != nil {
		prefix += fmt.Sprintf("[%s/%s]: ", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME)
	}
	return logging.Errorf(prefix+format, args...)
}

func cmdPluginErr(k8sArgs *types.K8sArgs, confName string, format string, args ...interface{}) error {
	msg := ""
	if k8sArgs != nil {
		msg += fmt.Sprintf("[%s/%s:%s]: ", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME, confName)
	}
	return logging.Errorf(msg+format, args...)
}

func CmdAdd(args *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) (cnitypes.Result, error) {
	n, err := types.LoadNetConf(args.StdinData)
	logging.Debugf("CmdAdd: %v, %v, %v", args, exec, kubeClient)
	if err != nil {
		return nil, cmdErr(nil, "error loading netconf: %v", err)
	}

	kubeClient, err = k8s.GetK8sClient(n.Kubeconfig, kubeClient)
	if err != nil {
		return nil, cmdErr(nil, "error getting k8s client: %v", err)
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return nil, cmdErr(nil, "error getting k8s args: %v", err)
	}

	if n.ReadinessIndicatorFile != "" {
		err := wait.PollImmediate(pollDuration, pollTimeout, func() (bool, error) {
			_, err := os.Stat(n.ReadinessIndicatorFile)
			return err == nil, nil
		})
		if err != nil {
			return nil, cmdErr(k8sArgs, "PollImmediate error waiting for ReadinessIndicatorFile: %v", err)
		}
	}

	pod := (*v1.Pod)(nil)
	if kubeClient != nil {
		pod, err = kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		if err != nil {
			var waitErr error
			// in case of ServiceUnavailable, retry 10 times with 0.5 sec interval
			if errors.IsServiceUnavailable(err) {
				pollDuration := 500 * time.Millisecond
				pollTimeout := 5 * time.Second
				waitErr = wait.PollImmediate(pollDuration, pollTimeout, func() (bool, error) {
					pod, err = kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
					return pod != nil, err
				})
				// retry failed, then return error with retry out
				if waitErr != nil {
					return nil, cmdErr(k8sArgs, "error getting pod by service unavailable: %v", err)
				}
			} else {
				// Other case, return error
				return nil, cmdErr(k8sArgs, "error getting pod: %v", err)
			}
		}
	}

	// resourceMap holds Pod device allocation information; only initizized if CRD contains 'resourceName' annotation.
	// This will only be initialized once and all delegate objects can reference this to look up device info.
	var resourceMap map[string]*types.ResourceInfo

	if n.ClusterNetwork != "" {
		resourceMap, err = k8s.GetDefaultNetworks(pod, n, kubeClient, resourceMap)
		if err != nil {
			return nil, cmdErr(k8sArgs, "failed to get clusterNetwork/defaultNetworks: %v", err)
		}
		// First delegate is always the master plugin
		n.Delegates[0].MasterPlugin = true
	}

	_, kc, err := k8s.TryLoadPodDelegates(pod, n, kubeClient, resourceMap)
	if err != nil {
		return nil, cmdErr(k8sArgs, "error loading k8s delegates k8s args: %v", err)
	}

	// cache the multus config
	if err := saveDelegates(args.ContainerID, n.CNIDir, n.Delegates); err != nil {
		return nil, cmdErr(k8sArgs, "error saving the delegates: %v", err)
	}

	var result, tmpResult cnitypes.Result
	var netStatus []nettypes.NetworkStatus
	cniArgs := os.Getenv("CNI_ARGS")
	for idx, delegate := range n.Delegates {
		ifName := getIfname(delegate, args.IfName, idx)
		rt, cniDeviceInfoPath := types.CreateCNIRuntimeConf(args, k8sArgs, ifName, n.RuntimeConfig, delegate)
		if cniDeviceInfoPath != "" {
			err = nadutils.CopyDeviceInfoForCNIFromDP(cniDeviceInfoPath, delegate.ResourceName, delegate.DeviceID)
			// Even if the filename is set, file may not be present. Ignore error,
			// but log and in the future may need to filter on specific errors.
			if err != nil {
				logging.Debugf("cmdAdd: CopyDeviceInfoForCNIFromDP returned an error - err=%v", err)
			}
		}

		tmpResult, err = delegateAdd(exec, kubeClient, pod, ifName, delegate, rt, n.BinDir, cniArgs)
		if err != nil {
			// If the add failed, tear down all networks we already added
			netName := delegate.Conf.Name
			if netName == "" {
				netName = delegate.ConfList.Name
			}
			// Ignore errors; DEL must be idempotent anyway
			_ = delPlugins(exec, nil, args, k8sArgs, n.Delegates, idx, n.RuntimeConfig, n.BinDir)
			return nil, cmdPluginErr(k8sArgs, netName, "error adding container to network %q: %v", netName, err)
		}

		// Remove gateway from routing table if the gateway is not used
		deletegateway := false
		adddefaultgateway := false
		if delegate.IsFilterGateway {
			deletegateway = true
			logging.Debugf("Marked interface %v for gateway deletion", ifName)
		} else {
			// Otherwise, determine if this interface now gets our default route.
			// According to 
			// https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ (4.1.2.1.9)
			// the list can be empty; if it is, we'll assume the CNI's config for the default gateway holds,
			// else we'll update the defaultgateway to the one specified.
			if delegate.GatewayRequest != nil && delegate.GatewayRequest[0] != nil {
				deletegateway = true
				adddefaultgateway = true
				logging.Debugf("Detected gateway override on interface %v to %v", ifName, delegate.GatewayRequest)
			}
		}

		if deletegateway {
			tmpResult, err = netutils.DeleteDefaultGW(args, ifName, &tmpResult)
			if err != nil {
				return nil, cmdErr(k8sArgs, "error deleting default gateway: %v", err)
			}
		}

		// Here we'll set the default gateway
		if adddefaultgateway {
			tmpResult, err = netutils.SetDefaultGW(args, ifName, delegate.GatewayRequest, &tmpResult)
			if err != nil {
				return nil, cmdErr(k8sArgs, "error setting default gateway: %v", err)
			}
		}

		// Master plugin result is always used if present
		if delegate.MasterPlugin || result == nil {
			result = tmpResult
		}

		// Read devInfo from CNIDeviceInfoFile if it exists so
		// it can be copied to the NetworkStatus.
		devinfo, err := getDelegateDeviceInfo(delegate, rt)
		if err != nil {
			// Even if the filename is set, file may not be present. Ignore error,
			// but log and in the future may need to filter on specific errors.
			logging.Debugf("cmdAdd: getDelegateDeviceInfo returned an error - err=%v", err)
		}

		// create the network status, only in case Multus as kubeconfig
		if n.Kubeconfig != "" && kc != nil {
			if !types.CheckSystemNamespaces(string(k8sArgs.K8S_POD_NAME), n.SystemNamespaces) {
				delegateNetStatus, err := nadutils.CreateNetworkStatus(tmpResult, delegate.Name, delegate.MasterPlugin, devinfo)
				if err != nil {
					return nil, cmdErr(k8sArgs, "error setting network status: %v", err)
				}

				netStatus = append(netStatus, *delegateNetStatus)
			}
		} else if devinfo != nil {
			// Warn that devinfo exists but could not add it to downwards API
			logging.Errorf("devinfo available, but no kubeConfig so NetworkStatus not modified.")
		}
	}

	// set the network status annotation in apiserver, only in case Multus as kubeconfig
	if n.Kubeconfig != "" && kc != nil {
		if !types.CheckSystemNamespaces(string(k8sArgs.K8S_POD_NAME), n.SystemNamespaces) {
			err = k8s.SetNetworkStatus(kubeClient, k8sArgs, netStatus, n)
			if err != nil {
				if strings.Contains(err.Error(), "failed to query the pod") {
					return nil, cmdErr(k8sArgs, "error setting the networks status, pod was already deleted: %v", err)
				}
				return nil, cmdErr(k8sArgs, "error setting the networks status: %v", err)
			}
		}
	}

	return result, nil
}

func CmdCheck(args *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) error {
	in, err := types.LoadNetConf(args.StdinData)
	logging.Debugf("CmdCheck: %v, %v, %v", args, exec, kubeClient)
	if err != nil {
		return err
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return cmdErr(nil, "error getting k8s args: %v", err)
	}

	for idx, delegate := range in.Delegates {
		ifName := getIfname(delegate, args.IfName, idx)

		rt, _ := types.CreateCNIRuntimeConf(args, k8sArgs, ifName, in.RuntimeConfig, delegate)
		err = delegateCheck(exec, ifName, delegate, rt, in.BinDir)
		if err != nil {
			return err
		}
	}

	return nil
}

func CmdDel(args *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) error {
	in, err := types.LoadNetConf(args.StdinData)
	logging.Debugf("CmdDel: %v, %v, %v", args, exec, kubeClient)
	if err != nil {
		return err
	}

	netnsfound := true
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		// if NetNs is passed down by the Cloud Orchestration Engine, or if it called multiple times
		// so don't return an error if the device is already removed.
		// https://github.com/kubernetes/kubernetes/issues/43014#issuecomment-287164444
		_, ok := err.(ns.NSPathNotExistErr)
		if ok {
			netnsfound = false
			logging.Debugf("CmdDel: WARNING netns may not exist, netns: %s, err: %s", args.Netns, err)
		} else {
			return cmdErr(nil, "failed to open netns %q: %v", netns, err)
		}
	}

	if netns != nil {
		defer netns.Close()
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return cmdErr(nil, "error getting k8s args: %v", err)
	}

	if in.ReadinessIndicatorFile != "" {
		err := wait.PollImmediate(pollDuration, pollTimeout, func() (bool, error) {
			_, err := os.Stat(in.ReadinessIndicatorFile)
			return err == nil, nil
		})
		if err != nil {
			return cmdErr(k8sArgs, "PollImmediate error waiting for ReadinessIndicatorFile (on del): %v", err)
		}
	}

	kubeClient, err = k8s.GetK8sClient(in.Kubeconfig, kubeClient)
	if err != nil {
		return cmdErr(nil, "error getting k8s client: %v", err)
	}

	pod := (*v1.Pod)(nil)
	if kubeClient != nil {
		pod, err = kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		if err != nil {
			var waitErr error
			// in case of ServiceUnavailable, retry 10 times with 0.5 sec interval
			if errors.IsServiceUnavailable(err) {
				pollDuration := 500 * time.Millisecond
				pollTimeout := 5 * time.Second
				waitErr = wait.PollImmediate(pollDuration, pollTimeout, func() (bool, error) {
					pod, err = kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
					return pod != nil, err
				})
				// retry failed, then return error with retry out
				if waitErr != nil {
					return cmdErr(k8sArgs, "error getting pod by service unavailable: %v", err)
				}
			} else if errors.IsNotFound(err) {
				// If not found, proceed to remove interface with cache
				pod = nil
			} else {
				// Other case, return error
				return cmdErr(k8sArgs, "error getting pod: %v", err)
			}
		}
	}

	// Read the cache to get delegates json for the pod
	netconfBytes, path, err := consumeScratchNetConf(args.ContainerID, in.CNIDir)
	if err != nil {
		// Fetch delegates again if cache is not exist and pod info can be read
		if os.IsNotExist(err) && pod != nil {
			if in.ClusterNetwork != "" {
				_, err = k8s.GetDefaultNetworks(pod, in, kubeClient, nil)
				if err != nil {
					return cmdErr(k8sArgs, "failed to get clusterNetwork/defaultNetworks: %v", err)
				}
				// First delegate is always the master plugin
				in.Delegates[0].MasterPlugin = true
			}

			// Get pod annotation and so on
			_, _, err := k8s.TryLoadPodDelegates(pod, in, kubeClient, nil)
			if err != nil {
				if len(in.Delegates) == 0 {
					// No delegate available so send error
					return cmdErr(k8sArgs, "failed to get delegates: %v", err)
				}
				// Get clusterNetwork before, so continue to delete
				logging.Errorf("Multus: failed to get delegates: %v, but continue to delete clusterNetwork", err)
			}
		} else {
			// The options to continue with a delete have been exhausted (cachefile + API query didn't work)
			// We cannot exit with an error as this may cause a sandbox to never get deleted.
			logging.Errorf("Multus: failed to get the cached delegates file: %v, cannot properly delete", err)
			return nil
		}
	} else {
		defer os.Remove(path)
		if err := json.Unmarshal(netconfBytes, &in.Delegates); err != nil {
			return cmdErr(k8sArgs, "failed to load netconf: %v", err)
		}
		// check plugins field and enable ConfListPlugin if there is
		for _, v := range in.Delegates {
			if len(v.ConfList.Plugins) != 0 {
				v.ConfListPlugin = true
			}
		}
		// First delegate is always the master plugin
		in.Delegates[0].MasterPlugin = true
	}

	// set CNIVersion in delegate CNI config if there is no CNIVersion and multus conf have CNIVersion.
	for _, v := range in.Delegates {
		if v.ConfListPlugin == true && v.ConfList.CNIVersion == "" && in.CNIVersion != "" {
			v.ConfList.CNIVersion = in.CNIVersion
			v.Bytes, err = json.Marshal(v.ConfList)
		}
	}

	// unset the network status annotation in apiserver, only in case Multus as kubeconfig
	if in.Kubeconfig != "" {
		if netnsfound {
			if !types.CheckSystemNamespaces(string(k8sArgs.K8S_POD_NAMESPACE), in.SystemNamespaces) {
				err := k8s.SetNetworkStatus(kubeClient, k8sArgs, nil, in)
				if err != nil {
					// error happen but continue to delete
					logging.Errorf("Multus: error unsetting the networks status: %v", err)
				}
			}
		} else {
			logging.Debugf("WARNING: Unset SetNetworkStatus skipped due to netns not found.")
		}
	}

	return delPlugins(exec, pod, args, k8sArgs, in.Delegates, len(in.Delegates)-1, in.RuntimeConfig, in.BinDir)
}

func main() {
	// Init command line flags to clear vendored packages' one, especially in init()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// add version flag
	versionOpt := false
	flag.BoolVar(&versionOpt, "version", false, "Show application version")
	flag.BoolVar(&versionOpt, "v", false, "Show application version")
	flag.Parse()
	if versionOpt == true {
		fmt.Printf("%s\n", PrintVersionString())
		return
	}

	skel.PluginMain(
		func(args *skel.CmdArgs) error {
			result, err := CmdAdd(args, nil, nil)
			if err != nil {
				return err
			}
			return result.Print()
		},
		func(args *skel.CmdArgs) error {
			return CmdCheck(args, nil, nil)
		},
		func(args *skel.CmdArgs) error { return CmdDel(args, nil, nil) },
		cniversion.All, "meta-plugin that delegates to other CNI plugins")
}
