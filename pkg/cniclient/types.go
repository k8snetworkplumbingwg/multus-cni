package cniclient

import (
	"encoding/json"
	"fmt"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"

	v1 "k8s.io/api/core/v1"

	multinetspecv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/multus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

// well known PID of a process running on the host (we share the host's PID
// namespace).
const hostProcessPID = 1

// CNIParams represent
type CNIParams struct {
	CniCmdArgs  skel.CmdArgs
	Namespace   string
	PodName     string
	NetworkName string
}

type Client interface {
	PluginPath() []string
	AddNetworks(kubeclient *k8sclient.ClientInfo, pod *v1.Pod, cniArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, delegateConf *types.DelegateNetConf, rtConf *libcni.RuntimeConf, multusNetconf *types.NetConf) (cnitypes.Result, error)
	RemoveNetworks(pod *v1.Pod, cniArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, delegateConf *types.DelegateNetConf, rtConf *types.RuntimeConfig, multusNetconf *types.NetConf) error
}

// CniPlugin represents a CNI plugin, along with the default cluster network
// configuration.
type CniPlugin struct {
	plugin libcni.CNIConfig
}

// NewCNI returns a new `CniPlugin` pointer, or an error.
func NewCNI(cniBinDir string) *CniPlugin {
	return &CniPlugin{
		plugin: libcni.CNIConfig{Path: []string{cniBinDir}},
	}
}

func (cniParams *CNIParams) BuildRuntimeConf() *libcni.RuntimeConf {
	logging.Verbosef("Pod name: %s; netns path %s; namespace: %s", cniParams.PodName, cniParams.CniCmdArgs.Netns, cniParams.Namespace)

	return &libcni.RuntimeConf{
		ContainerID: cniParams.CniCmdArgs.ContainerID,
		NetNS:       cniParams.CniCmdArgs.Netns,
		IfName:      cniParams.CniCmdArgs.IfName,
		Args: [][2]string{
			{"IgnoreUnknown", "true"},
			{"K8S_POD_NAMESPACE", cniParams.Namespace},
			{"K8S_POD_NAME", cniParams.PodName},
			{"K8S_POD_INFRA_CONTAINER_ID", cniParams.CniCmdArgs.ContainerID},
			{"K8S_POD_NETWORK", cniParams.NetworkName},
		},
	}
}

func (cni *CniPlugin) PluginPath() []string {
	return cni.plugin.Path
}

// AddNetworks taps into the host namespace filesystem (akin to chroot) and
// triggers a CNI_ADD for an existing interface over the delegate CNI plugin.
func (cni *CniPlugin) AddNetworks(kubeclient *k8sclient.ClientInfo, pod *v1.Pod, cniArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, delegateConf *types.DelegateNetConf, rtConf *libcni.RuntimeConf, multusNetconf *types.NetConf) (cnitypes.Result, error) {
	logging.Verbosef("AddNetworks: cni args: %+v", *cniArgs)
	fd, err := tapIntoMountNamespace(hostProcessPID)
	defer closeFD(fd)
	if err != nil {
		return nil, err
	}
	defer func(didSucceed bool) {
		if didSucceed {
			fd, _ = tapIntoMountNamespace(uint(os.Getpid()))
			closeFD(fd)
		}
	}(err == nil)

	netStatus, err := networkStatus(pod)
	if err != nil {
		return nil, err
	}

	res, netStatus, err := multus.AddDelegate(cniArgs, nil, kubeclient, delegateConf, 0, k8sArgs, multusNetconf, pod, netStatus, rtConf)
	if err != nil {
		return nil, err
	}

	ips, err := multus.IPsFromResult(res)
	if err != nil {
		return nil, err
	}
	multus.SendKubernetesEvents(pod, delegateConf.Name, kubeclient, rtConf, ips)

	if err := k8sclient.SetNetworkStatus(kubeclient, k8sArgs, netStatus, multusNetconf); err != nil {
		if strings.Contains(err.Error(), "failed to query the pod") {
			return nil, fmt.Errorf("error setting the networks status: %v", err)
		}
		return nil, fmt.Errorf("error setting the networks status: %v", err)
	}

	return res, nil
}

func networkStatus(pod *v1.Pod) ([]multinetspecv1.NetworkStatus, error) {
	netStatusString, wasFound := pod.Annotations[multinetspecv1.NetworkStatusAnnot]

	var netStatus []multinetspecv1.NetworkStatus
	if wasFound {
		if err := json.Unmarshal([]byte(netStatusString), &netStatus); err != nil {
			return nil, fmt.Errorf("failed to unmarshall the netstatus annotation: %v", err)
		}
	}
	return netStatus, nil
}

// RemoveNetworks taps into the host namespace filesystem (akin to chroot) and
// triggers a CNI_DEL for an existing interface over the delegate CNI plugin.
func (cni *CniPlugin) RemoveNetworks(pod *v1.Pod, cniArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, delegateConf *types.DelegateNetConf, rtConf *types.RuntimeConfig, multusNetconf *types.NetConf) error {
	fd, err := tapIntoMountNamespace(hostProcessPID)
	defer closeFD(fd)
	if err != nil {
		return err
	}
	defer func(didSucceed bool) {
		if didSucceed {
			fd, _ = tapIntoMountNamespace(uint(os.Getpid()))
			closeFD(fd)
		}
	}(err == nil)
	return multus.DelPlugins(nil, pod, cniArgs, k8sArgs, []*types.DelegateNetConf{delegateConf}, 0, rtConf, multusNetconf)
}

func closeFD(fd *os.File) {
	if fd != nil {
		_ = fd.Close()
	}
}

func tapIntoMountNamespace(pid uint) (*os.File, error) {
	hostMountNamespace := fmt.Sprintf("/proc/%d/ns/mnt", pid)
	fd, err := os.Open(hostMountNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to open mount namespace: %v", err)
	}
	if err = unix.Unshare(unix.CLONE_NEWNS); err != nil {
		return fd, fmt.Errorf("failed to detach from parent mount namespace: %v", err)
	}
	if err := unix.Setns(int(fd.Fd()), unix.CLONE_NEWNS); err != nil {
		return fd, fmt.Errorf("failed to join the mount namespace: %v", err)
	}
	return fd, nil
}
