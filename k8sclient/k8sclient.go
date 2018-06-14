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
	"bytes"
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

func getPodNetworkAnnotation(client KubeClient, k8sArgs types.K8sArgs) (string, error) {
	var annot string
	var err error

	pod, err := client.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
	if err != nil {
		return annot, fmt.Errorf("getPodNetworkAnnotation: failed to query the pod %v in out of cluster comm: %v", string(k8sArgs.K8S_POD_NAME), err)
	}

	return pod.Annotations["kubernetes.v1.cni.cncf.io/networks"], nil
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

func parsePodNetworkObject(podnetwork string) ([]map[string]interface{}, error) {
	var podNet []map[string]interface{}

	if podnetwork == "" {
		return nil, fmt.Errorf("parsePodNetworkObject: pod annotation not having \"network\" as key, refer Multus README.md for the usage guide")
	}

	// Parse the podnetwork string, and assume it is JSON.
	if err := json.Unmarshal([]byte(podnetwork), &podNet); err != nil {

		// If JSON doesn't parse, assume comma-delimited.
		commaItems := strings.Split(podnetwork, ",")

		// Build a map from the comma delimited items.
		for i := range commaItems {

			// Remove leading and trailing whitespace.
			commaItems[i] = strings.TrimSpace(commaItems[i])

			// Parse network name (i.e. <namespace>/<network name>@<ifname>)
			netNsName, networkName, netIfName, err := parsePodNetworkObjectName(commaItems[i])
			if err != nil {
				return nil, fmt.Errorf("parsePodNetworkObject: %v", err)
			}
			m := make(map[string]interface{})
			m["name"] = networkName
			if netNsName != "" {
				m["namespace"] = netNsName
			}
			if netIfName != "" {
				m["interfaceRequest"] = netIfName
			}

			podNet = append(podNet, m)
		}
	}

	return podNet, nil
}

func getCNIConfig(name string, primary bool, ifname string, confdir string) (string, error) {

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
		fmt.Errorf("No networks found in %s", confdir)
	case len(files) == 0:
		fmt.Errorf("No networks found in %s", confdir)
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

			return getConfig(string(conf.Bytes[:]), primary, ifname), nil

		}
	}

	return "", fmt.Errorf("no network available in the name %s in cni dir %s", name, confdir)
}

func getPlugin(plugin string, name string, primary bool, ifname string) string {
	tmpconfig := []string{}

	tmpconfig = append(tmpconfig, fmt.Sprintf(`{"cniVersion": "0.3.1" , "name": "%s", "type": "%s"`, name, plugin))

	if primary != false {
		tmpconfig = append(tmpconfig, `, "masterplugin": true`)
	}

	if ifname != "" {
		tmpconfig = append(tmpconfig, fmt.Sprintf(`, "ifnameRequest": "%s"`, ifname))
	}

	tmpconfig = append(tmpconfig, "}")

	return strings.Join(tmpconfig, "")

}

func getConfig(config string, primary bool, ifname string) string {
	tmpconfig := []string{}

	config = strings.TrimSpace(config)
	tmpconfig = append(tmpconfig, config[:1])

	if primary != false {
		tmpconfig = append(tmpconfig, ` "masterplugin": true,`)
	}

	if ifname != "" {
		tmpconfig = append(tmpconfig, fmt.Sprintf(` "ifnameRequest": "%s",`, ifname))
	}

	tmpconfig = append(tmpconfig, config[1:])

	return strings.Join(tmpconfig, "")

}

func getNetSpec(ns types.NetworkSpec, name string, primary bool, ifname string) (string, error) {

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
		return getPlugin(ns.Plugin, name, primary, ifname), nil
	}

	// Config contains a standard JSON-encoded CNI configuration
	// or configuration list which defines the plugin chain to
	// execute.  If present, this key takes precedence over
	// ‘Plugin’.
	return getConfig(ns.Config, primary, ifname), nil

}

func getNetObject(net types.Network, primary bool, ifname string, confdir string) (string, error) {
	var config string
	var err error

	if (types.NetworkSpec{}) == net.Spec {
		config, err = getCNIConfig(net.Metadata.Name, primary, ifname, confdir)
		if err != nil {
			return "", fmt.Errorf("getNetObject: err in getCNIConfig: %v", err)
		}
	} else {
		config, err = getNetSpec(net.Spec, net.Metadata.Name, primary, ifname)
		if err != nil {
			return "", fmt.Errorf("getNetObject: err in getNetSpec: %v", err)
		}
	}

	return config, nil
}

func getnetplugin(client KubeClient, networkinfo map[string]interface{}, primary bool, confdir string) (string, error) {
	networkname := networkinfo["name"].(string)
	if networkname == "" {
		return "", fmt.Errorf("getnetplugin: network name can't be empty")
	}

	netNsName := "default"
	if networkinfo["namespace"] != nil {
		netNsName = networkinfo["namespace"].(string)
	}

	tprclient := fmt.Sprintf("/apis/kubernetes.cni.cncf.io/v1/namespaces/%s/networks/%s", netNsName, networkname)

	netobjdata, err := client.GetRawWithPath(tprclient)
	if err != nil {
		return "", fmt.Errorf("getnetplugin: failed to get CRD (result: %s), refer Multus README.md for the usage guide: %v", netobjdata, err)
	}

	netobj := types.Network{}
	if err := json.Unmarshal(netobjdata, &netobj); err != nil {
		return "", fmt.Errorf("getnetplugin: failed to get the netplugin data: %v", err)
	}

	ifnameRequest := ""
	if networkinfo["interfaceRequest"] != nil {
		ifnameRequest = networkinfo["interfaceRequest"].(string)
	}

	netargs, err := getNetObject(netobj, primary, ifnameRequest, confdir)
	if err != nil {
		return "", err
	}

	return netargs, nil
}

func getPodNetworkObj(client KubeClient, netObjs []map[string]interface{}, confdir string) (string, error) {

	var np string
	var err error

	var str bytes.Buffer
	str.WriteString("[")

	for index, net := range netObjs {
		var primary bool

		if index == 0 {
			primary = true
		}

		np, err = getnetplugin(client, net, primary, confdir)
		if err != nil {
			return "", fmt.Errorf("getPodNetworkObj: failed in getting the netplugin: %v", err)
		}

		str.WriteString(np)
		if index != (len(netObjs) - 1) {
			str.WriteString(",")
		}
	}

	str.WriteString("]")
	netconf := str.String()
	return netconf, nil
}

func getMultusDelegates(delegate string) ([]*types.DelegateNetConf, error) {
	if delegate == "" {
		return nil, fmt.Errorf("getMultusDelegates: TPR network obj data can't be empty")
	}

	n, err := types.LoadNetConf([]byte("{\"delegates\": " + delegate + "}"))
	if err != nil {
		return nil, fmt.Errorf("getMultusDelegates: failed to load netconf for delegate %v: %v", delegate, err)
	}

	if len(n.Delegates) == 0 {
		return nil, fmt.Errorf(`getMultusDelegates: "delegates" is must, refer Multus README.md for the usage guide`)
	}

	return n.Delegates, nil
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

	netAnnot, err := getPodNetworkAnnotation(k8sclient, k8sArgs)
	if err != nil {
		return nil, err
	}

	if len(netAnnot) == 0 {
		return nil, &NoK8sNetworkError{"no kubernetes network found"}
	}

	netObjs, err := parsePodNetworkObject(netAnnot)
	if err != nil {
		return nil, err
	}

	multusDelegates, err := getPodNetworkObj(k8sclient, netObjs, confdir)
	if err != nil {
		return nil, err
	}

	return getMultusDelegates(multusDelegates)
}
