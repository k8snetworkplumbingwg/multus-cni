// Copyright (c) 2016 Intel Corporation
// Copyright (c) 2021 Multus Authors
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
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8snet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"

	k8s "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/netutils"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

const (
	shortPollDuration    = 250 * time.Millisecond
	informerPollDuration = 50 * time.Millisecond
	shortPollTimeout     = 2500 * time.Millisecond
)

var (
	version       = "master@git"
	commit        = "unknown commit"
	date          = "unknown date"
	gitTreeState  = ""
	releaseStatus = ""
)

// PrintVersionString ...
func PrintVersionString() string {
	return fmt.Sprintf("version:%s(%s%s), commit:%s, date:%s", version, gitTreeState, releaseStatus, commit, date)
}

func saveScratchNetConf(containerID, dataDir string, netconf []byte) error {
	logging.Debugf("saveScratchNetConf: %s, %s, %s", containerID, dataDir, string(netconf))
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return logging.Errorf("saveScratchNetConf: failed to create the multus data directory(%q): %v", dataDir, err)
	}

	path := filepath.Join(dataDir, containerID)

	err := os.WriteFile(path, netconf, 0600)
	if err != nil {
		return logging.Errorf("saveScratchNetConf: failed to write container data in the path(%q): %v", path, err)
	}

	return err
}

func consumeScratchNetConf(containerID, dataDir string) ([]byte, string, error) {
	logging.Debugf("consumeScratchNetConf: %s, %s", containerID, dataDir)
	path := filepath.Join(dataDir, containerID)

	b, err := os.ReadFile(path)
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

func getDelegateDeviceInfo(_ *types.DelegateNetConf, runtimeConf *libcni.RuntimeConf) (*nettypes.DeviceInfo, error) {
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
		return logging.Errorf("error in getting result from CheckNetwork: %v", err)
	}

	return err
}

func confDel(rt *libcni.RuntimeConf, rawNetconf []byte, multusNetconf *types.NetConf, exec invoke.Exec) error {
	logging.Debugf("confDel: %v, %s", rt, string(rawNetconf))
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

// DelegateAdd ...
func DelegateAdd(exec invoke.Exec, kubeClient *k8s.ClientInfo, pod *v1.Pod, delegate *types.DelegateNetConf, rt *libcni.RuntimeConf, multusNetconf *types.NetConf) (cnitypes.Result, error) {
	logging.Debugf("DelegateAdd: %v, %v, %v", exec, delegate, rt)

	if err := validateIfName(rt.NetNS, rt.IfName); err != nil {
		return nil, logging.Errorf("DelegateAdd: cannot set %q interface name to %q: %v", delegate.Conf.Type, rt.IfName, err)
	}

	// Deprecated in ver 3.5.
	if delegate.MacRequest != "" || delegate.IPRequest != nil {
		if delegate.MacRequest != "" {
			// validate Mac address
			_, err := net.ParseMAC(delegate.MacRequest)
			if err != nil {
				return nil, logging.Errorf("DelegateAdd: failed to parse mac address %q", delegate.MacRequest)
			}

			logging.Debugf("DelegateAdd: set MAC address %q to %q", delegate.MacRequest, rt.IfName)
			rt.Args = append(rt.Args, [2]string{"MAC", delegate.MacRequest})
		}

		if delegate.IPRequest != nil {
			// validate IP address
			for _, ip := range delegate.IPRequest {
				if strings.Contains(ip, "/") {
					_, _, err := net.ParseCIDR(ip)
					if err != nil {
						return nil, logging.Errorf("DelegateAdd: failed to parse IP address %q", ip)
					}
				} else if net.ParseIP(ip) == nil {
					return nil, logging.Errorf("DelegateAdd: failed to parse IP address %q", ip)
				}
			}

			ips := strings.Join(delegate.IPRequest, ",")
			logging.Debugf("DelegateAdd: set IP address %q to %q", ips, rt.IfName)
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
	res, err := cni100.NewResultFromResult(result)
	if err != nil {
		logging.Errorf("DelegateAdd: error converting result: %v", err)
		return result, nil
	}
	for _, ip := range res.IPs {
		ips = append(ips, ip.Address.String())
	}

	if pod != nil {
		// check Interfaces and IPs because some CNI plugin just return empty result
		if res.Interfaces != nil || res.IPs != nil {
			// send kubernetes events
			if delegate.Name != "" {
				kubeClient.Eventf(pod, v1.EventTypeNormal, "AddedInterface", "Add %s %v from %s", rt.IfName, ips, delegate.Name)
			} else {
				kubeClient.Eventf(pod, v1.EventTypeNormal, "AddedInterface", "Add %s %v", rt.IfName, ips)
			}
		}
	} else {
		// for further debug https://github.com/k8snetworkplumbingwg/multus-cni/issues/481
		logging.Errorf("DelegateAdd: pod nil pointer: namespace: %s, name: %s, container id: %s, pod: %v", rt.Args[1][1], rt.Args[2][1], rt.Args[3][1], pod)
	}
	return result, nil
}

// DelegateCheck ...
func DelegateCheck(exec invoke.Exec, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, multusNetconf *types.NetConf) error {
	logging.Debugf("DelegateCheck: %v, %v, %v", exec, delegateConf, rt)

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
			return logging.Errorf("DelegateCheck: error invoking ConflistCheck - %q: %v", delegateConf.ConfList.Name, err)
		}
	} else {
		err = confCheck(rt, delegateConf.Bytes, multusNetconf, exec)
		if err != nil {
			return logging.Errorf("DelegateCheck: error invoking DelegateCheck - %q: %v", delegateConf.Conf.Type, err)
		}
	}

	return err
}

// DelegateDel ...
func DelegateDel(exec invoke.Exec, pod *v1.Pod, delegateConf *types.DelegateNetConf, rt *libcni.RuntimeConf, multusNetconf *types.NetConf) error {
	logging.Debugf("DelegateDel: %v, %v, %v, %v", exec, pod, delegateConf, rt)

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
			return logging.Errorf("DelegateDel: error invoking ConflistDel - %q: %v", delegateConf.ConfList.Name, err)
		}
	} else {
		err = confDel(rt, delegateConf.Bytes, multusNetconf, exec)
		if err != nil {
			return logging.Errorf("DelegateDel: error invoking DelegateDel - %q: %v", delegateConf.Conf.Type, err)
		}
	}

	return err
}

// delPlugins deletes plugins in reverse order from lastdIdx
// Uses netRt as base RuntimeConf (coming from NetConf) but merges it
// with each of the delegates' configuration
func delPlugins(exec invoke.Exec, pod *v1.Pod, args *skel.CmdArgs, k8sArgs *types.K8sArgs, delegates []*types.DelegateNetConf, lastIdx int, netRt *types.RuntimeConfig, multusNetconf *types.NetConf) error {
	logging.Debugf("delPlugins: %v, %v, %v, %v, %v, %d, %v", exec, pod, args, k8sArgs, delegates, lastIdx, netRt)

	var errorstrings []string
	for idx := lastIdx; idx >= 0; idx-- {
		ifName := getIfname(delegates[idx], args.IfName, idx)
		rt, cniDeviceInfoPath := types.CreateCNIRuntimeConf(args, k8sArgs, ifName, netRt, delegates[idx])
		// Attempt to delete all but do not error out, instead, collect all errors.
		if err := DelegateDel(exec, pod, delegates[idx], rt, multusNetconf); err != nil {
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

func cmdPluginErr(k8sArgs *types.K8sArgs, confName string, format string, args ...interface{}) error {
	msg := ""
	if k8sArgs != nil {
		msg += fmt.Sprintf("[%s/%s/%s:%s]: ", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME, k8sArgs.K8S_POD_UID, confName)
	}
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

// GetPod retrieves Kubernetes Pod object from given namespace/name in k8sArgs (i.e. cni args)
// GetPod also get pod UID, but it is not used to retrieve, but it is used for double check
func GetPod(kubeClient *k8s.ClientInfo, k8sArgs *types.K8sArgs, isDel bool) (*v1.Pod, error) {
	if kubeClient == nil {
		return nil, nil
	}

	podNamespace := string(k8sArgs.K8S_POD_NAMESPACE)
	podName := string(k8sArgs.K8S_POD_NAME)
	podUID := string(k8sArgs.K8S_POD_UID)

	// Keep track of how long getting the pod takes
	logging.Debugf("GetPod for [%s/%s] starting", podNamespace, podName)
	start := time.Now()
	defer func() {
		logging.Debugf("GetPod for [%s/%s] took %v", podNamespace, podName, time.Since(start))
	}()

	// Use a fairly long 0.25 sec interval so we don't hammer the apiserver
	pollDuration := shortPollDuration
	retryOnNotFound := func(error) bool {
		return false
	}

	if kubeClient.PodInformer != nil {
		logging.Debugf("GetPod for [%s/%s] will use informer cache", podNamespace, podName)
		// Use short retry intervals with the informer since it's a local cache
		pollDuration = informerPollDuration
		// Retry NotFound on ADD since the cache may be a bit behind the apiserver
		retryOnNotFound = func(e error) bool {
			return !isDel && errors.IsNotFound(e)
		}
	}

	var pod *v1.Pod
	if err := wait.PollImmediate(pollDuration, shortPollTimeout, func() (bool, error) {
		var getErr error
		// Use context with a short timeout so the call to API server doesn't take too long.
		ctx, cancel := context.WithTimeout(context.TODO(), pollDuration)
		defer cancel()
		pod, getErr = kubeClient.GetPodContext(ctx, podNamespace, podName)
		if isCriticalRequestRetriable(getErr) || retryOnNotFound(getErr) {
			return false, nil
		}
		return pod != nil, getErr
	}); err != nil {
		if isDel && errors.IsNotFound(err) {
			// On DEL pod may already be gone from apiserver/informer
			return nil, nil
		}
		// Try one more time to get the pod directly from the apiserver;
		// TODO: figure out why static pods don't show up via the informer
		// and always hit this case.
		ctx, cancel := context.WithTimeout(context.TODO(), pollDuration)
		defer cancel()
		pod, err = kubeClient.GetPodContext(ctx, podNamespace, podName)
		if err != nil {
			return nil, cmdErr(k8sArgs, "error waiting for pod: %v", err)
		}
	}

	// In case of static pod, UID through kube api is different because of mirror pod, hence it is expected.
	if podUID != "" && string(pod.UID) != podUID && !k8s.IsStaticPod(pod) {
		msg := fmt.Sprintf("expected pod UID %q but got %q from Kube API", podUID, pod.UID)
		if isDel {
			// On CNI DEL we just operate on the cache when these mismatch, we don't error out.
			// For example: stateful sets namespace/name can remain the same while podUID changes.
			logging.Verbosef("warning: %s", msg)
			return nil, nil
		}
		return nil, cmdErr(k8sArgs, msg)
	}

	return pod, nil
}

// CmdAdd ...
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
		if err := types.GetReadinessIndicatorFile(n.ReadinessIndicatorFile); err != nil {
			return nil, cmdErr(k8sArgs, "have you checked that your default network is ready? still waiting for readinessindicatorfile @ %v. pollimmediate error: %v", n.ReadinessIndicatorFile, err)
		}
	}

	pod, err := GetPod(kubeClient, k8sArgs, false)
	if err != nil {
		return nil, err
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
	for idx, delegate := range n.Delegates {
		ifName := getIfname(delegate, args.IfName, idx)
		rt, cniDeviceInfoPath := types.CreateCNIRuntimeConf(args, k8sArgs, ifName, n.RuntimeConfig, delegate)
		if cniDeviceInfoPath != "" && delegate.ResourceName != "" && delegate.DeviceID != "" {
			err = nadutils.CopyDeviceInfoForCNIFromDP(cniDeviceInfoPath, delegate.ResourceName, delegate.DeviceID)
			// Even if the filename is set, file may not be present. Ignore error,
			// but log and in the future may need to filter on specific errors.
			if err != nil {
				logging.Debugf("CmdAdd: CopyDeviceInfoForCNIFromDP returned an error - err=%v", err)
			}
		}

		// We collect the delegate netName for the cachefile name as well as following errors
		netName := delegate.Conf.Name
		if netName == "" {
			netName = delegate.ConfList.Name
		}
		tmpResult, err = DelegateAdd(exec, kubeClient, pod, delegate, rt, n)
		if err != nil {
			// If the add failed, tear down all networks we already added
			// Ignore errors; DEL must be idempotent anyway
			_ = delPlugins(exec, nil, args, k8sArgs, n.Delegates, idx, n.RuntimeConfig, n)
			return nil, cmdPluginErr(k8sArgs, netName, "error adding container to network %q: %v", netName, err)
		}

		// Master plugin result is always used if present
		if delegate.MasterPlugin || result == nil {
			result = tmpResult
		}

		res, err := cni100.NewResultFromResult(tmpResult)
		if err != nil {
			logging.Errorf("CmdAdd: failed to read result: %v, but proceed", err)
		}

		// check Interfaces and IPs because some CNI plugin does not create any interface
		// and just returns empty result
		if res != nil && (res.Interfaces != nil || res.IPs != nil) {
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
				if delegate.GatewayRequest != nil && len(*delegate.GatewayRequest) != 0 {
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
				if delegate.GatewayRequest != nil && len(*delegate.GatewayRequest) != 0 {
					deleteV6gateway = true
					adddefaultgateway = true
					logging.Debugf("Detected gateway override on interface %v to %v", ifName, delegate.GatewayRequest)
				}
			}

			// Remove gateway if `default-route` network selection is specified
			if deleteV4gateway || deleteV6gateway {
				err = netutils.DeleteDefaultGW(args.Netns, ifName)
				if err != nil {
					return nil, cmdErr(k8sArgs, "error deleting default gateway: %v", err)
				}
				err = netutils.DeleteDefaultGWCache(n.CNIDir, rt, netName, ifName, deleteV4gateway, deleteV6gateway)
				if err != nil {
					return nil, cmdErr(k8sArgs, "error deleting default gateway in cache: %v", err)
				}
			}

			// Here we'll set the default gateway which specified in `default-route` network selection
			if adddefaultgateway {
				err = netutils.SetDefaultGW(args.Netns, ifName, *delegate.GatewayRequest)
				if err != nil {
					return nil, cmdErr(k8sArgs, "error setting default gateway: %v", err)
				}
				err = netutils.AddDefaultGWCache(n.CNIDir, rt, netName, ifName, *delegate.GatewayRequest)
				if err != nil {
					return nil, cmdErr(k8sArgs, "error setting default gateway in cache: %v", err)
				}
			}
		}

		// Read devInfo from CNIDeviceInfoFile if it exists so
		// it can be copied to the NetworkStatus.
		devinfo, err := getDelegateDeviceInfo(delegate, rt)
		if err != nil {
			// Even if the filename is set, file may not be present. Ignore error,
			// but log and in the future may need to filter on specific errors.
			logging.Debugf("CmdAdd: getDelegateDeviceInfo returned an error - err=%v", err)
		}

		// Create the network statuses, only in case Multus has kubeconfig
		if kubeClient != nil && kc != nil {
			if !types.CheckSystemNamespaces(string(k8sArgs.K8S_POD_NAME), n.SystemNamespaces) {
				delegateNetStatuses, err := nadutils.CreateNetworkStatuses(tmpResult, delegate.Name, delegate.MasterPlugin, devinfo)
				if err != nil {
					return nil, cmdErr(k8sArgs, "error setting network statuses: %v", err)
				}

				// Append all returned statuses after dereferencing each
				for _, status := range delegateNetStatuses {
					netStatus = append(netStatus, *status)
				}
			}
		} else if devinfo != nil {
			// Warn that devinfo exists but could not add it to downwards API
			logging.Errorf("devinfo available, but no kubeConfig so NetworkStatus not modified.")
		}
	}

	// set the network status annotation in apiserver, only in case Multus as kubeconfig
	if kubeClient != nil && kc != nil {
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

// CmdCheck ...
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
		err = DelegateCheck(exec, delegate, rt, in)
		if err != nil {
			return err
		}
	}

	return nil
}

// CmdDel ...
func CmdDel(args *skel.CmdArgs, exec invoke.Exec, kubeClient *k8s.ClientInfo) error {
	in, err := types.LoadNetConf(args.StdinData)
	logging.Debugf("CmdDel: %v, %v, %v", args, exec, kubeClient)
	if err != nil {
		return err
	}

	netns, err := ns.GetNS(args.Netns)
	if netns != nil {
		defer netns.Close()
	}

	k8sArgs, err := k8s.GetK8sArgs(args)
	if err != nil {
		return cmdErr(nil, "error getting k8s args: %v", err)
	}

	if in.ReadinessIndicatorFile != "" {
		readinessfileexists, err := types.ReadinessIndicatorExistsNow(in.ReadinessIndicatorFile)
		if err != nil {
			return cmdErr(k8sArgs, "error checking readinessindicatorfile on CNI DEL @ %v: %v", in.ReadinessIndicatorFile, err)
		}

		if !readinessfileexists {
			logging.Verbosef("warning: readinessindicatorfile @ %v does not exist on CNI DEL", in.ReadinessIndicatorFile)
		}
	}

	kubeClient, err = k8s.GetK8sClient(in.Kubeconfig, kubeClient)
	if err != nil {
		return cmdErr(nil, "error getting k8s client: %v", err)
	}

	pod, err := GetPod(kubeClient, k8sArgs, true)
	if err != nil {
		// GetPod may be failed but just do print error in its log and continue to delete
		logging.Errorf("Multus: GetPod failed: %v, but continue to delete", err)
	}

	// Read the cache to get delegates json for the pod
	netconfBytes, path, err := consumeScratchNetConf(args.ContainerID, in.CNIDir)
	useCacheConf := false
	if err == nil {
		in.Delegates = []*types.DelegateNetConf{}
		if err := json.Unmarshal(netconfBytes, &in.Delegates); err != nil {
			logging.Errorf("Multus: failed to load netconf: %v", err)
		} else {
			useCacheConf = true
			// check plugins field and enable ConfListPlugin if there is
			for _, v := range in.Delegates {
				if len(v.ConfList.Plugins) != 0 {
					v.ConfListPlugin = true
				}
			}
			// First delegate is always the master plugin
			in.Delegates[0].MasterPlugin = true
		}
	}

	if !useCacheConf {
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
	}

	// set CNIVersion in delegate CNI config if there is no CNIVersion and multus conf have CNIVersion.
	for _, v := range in.Delegates {
		if v.ConfListPlugin && v.ConfList.CNIVersion == "" && in.CNIVersion != "" {
			v.ConfList.CNIVersion = in.CNIVersion
			v.Bytes, err = json.Marshal(v.ConfList)
			if err != nil {
				// error happen but continue to delete
				logging.Errorf("Multus: failed to marshal delegate %q config: %v", v.Name, err)
			}
		}
	}

	e := delPlugins(exec, pod, args, k8sArgs, in.Delegates, len(in.Delegates)-1, in.RuntimeConfig, in)

	// Enable Option only delegate plugin delete success to delete cache file
	// CNI Runtime maybe return an error to block sandbox cleanup a while initiative,
	// like starting, prepare something, it will be OK when retry later
	// put "delete cache file" off later ensure have enough info delegate DEL message when Pod has been fully
	// deleted from ETCD before sandbox cleanup success..
	if in.RetryDeleteOnError {
		if useCacheConf {
			// Kubelet though this error as has been cleanup success and never retry, clean cache also
			// Block sandbox cleanup error message can not contain "no such file or directory", CNI Runtime maybe should adaptor it !
			if e == nil || strings.Contains(e.Error(), "no such file or directory") {
				_ = os.Remove(path) // lgtm[go/path-injection]
			}
		}
	} else {
		if useCacheConf {
			// remove used cache file
			_ = os.Remove(path) // lgtm[go/path-injection]
		}
	}

	return e
}
