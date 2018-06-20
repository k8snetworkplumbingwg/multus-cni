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

package k8sclient

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/intel/multus-cni/types"
)

// NoK8sNetworkError indicates error, no network in kubernetes
type NoK8sNetworkError struct {
	message string
}

func (e *NoK8sNetworkError) Error() string { return string(e.message) }

type defaultKubeClient struct {
	client kubernetes.Interface
}

// defaultKubeClient implements KubeClient
var _ KubeClient = &defaultKubeClient{}

func (d *defaultKubeClient) GetRawWithPath(path string) ([]byte, error) {
	return d.client.ExtensionsV1beta1().RESTClient().Get().AbsPath(path).DoRaw()
}

func (d *defaultKubeClient) GetPod(namespace, name string) (*v1.Pod, error) {
	return d.client.Core().Pods(namespace).Get(name, metav1.GetOptions{})
}

func createK8sClient(kubeconfig string) (KubeClient, error) {
	// uses the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("createK8sClient: failed to get context for the kubeconfig %v, refer Multus README.md for the usage guide: %v", kubeconfig, err)
	}

	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &defaultKubeClient{client: client}, nil
}

func getPodNetworkAnnotation(client KubeClient, k8sArgs types.K8sArgs) (string, string, error) {
	var err error

	pod, err := client.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
	if err != nil {
		return "", "", fmt.Errorf("getPodNetworkAnnotation: failed to query the pod %v in out of cluster comm: %v", string(k8sArgs.K8S_POD_NAME), err)
	}

	return pod.Annotations["kubernetes.v1.cni.cncf.io/networks"], pod.ObjectMeta.Namespace, nil
}

func parsePodNetworkObjectName(podnetwork string) (string, string, string, error) {
	var netNsName string
	var netIfName string
	var networkName string

	slashItems := strings.Split(podnetwork, "/")
	if len(slashItems) == 2 {
		netNsName = strings.TrimSpace(slashItems[0])
		networkName = slashItems[1]
	} else if len(slashItems) == 1 {
		networkName = slashItems[0]
	} else {
		return "", "", "", fmt.Errorf("Invalid network object (failed at '/')")
	}

	atItems := strings.Split(networkName, "@")
	networkName = strings.TrimSpace(atItems[0])
	if len(atItems) == 2 {
		netIfName = strings.TrimSpace(atItems[1])
	} else if len(atItems) != 1 {
		return "", "", "", fmt.Errorf("Invalid network object (failed at '@')")
	}

	// Check and see if each item matches the specification for valid attachment name.
	// "Valid attachment names must be comprised of units of the DNS-1123 label format"
	// [a-z0-9]([-a-z0-9]*[a-z0-9])?
	// And we allow at (@), and forward slash (/) (units separated by commas)
	// It must start and end alphanumerically.
	allItems := []string{netNsName, networkName, netIfName}
	for i := range allItems {
		matched, _ := regexp.MatchString("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", allItems[i])
		if !matched && len([]rune(allItems[i])) > 0 {
			return "", "", "", fmt.Errorf(fmt.Sprintf("Failed to parse: one or more items did not match comma-delimited format (must consist of lower case alphanumeric characters). Must start and end with an alphanumeric character), mismatch @ '%v'", allItems[i]))
		}
	}

	return netNsName, networkName, netIfName, nil
}

func parsePodNetworkAnnotation(podNetworks, defaultNamespace string) ([]*types.NetworkSelectionElement, error) {
	var networks []*types.NetworkSelectionElement

	if podNetworks == "" {
		return nil, fmt.Errorf("parsePodNetworkAnnotation: pod annotation not having \"network\" as key, refer Multus README.md for the usage guide")
	}

	if strings.IndexAny(podNetworks, "[{\"") >= 0 {
		if err := json.Unmarshal([]byte(podNetworks), &networks); err != nil {
			return nil, fmt.Errorf("parsePodNetworkAnnotation: failed to parse pod Network Attachment Selection Annotation JSON format: %v", err)
		}
	} else {
		// Comma-delimited list of network attachment object names
		for _, item := range strings.Split(podNetworks, ",") {
			// Remove leading and trailing whitespace.
			item = strings.TrimSpace(item)

			// Parse network name (i.e. <namespace>/<network name>@<ifname>)
			netNsName, networkName, netIfName, err := parsePodNetworkObjectName(item)
			if err != nil {
				return nil, fmt.Errorf("parsePodNetworkAnnotation: %v", err)
			}

			networks = append(networks, &types.NetworkSelectionElement{
				Name:             networkName,
				Namespace:        netNsName,
				InterfaceRequest: netIfName,
			})
		}
	}

	for _, net := range networks {
		if net.Namespace == "" {
			net.Namespace = defaultNamespace
		}
	}

	return networks, nil
}

func getCNIConfig(name string, ifname string, confdir string) (string, error) {

	// In the absence of valid keys in a Spec, the runtime (or
	// meta-plugin) should load and execute a CNI .configlist
	// or .config (in that order) file on-disk whose JSON
	// “name” key matches this Network object’s name.

	//Todo
	// support conflist for chaining mechanism
	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go#getDefaultCNINetwork
	files, err := libcni.ConfFiles(confdir, []string{".conf", ".json"})
	switch {
	case err != nil:
		return "", fmt.Errorf("No networks found in %s", confdir)
	case len(files) == 0:
		return "", fmt.Errorf("No networks found in %s", confdir)
	}

	for _, confFile := range files {
		conf, err := libcni.ConfFromFile(confFile)
		if err != nil {
			return "", fmt.Errorf("Error loading CNI config file %s: %v", confFile, err)
		}

		if conf.Network.Name == name {
			// Ensure the config has a "type" so we know what plugin to run.
			// Also catches the case where somebody put a conflist into a conf file.
			if conf.Network.Type == "" {
				return "", fmt.Errorf("Error loading CNI config file %s: no 'type'; perhaps this is a .conflist?", confFile)
			}

			return getConfig(string(conf.Bytes[:]), ifname), nil

		}
	}

	return "", fmt.Errorf("no network available in the name %s in cni dir %s", name, confdir)
}

func getPlugin(plugin string, name string, ifname string) string {
	tmpconfig := []string{}

	tmpconfig = append(tmpconfig, fmt.Sprintf(`{"cniVersion": "0.3.1" , "name": "%s", "type": "%s"`, name, plugin))

	if ifname != "" {
		tmpconfig = append(tmpconfig, fmt.Sprintf(`, "ifnameRequest": "%s"`, ifname))
	}

	tmpconfig = append(tmpconfig, "}")

	return strings.Join(tmpconfig, "")

}

func getConfig(config string, ifname string) string {
	tmpconfig := []string{}

	config = strings.TrimSpace(config)
	tmpconfig = append(tmpconfig, config[:1])

	if ifname != "" {
		tmpconfig = append(tmpconfig, fmt.Sprintf(` "ifnameRequest": "%s",`, ifname))
	}

	tmpconfig = append(tmpconfig, config[1:])

	return strings.Join(tmpconfig, "")

}

func getNetSpec(ns types.NetworkSpec, name string, ifname string) (string, error) {

	if ns.Plugin == "" && ns.Config == "" {
		return "", fmt.Errorf("Network Object spec plugin and config can't be empty")
	}

	if ns.Plugin != "" && ns.Config != "" {
		return "", fmt.Errorf("Network Object spec can't have both plugin and config")
	}

	if ns.Plugin != "" {
		// Plugin contains the name of a CNI plugin on-disk in a
		// runtime-defined path (eg /opt/cni/bin and/or other paths.
		// This plugin should be executed with a basic CNI JSON
		// configuration on stdin containing the Network object
		// name and the plugin:
		//   { “cniVersion”: “0.3.1”, “type”: <Plugin>, “name”: <Network.Name> }
		// and any additional “runtimeConfig” field per the
		// CNI specification and conventions.
		return getPlugin(ns.Plugin, name, ifname), nil
	}

	// Config contains a standard JSON-encoded CNI configuration
	// or configuration list which defines the plugin chain to
	// execute.  If present, this key takes precedence over
	// ‘Plugin’.
	return getConfig(ns.Config, ifname), nil

}

func cniConfigFromNetworkResource(customResource *types.Network, net *types.NetworkSelectionElement, confdir string) (string, error) {
	var config string
	var err error

	if (types.NetworkSpec{}) == customResource.Spec {
		// Network Spec empty; generate delegate from CNI JSON config
		// from the configuration directory that has the same network
		// name as the custom resource
		config, err = getCNIConfig(customResource.Metadata.Name, net.InterfaceRequest, confdir)
		if err != nil {
			return "", fmt.Errorf("cniConfigFromNetworkResource: err in getCNIConfig: %v", err)
		}
	} else {
		// Generate delegate from CNI configuration embedded in the
		// custom resource
		config, err = getNetSpec(customResource.Spec, customResource.Metadata.Name, net.InterfaceRequest)
		if err != nil {
			return "", fmt.Errorf("cniConfigFromNetworkResource: err in getNetSpec: %v", err)
		}
	}

	return config, nil
}

func getKubernetesDelegate(client KubeClient, net *types.NetworkSelectionElement, confdir string) (*types.DelegateNetConf, error) {
	rawPath := fmt.Sprintf("/apis/kubernetes.cni.cncf.io/v1/namespaces/%s/networks/%s", net.Namespace, net.Name)
	netData, err := client.GetRawWithPath(rawPath)
	if err != nil {
		return nil, fmt.Errorf("getKubernetesDelegate: failed to get network resource, refer Multus README.md for the usage guide: %v", err)
	}

	customResource := &types.Network{}
	if err := json.Unmarshal(netData, customResource); err != nil {
		return nil, fmt.Errorf("getKubernetesDelegate: failed to get the netplugin data: %v", err)
	}

	cniConfig, err := cniConfigFromNetworkResource(customResource, net, confdir)
	if err != nil {
		return nil, err
	}

	delegate, err := types.LoadDelegateNetConf([]byte(cniConfig))
	if err != nil {
		return nil, err
	}

	return delegate, nil
}

type KubeClient interface {
	GetRawWithPath(path string) ([]byte, error)
	GetPod(namespace, name string) (*v1.Pod, error)
}

func GetK8sNetwork(args *skel.CmdArgs, kubeconfig string, k8sclient KubeClient, confdir string) ([]*types.DelegateNetConf, error) {
	k8sArgs := types.K8sArgs{}

	err := cnitypes.LoadArgs(args.Args, &k8sArgs)
	if err != nil {
		return nil, err
	}

	if k8sclient == nil {
		k8sclient, err = createK8sClient(kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	netAnnot, defaultNamespace, err := getPodNetworkAnnotation(k8sclient, k8sArgs)
	if err != nil {
		return nil, err
	}

	if len(netAnnot) == 0 {
		return nil, &NoK8sNetworkError{"no kubernetes network found"}
	}

	networks, err := parsePodNetworkAnnotation(netAnnot, defaultNamespace)
	if err != nil {
		return nil, err
	}

	// Read all network objects referenced by 'networks'
	var delegates []*types.DelegateNetConf
	for _, net := range networks {
		delegate, err := getKubernetesDelegate(k8sclient, net, confdir)
		if err != nil {
			return nil, fmt.Errorf("GetK8sNetwork: failed getting the delegate: %v", err)
		}
		delegates = append(delegates, delegate)
	}

	return delegates, nil
}
