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
	"github.com/containernetworking/plugins/pkg/ns"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	"github.com/vishvananda/netlink"
	k8s "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/netutils"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8snet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	shortPollDuration = 250 * time.Millisecond
	shortPollTimeout  = 2500 * time.Millisecond
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

//PrintVersionString ...
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

func confAdd(rt *libcni.RuntimeConf, rawNetconf []byte, multusNetconf *types.NetConf, exec invoke.Exec) (cnitypes.Result, error) {
	logging.Debugf("confAdd: %v, %s", rt, string(rawNetconf))
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{multusNetconf.BinDir}, binDirs...)
	cniNet := libcni.NewCNIConfigWithCacheDir(binDirs, multusNetconf.CNIDir, exec)

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

func confCheck(rt *libcni.RuntimeConf, rawNetconf []byte, multusNetconf *types.NetConf, exec invoke.Exec) error {
	logging.Debugf("confCheck: %v, %s", rt, string(rawNetconf))

	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{multusNetconf.BinDir}, binDirs...)
	cniNet := libcni.NewCNIConfigWithCacheDir(binDirs, multusNetconf.CNIDir, exec)

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

func confDel(rt *libcni.RuntimeConf, rawNetconf []byte, multusNetconf *types.NetConf, exec invoke.Exec) error {
	logging.Debugf("conflistDel: %v, %s", rt, string(rawNetconf))
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{multusNetconf.BinDir}, binDirs...)
	cniNet := libcni.NewCNIConfigWithCacheDir(binDirs, multusNetconf.CNIDir, exec)

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

func conflistAdd(rt *libcni.RuntimeConf, rawnetconflist []byte, multusNetconf *types.NetConf, exec invoke.Exec) (cnitypes.Result, error) {
	logging.Debugf("conflistAdd: %v, %s", rt, string(rawnetconflist))
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{multusNetconf.BinDir}, binDirs...)
	cniNet := libcni.NewCNIConfigWithCacheDir(binDirs, multusNetconf.CNIDir, exec)

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

func conflistCheck(rt *libcni.RuntimeConf, rawnetconflist []byte, multusNetconf *types.NetConf, exec invoke.Exec) error {
	logging.Debugf("conflistCheck: %v, %s", rt, string(rawnetconflist))

	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{multusNetconf.BinDir}, binDirs...)
	cniNet := libcni.NewCNIConfigWithCacheDir(binDirs, multusNetconf.CNIDir, exec)

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

func conflistDel(rt *libcni.RuntimeConf, rawnetconflist []byte, multusNetconf *types.NetConf, exec invoke.Exec) error {
	logging.Debugf("conflistDel: %v, %s", rt, string(rawnetconflist))
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go
	binDirs := filepath.SplitList(os.Getenv("CNI_PATH"))
	binDirs = append([]string{multusNetconf.BinDir}, binDirs...)
	cniNet := libcni.NewCNIConfigWithCacheDir(binDirs, multusNetconf.CNIDir, exec)

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

func delegateAdd(exec invoke.Exec, kubeClient *k8s.ClientInfo, pod *v1.Pod, netns string, ifName string, delegate *types.DelegateNetConf, rt *libcni.RuntimeConf, multusNetconf *types.NetConf, cniArgs string) (cnitypes.Result, error) {
	logging.Debugf("delegateAdd: %v, %s, %v, %v", exec, ifName, delegate, rt)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return nil, logging.Errorf("delegateAdd: error setting environment variable CNI_IFNAME")
	}

	if err := validateIfName(netns, ifName); err != nil {
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
		result, err = conflistAdd(rt, delegate.Bytes, multusNetconf, exec)
		if err != nil {
			return nil, err
		}
	} else {
		result, err = confAdd(rt, delegate.Bytes, multusNetconf, exec)
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
		// for further debug https://github.com/k8snetworkplumbingwg/multus-cni/issues/481
		logging.Errorf("delegateAdd: pod nil pointer: namespace: %s, name: %s, container id: %s, pod: %v", rt.Args[1][1], rt.Args[2][1], rt.Args[3][1], pod)
	}

	return result, nil
}

func delegateCheck(exec invoke.Exec, ifName string, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, multusNetconf *types.NetConf) error {
	logging.Debugf("delegateCheck: %v, %s, %v, %v", exec, ifName, delegateConf, rt)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return logging.Errorf("delegateCheck: error setting environment variable CNI_IFNAME")
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
		err = conflistCheck(rt, delegateConf.Bytes, multusNetconf, exec)
		if err != nil {
			return logging.Errorf("delegateCheck: error invoking ConflistCheck - %q: %v", delegateConf.ConfList.Name, err)
		}
	} else {
		err = confCheck(rt, delegateConf.Bytes, multusNetconf, exec)
		if err != nil {
			return logging.Errorf("delegateCheck: error invoking DelegateCheck - %q: %v", delegateConf.Conf.Type, err)
		}
	}

	return err
}

func delegateDel(exec invoke.Exec, pod *v1.Pod, ifName string, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, multusNetconf *types.NetConf) error {
	logging.Debugf("delegateDel: %v, %v, %s, %v, %v", exec, pod, ifName, delegateConf, rt)
	if os.Setenv("CNI_IFNAME", ifName) != nil {
		return logging.Errorf("delegateDel: error setting environment variable CNI_IFNAME")
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
		err = conflistDel(rt, delegateConf.Bytes, multusNetconf, exec)
		if err != nil {
			return logging.Errorf("delegateDel: error invoking ConflistDel - %q: %v", delegateConf.ConfList.Name, err)
		}
	} else {
		err = confDel(rt, delegateConf.Bytes, multusNetconf, exec)
		if err != nil {
			return logging.Errorf("delegateDel: error invoking DelegateDel - %q: %v", delegateConf.Conf.Type, err)
		}
	}

	return err
}

// Cmd holds the required CNI spec data required to configure networking in
// the pod's sandbox
type Cmd struct {
	ContainerID   string
	InterfaceName string
	NetnsPath     string
	PodName       string
	PodNamespace  string
	PodUID        string
	SandboxID     string
}

// delPlugins deletes plugins in reverse order from lastdIdx
// Uses netRt as base RuntimeConf (coming from NetConf) but merges it
// with each of the delegates' configuration
func (mc *Cmd) delPlugins(exec invoke.Exec, pod *v1.Pod, delegates []*types.DelegateNetConf, lastIdx int, netRt *types.RuntimeConfig, multusNetconf *types.NetConf) error {
	logging.Debugf("delPlugins: %v, %v, %v, %d, %v", exec, pod, delegates, lastIdx, netRt)
	if os.Setenv("CNI_COMMAND", "DEL") != nil {
		return logging.Errorf("delPlugins: error setting environment variable CNI_COMMAND to a value of DEL")
	}

	var errorstrings []string
	for idx := lastIdx; idx >= 0; idx-- {
		ifName := getIfname(delegates[idx], mc.InterfaceName, idx)

		rt, cniDeviceInfoPath := types.NewCNIRuntimeConf(
			mc.ContainerID, mc.SandboxID, mc.PodName, mc.PodNamespace, mc.PodUID, mc.NetnsPath, ifName, netRt, delegates[idx])

		// Attempt to delete all but do not error out, instead, collect all errors.
		if err := delegateDel(exec, pod, ifName, delegates[idx], rt, multusNetconf); err != nil {
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
		prefix += fmt.Sprintf("[%s/%s/%s]: ", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME, k8sArgs.K8S_POD_UID)
	}
	return logging.Errorf(prefix+format, args...)
}

func cmdPluginErr(namespace string, podName string, podUID string, confName string, format string, args ...interface{}) error {
	msg := fmt.Sprintf("[%s/%s/%s:%s]: ", namespace, podName, podUID, confName)
	return logging.Errorf(msg+format, args...)
}

func isCriticalRequestRetriable(err error) bool {
	logging.Debugf("isCriticalRequestRetriable: %v", err)
	errorTypesAllowingRetry := []func(error) bool{
		errors.IsServiceUnavailable, errors.IsInternalError, k8snet.IsConnectionReset, k8snet.IsConnectionRefused}
	for _, f := range errorTypesAllowingRetry {
		if f(err) {
			return true
		}
	}
	return false
}

// GetPod gets the data from the pod identified by `namespace/name`. If the
// returned object's UID does not match the expected `uid` an error is returned.
// When the `warnOnly` flag is provided, warnings are logged instead of returned.
func GetPod(kubeClient *k8s.ClientInfo, podNamespace string, podName string, podUID string, warnOnly bool) (*v1.Pod, error) {
	pod, err := kubeClient.GetPod(podNamespace, podName)
	if err != nil {
		// in case of a retriable error, retry 10 times with 0.25 sec interval
		if isCriticalRequestRetriable(err) {
			waitErr := wait.PollImmediate(shortPollDuration, shortPollTimeout, func() (bool, error) {
				pod, err = kubeClient.GetPod(podNamespace, podName)
				return pod != nil, err
			})
			// retry failed, then return error with retry out
			if waitErr != nil {
				return nil, fmt.Errorf("error waiting for pod: %v", err)
			}
		} else if warnOnly && errors.IsNotFound(err) {
			// If not found, proceed to remove interface with cache
			return nil, nil
		} else {
			// Other case, return error
			return nil, fmt.Errorf("error getting pod: %v", err)
		}
	}

	// In case of static pod, UID through kube api is different because of mirror pod, hence it is expected.
	if podUID != "" && string(pod.UID) != podUID && !k8s.IsStaticPod(pod) {
		msg := fmt.Sprintf("expected pod UID %q but got %q from Kube API", podUID, pod.UID)
		if warnOnly {
			// On CNI DEL we just operate on the cache when these mismatch, we don't error out.
			// For example: stateful sets namespace/name can remain the same while podUID changes.
			logging.Verbosef("warning: %s", msg)
			return nil, nil
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return pod, nil
}

func getPod(kubeClient *k8s.ClientInfo, k8sArgs *types.K8sArgs, warnOnly bool) (*v1.Pod, error) {
	if kubeClient == nil {
		return nil, nil
	}

	podNamespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	podUID := string(k8sArgs.K8S_POD_UID)

	pod, err := GetPod(kubeClient, podNamespace, podName, podUID, warnOnly)
	if err != nil {
		return nil, cmdErr(k8sArgs, "%v", err)
	}

	return pod, nil
}

//CmdAdd ...
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

	pod, err := getPod(kubeClient, k8sArgs, false)
	if err != nil {
		return nil, err
	}

	netnsPath := args.Netns
	if netns, found := os.LookupEnv("CNI_NETNS"); found {
		netnsPath = netns
	}

	ifName := args.IfName
	if interfaceName, found := os.LookupEnv("CNI_IFNAME"); found {
		ifName = interfaceName
	}
	result, err := NewMultusCmd(
		args.ContainerID,
		string(k8sArgs.K8S_POD_INFRA_CONTAINER_ID),
		ifName,
		netnsPath,
		string(k8sArgs.K8S_POD_NAME),
		string(k8sArgs.K8S_POD_NAMESPACE),
		string(k8sArgs.K8S_POD_UID)).Add(n, pod, exec, kubeClient)
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}
	return result, nil
}

//CmdCheck ...
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

	return NewMultusCmd(
		args.ContainerID,
		string(k8sArgs.K8S_POD_INFRA_CONTAINER_ID),
		args.IfName,
		args.Netns,
		string(k8sArgs.K8S_POD_NAME),
		string(k8sArgs.K8S_POD_NAMESPACE),
		string(k8sArgs.K8S_POD_UID)).Check(in, exec)
}

//CmdDel ...
func CmdDel(args *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) error {
	in, err := types.LoadNetConf(args.StdinData)
	logging.Debugf("CmdDel: %v, %v, %v", args, exec, kubeClient)
	if err != nil {
		return err
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return cmdErr(nil, "error getting k8s args: %v", err)
	}

	kubeClient, err = k8s.GetK8sClient(in.Kubeconfig, kubeClient)
	if err != nil {
		return cmdErr(nil, "error getting k8s client: %v", err)
	}

	pod, err := getPod(kubeClient, k8sArgs, true)
	if err != nil {
		return err
	}

	return NewMultusCmd(
		args.ContainerID,
		string(k8sArgs.K8S_POD_INFRA_CONTAINER_ID),
		args.IfName,
		args.Netns,
		string(k8sArgs.K8S_POD_NAME),
		string(k8sArgs.K8S_POD_NAMESPACE),
		string(k8sArgs.K8S_POD_UID),
	).Delete(in, pod, exec, kubeClient)
}

// NewMultusCmd return a new ADD / CHECK / DEL command
func NewMultusCmd(containerID, sandboxID, ifaceName, netnsPath, podName, podNamespace, podUID string) *Cmd {
	return &Cmd{
		ContainerID:   containerID,
		SandboxID:     sandboxID,
		InterfaceName: ifaceName,
		NetnsPath:     netnsPath,
		PodName:       podName,
		PodNamespace:  podNamespace,
		PodUID:        podUID,
	}
}

// Add is the handler of the CNI ADD command
func (mc *Cmd) Add(cniConf *types.NetConf, pod *v1.Pod, exec invoke.Exec, kubeclient *k8s.ClientInfo) (cnitypes.Result, error) {
	var err error
	if cniConf.ReadinessIndicatorFile != "" {
		err := wait.PollImmediate(pollDuration, pollTimeout, func() (bool, error) {
			_, err := os.Stat(cniConf.ReadinessIndicatorFile)
			return err == nil, nil
		})
		if err != nil {
			return nil, cmdErr(mc.printableK8sArgs(), "have you checked that your default network is ready? still waiting for readinessindicatorfile @ %v. pollimmediate error: %v", cniConf.ReadinessIndicatorFile, err)
		}
	}

	// resourceMap holds Pod device allocation information; only initizized if CRD contains 'resourceName' annotation.
	// This will only be initialized once and all delegate objects can reference this to look up device info.
	var resourceMap map[string]*types.ResourceInfo

	if cniConf.ClusterNetwork != "" {
		resourceMap, err = k8s.GetDefaultNetworks(pod, cniConf, kubeclient, resourceMap)
		if err != nil {
			return nil, cmdErr(mc.printableK8sArgs(), "failed to get clusterNetwork/defaultNetworks: %v", err)
		}
		// First delegate is always the master plugin
		cniConf.Delegates[0].MasterPlugin = true
	}

	_, kc, err := k8s.TryLoadPodDelegates(pod, cniConf, kubeclient, resourceMap)
	if err != nil {
		return nil, cmdErr(mc.printableK8sArgs(), "error loading k8s delegates k8s args: %v", err)
	}

	// cache the multus config
	if err := saveDelegates(mc.ContainerID, cniConf.CNIDir, cniConf.Delegates); err != nil {
		return nil, cmdErr(mc.printableK8sArgs(), "error saving the delegates: %v", err)
	}

	var result, tmpResult cnitypes.Result
	var netStatus []nettypes.NetworkStatus
	cniArgs := os.Getenv("CNI_ARGS")

	for idx, delegate := range cniConf.Delegates {
		ifName := getIfname(delegate, mc.InterfaceName, idx)
		rt, cniDeviceInfoPath := types.NewCNIRuntimeConf(
			mc.ContainerID, mc.SandboxID, mc.PodName, mc.PodNamespace, mc.PodUID, mc.NetnsPath, ifName, cniConf.RuntimeConfig, delegate)
		if cniDeviceInfoPath != "" && delegate.ResourceName != "" && delegate.DeviceID != "" {
			err = nadutils.CopyDeviceInfoForCNIFromDP(cniDeviceInfoPath, delegate.ResourceName, delegate.DeviceID)
			// Even if the filename is set, file may not be present. Ignore error,
			// but log and in the future may need to filter on specific errors.
			if err != nil {
				logging.Debugf("cmdAdd: CopyDeviceInfoForCNIFromDP returned an error - err=%v", err)
			}
		}

		netName := ""
		tmpResult, err = delegateAdd(exec, kubeclient, pod, mc.NetnsPath, ifName, delegate, rt, cniConf, cniArgs)
		if err != nil {
			// If the add failed, tear down all networks we already added
			netName = delegate.Conf.Name
			if netName == "" {
				netName = delegate.ConfList.Name
			}
			// Ignore errors; DEL must be idempotent anyway
			_ = mc.delPlugins(exec, nil, cniConf.Delegates, idx, cniConf.RuntimeConfig, cniConf)
			return nil, cmdPluginErr(mc.PodNamespace, mc.PodName, mc.PodUID, netName, "error adding container to network %q: %v", netName, err)
		}

		// Remove gateway from routing table if the gateway is not used
		deleteV4gateway := false
		deleteV6gateway := false
		adddefaultgateway := false
		if delegate.IsFilterV4Gateway {
			deleteV4gateway = true
			logging.Debugf("Marked interface %v for v4 gateway deletion", ifName)
		} else {
			// Otherwise, determine if this interface now gets our default route.
			// According to
			// https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ (4.1.2.1.9)
			// the list can be empty; if it is, we'll assume the CNI's config for the default gateway holds,
			// else we'll update the defaultgateway to the one specified.
			if delegate.GatewayRequest != nil && delegate.GatewayRequest[0] != nil {
				deleteV4gateway = true
				adddefaultgateway = true
				logging.Debugf("Detected gateway override on interface %v to %v", ifName, delegate.GatewayRequest)
			}
		}

		if delegate.IsFilterV6Gateway {
			deleteV6gateway = true
			logging.Debugf("Marked interface %v for v6 gateway deletion", ifName)
		} else {
			// Otherwise, determine if this interface now gets our default route.
			// According to
			// https://docs.google.com/document/d/1Ny03h6IDVy_e_vmElOqR7UdTPAG_RNydhVE1Kx54kFQ (4.1.2.1.9)
			// the list can be empty; if it is, we'll assume the CNI's config for the default gateway holds,
			// else we'll update the defaultgateway to the one specified.
			if delegate.GatewayRequest != nil && delegate.GatewayRequest[0] != nil {
				deleteV6gateway = true
				adddefaultgateway = true
				logging.Debugf("Detected gateway override on interface %v to %v", ifName, delegate.GatewayRequest)
			}
		}

		// Remove namespace from delegate.Name for Add/Del CNI cache
		nameSlice := strings.Split(delegate.Name, "/")
		netName = nameSlice[len(nameSlice)-1]

		// Remove gateway if `default-route` network selection is specified
		if deleteV4gateway || deleteV6gateway {
			err = netutils.DeleteDefaultGW(mc.NetnsPath, ifName)
			if err != nil {
				return nil, cmdErr(mc.printableK8sArgs(), "error deleting default gateway: %v", err)
			}
			err = netutils.DeleteDefaultGWCache(cniConf.CNIDir, rt, netName, ifName, deleteV4gateway, deleteV6gateway)
			if err != nil {
				return nil, cmdErr(mc.printableK8sArgs(), "error deleting default gateway in cache: %v", err)
			}
		}

		// Here we'll set the default gateway which specified in `default-route` network selection
		if adddefaultgateway {
			err = netutils.SetDefaultGW(mc.NetnsPath, ifName, delegate.GatewayRequest)
			if err != nil {
				return nil, cmdErr(mc.printableK8sArgs(), "error setting default gateway: %v", err)
			}
			err = netutils.AddDefaultGWCache(cniConf.CNIDir, rt, netName, ifName, delegate.GatewayRequest)
			if err != nil {
				return nil, cmdErr(mc.printableK8sArgs(), "error setting default gateway in cache: %v", err)
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
		if cniConf.Kubeconfig != "" && kc != nil {
			if !types.CheckSystemNamespaces(mc.PodName, cniConf.SystemNamespaces) {
				delegateNetStatus, err := nadutils.CreateNetworkStatus(tmpResult, delegate.Name, delegate.MasterPlugin, devinfo)
				if err != nil {
					return nil, cmdErr(mc.printableK8sArgs(), "error setting network status: %v", err)
				}

				netStatus = append(netStatus, *delegateNetStatus)
			}
		} else if devinfo != nil {
			// Warn that devinfo exists but could not add it to downwards API
			logging.Errorf("devinfo available, but no kubeConfig so NetworkStatus not modified.")
		}
	}

	// set the network status annotation in apiserver, only in case Multus as kubeconfig
	if cniConf.Kubeconfig != "" && kc != nil {
		if !types.CheckSystemNamespaces(mc.PodName, cniConf.SystemNamespaces) {
			if err := k8s.SetPodNetworkStatusAnnotation(kubeclient, mc.PodName, mc.PodNamespace, mc.PodUID, netStatus, cniConf); err != nil {
				if strings.Contains(err.Error(), "failed to query the GetPod") {
					return nil, cmdErr(mc.printableK8sArgs(), "error setting the networks status, GetPod was already deleted: %v", err)
				}
				return nil, cmdErr(mc.printableK8sArgs(), "error setting the networks status: %v", err)
			}
		}
	}

	return result, nil
}

func (mc *Cmd) printableK8sArgs() *types.K8sArgs {
	return &types.K8sArgs{
		K8S_POD_NAME:      cnitypes.UnmarshallableString(mc.PodName),
		K8S_POD_NAMESPACE: cnitypes.UnmarshallableString(mc.PodNamespace),
		K8S_POD_UID:       cnitypes.UnmarshallableString(mc.PodUID),
	}
}

// Check is the handler of the CNI CHECK command
func (mc *Cmd) Check(cniConf *types.NetConf, exec invoke.Exec) error {
	for idx, delegate := range cniConf.Delegates {
		ifName := getIfname(delegate, mc.InterfaceName, idx)

		rt, _ := types.NewCNIRuntimeConf(
			mc.ContainerID,
			mc.SandboxID,
			mc.PodName,
			mc.PodNamespace,
			mc.PodUID,
			mc.NetnsPath,
			ifName,
			cniConf.RuntimeConfig,
			delegate)
		if err := delegateCheck(exec, ifName, delegate, rt, cniConf); err != nil {
			return err
		}
	}
	return nil
}

// Delete is the handler of the CNI DEL command
func (mc *Cmd) Delete(cniConf *types.NetConf, pod *v1.Pod, exec invoke.Exec, kubeclient *k8s.ClientInfo) error {
	var err error
	if cniConf.ReadinessIndicatorFile != "" {
		err := wait.PollImmediate(pollDuration, pollTimeout, func() (bool, error) {
			_, err := os.Stat(cniConf.ReadinessIndicatorFile)
			return err == nil, nil
		})
		if err != nil {
			return cmdErr(mc.printableK8sArgs(), "have you checked that your default network is ready? still waiting for readinessindicatorfile @ %v. pollimmediate error: %v", cniConf.ReadinessIndicatorFile, err)
		}
	}

	netnsfound := true
	netns, err := ns.GetNS(mc.NetnsPath)
	if err != nil {
		// if NetNs is passed down by the Cloud Orchestration Engine, or if it called multiple times
		// so don't return an error if the device is already removed.
		// https://github.com/kubernetes/kubernetes/issues/43014#issuecomment-287164444
		_, ok := err.(ns.NSPathNotExistErr)
		if ok {
			netnsfound = false
			logging.Debugf("CmdDel: WARNING netns may not exist, netns: %s, err: %s", mc.NetnsPath, err)
		} else {
			return cmdErr(mc.printableK8sArgs(), "failed to open netns %q: %v", netns, err)
		}
	}

	if netns != nil {
		defer netns.Close()
	}

	// Read the cache to get delegates json for the GetPod
	netconfBytes, path, err := consumeScratchNetConf(mc.ContainerID, cniConf.CNIDir)
	if err != nil {
		// Fetch delegates again if cache is not exist and pod info can be read
		if os.IsNotExist(err) && pod != nil {
			if cniConf.ClusterNetwork != "" {
				_, err = k8s.GetDefaultNetworks(pod, cniConf, kubeclient, nil)
				if err != nil {
					return cmdErr(mc.printableK8sArgs(), "failed to get clusterNetwork/defaultNetworks: %v", err)
				}
				// First delegate is always the master plugin
				cniConf.Delegates[0].MasterPlugin = true
			}

			// Get GetPod annotation and so on
			_, _, err := k8s.TryLoadPodDelegates(pod, cniConf, kubeclient, nil)
			if err != nil {
				if len(cniConf.Delegates) == 0 {
					// No delegate available so send error
					return cmdErr(mc.printableK8sArgs(), "failed to get delegates: %v", err)
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
		if err := json.Unmarshal(netconfBytes, &cniConf.Delegates); err != nil {
			return cmdErr(mc.printableK8sArgs(), "failed to load netconf: %v", err)
		}
		// check plugins field and enable ConfListPlugin if there is
		for _, v := range cniConf.Delegates {
			if len(v.ConfList.Plugins) != 0 {
				v.ConfListPlugin = true
			}
		}
		// First delegate is always the master plugin
		cniConf.Delegates[0].MasterPlugin = true
	}

	// set CNIVersion in delegate CNI config if there is no CNIVersion and multus conf have CNIVersion.
	for _, v := range cniConf.Delegates {
		if v.ConfListPlugin == true && v.ConfList.CNIVersion == "" && cniConf.CNIVersion != "" {
			v.ConfList.CNIVersion = cniConf.CNIVersion
			v.Bytes, err = json.Marshal(v.ConfList)
			if err != nil {
				// error happen but continue to delete
				logging.Errorf("Multus: failed to marshal delegate %q config: %v", v.Name, err)
			}
		}
	}

	// unset the network status annotation in apiserver, only in case Multus as kubeconfig
	if cniConf.Kubeconfig != "" {
		if netnsfound {
			if !types.CheckSystemNamespaces(mc.PodNamespace, cniConf.SystemNamespaces) {
				err := k8s.SetPodNetworkStatusAnnotation(kubeclient, mc.PodName, mc.PodNamespace, mc.PodUID, nil, cniConf)
				if err != nil {
					// error happen but continue to delete
					logging.Errorf("Multus: error unsetting the networks status: %v", err)
				}
			}
		} else {
			logging.Debugf("WARNING: Unset SetNetworkStatus skipped due to netns not found.")
		}
	}

	return mc.delPlugins(exec, pod, cniConf.Delegates, len(cniConf.Delegates)-1, cniConf.RuntimeConfig, cniConf)
}
