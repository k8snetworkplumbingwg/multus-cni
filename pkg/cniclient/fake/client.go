package fake

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types040 "github.com/containernetworking/cni/pkg/types/040"

	v1 "k8s.io/api/core/v1"

	multinetspecv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	testhelpers "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/testing"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

// CniPlugin represents a CNI plugin, along with the default cluster network
// configuration.
type CniPlugin struct {
	plugin libcni.CNIConfig
}

func NewFakeCNI(binDirPath string) *CniPlugin {
	return &CniPlugin{plugin: libcni.CNIConfig{Path: []string{binDirPath}}}
}

func (cni *CniPlugin) PluginPath() []string {
	return cni.plugin.Path
}

func (cni *CniPlugin) AddNetworks(kubeclient *k8sclient.ClientInfo, pod *v1.Pod, cniArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, delegateConf *types.DelegateNetConf, rtConf *libcni.RuntimeConf, multusNetconf *types.NetConf) (cnitypes.Result, error) {
	netStatus, err := networkStatus(pod)
	if err != nil {
		return nil, err
	}

	result := &types040.Result{
		CNIVersion: "0.4.0",
		IPs: []*types040.IPConfig{
			{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		},
	}

	delegateNetStatus, err := nadutils.CreateNetworkStatus(result, delegateConf.Name, false, nil)
	if err != nil {
		return nil, err
	}

	netStatus = append(netStatus, *delegateNetStatus)

	if err := k8sclient.SetNetworkStatus(kubeclient, k8sArgs, netStatus, multusNetconf); err != nil {
		return nil, fmt.Errorf("error setting the networks status: %v", err)
	}
	return result, nil
}

func (cni *CniPlugin) RemoveNetworks(pod *v1.Pod, cniArgs *skel.CmdArgs, k8sArgs *types.K8sArgs, delegateConf *types.DelegateNetConf, rtConf *types.RuntimeConfig, multusNetconf *types.NetConf) error {
	return nil
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
