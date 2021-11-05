package k8sclient

import (
	"fmt"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
	v1 "k8s.io/api/core/v1"
)

// NetConfGenerator generates the delegates that go in the multus configuration
// struct.
type NetConfGenerator struct {
	clientInfo  *ClientInfo
	conf        *types.NetConf
	pod         *v1.Pod
	networks    []*types.NetworkSelectionElement
	resourceMap map[string]*types.ResourceInfo
}

// NewHotplugNetConfGenerator creates a multus netconf generator for the hotplug scenario.
func NewHotplugNetConfGenerator(clientInfo *ClientInfo, initialConf *types.NetConf, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo, networkName string, ifaceName string) (*NetConfGenerator, error) {
	podNetworks, err := GetPodNetwork(pod)
	if err != nil {
		return nil, err
	}

	network, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName(networkName, ifaceName))
	if err != nil {
		return nil, &NoK8sNetworkError{
			message: fmt.Sprintf("could not find network: %s; iface: %s in the pod's annotations", networkName, ifaceName)}
	}

	initialConf.Delegates = []*types.DelegateNetConf{}
	return &NetConfGenerator{
		clientInfo:  clientInfo,
		conf:        initialConf,
		pod:         pod,
		networks:    []*types.NetworkSelectionElement{network},
		resourceMap: resourceMap,
	}, nil
}

// NewHotUnplugNetConfGenerator creates a multus netconf generator for the hot-unplug scenario.
func NewHotUnplugNetConfGenerator(clientInfo *ClientInfo, initialConf *types.NetConf, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo, networkName string, ifaceName string) (*NetConfGenerator, error) {
	podNetworks, err := GetPodNetworkFromStatus(pod)
	if err != nil {
		return nil, err
	}

	network, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName(networkName, ifaceName))
	if err != nil {
		return nil, &NoK8sNetworkError{
			message: fmt.Sprintf("could not find network: %s; iface: %s in the pod's annotations", networkName, ifaceName)}
	}

	initialConf.Delegates = []*types.DelegateNetConf{}
	return &NetConfGenerator{
		clientInfo:  clientInfo,
		conf:        initialConf,
		pod:         pod,
		networks:    []*types.NetworkSelectionElement{network},
		resourceMap: resourceMap,
	}, nil
}

func filterNetworkByNetAndIfaceName(networkName string, ifaceName string) func(net types.NetworkSelectionElement) bool {
	return func(net types.NetworkSelectionElement) bool {
		if net.Name == networkName && net.InterfaceRequest == ifaceName {
			return true
		}
		return false
	}
}

// NewNetConfGenerator returns a multus netconf generator for use on pod
// creation / deletion.
func NewNetConfGenerator(clientInfo *ClientInfo, initialConf *types.NetConf, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) (*NetConfGenerator, error) {
	networks, err := GetPodNetwork(pod)
	if err != nil {
		return nil, err
	}

	return &NetConfGenerator{
		clientInfo:  clientInfo,
		conf:        initialConf,
		pod:         pod,
		networks:    networks,
		resourceMap: resourceMap,
	}, nil
}

func (cg *NetConfGenerator) generate() (*types.NetConf, error) {
	delegates, err := cg.computeDelegates()
	if err != nil {
		return nil, err
	}
	cg.conf.Delegates = append(cg.conf.Delegates, delegates...)
	cg.handleDefaultRoute()
	return cg.conf, nil
}

func (cg *NetConfGenerator) computeDelegates() ([]*types.DelegateNetConf, error) {
	if cg.networks != nil {
		delegates, err := GetNetworkDelegates(cg.clientInfo, cg.pod, cg.networks, cg.conf, cg.resourceMap)
		if err != nil {
			if _, ok := err.(*NoK8sNetworkError); ok {
				return nil, nil
			}
			return nil, logging.Errorf("TryLoadPodDelegates: error in getting k8s network for pod: %v", err)
		}

		return delegates, nil
	}

	return nil, nil
}

func (cg *NetConfGenerator) handleDefaultRoute() {
	// Check gatewayRequest is configured in delegates
	// and mark its config if gateway filter is required
	isGatewayConfigured := false
	for _, delegate := range cg.conf.Delegates {
		if delegate.GatewayRequest != nil {
			isGatewayConfigured = true
			break
		}
	}

	if isGatewayConfigured == true {
		types.CheckGatewayConfig(cg.conf.Delegates)
	}
}
