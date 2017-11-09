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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
)

const defaultCNIDir = "/var/lib/cni/multus"

var masterpluginEnabled bool
var defaultcninetwork bool

type NetConf struct {
	types.NetConf
	CNIDir     string                   `json:"cniDir"`
	Delegates  []map[string]interface{} `json:"delegates"`
	Kubeconfig string                   `json:"kubeconfig"`
}

type PodNet struct {
	Networkname string `json:"name"`
}

type netplugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" description:"standard object metadata"`
	Plugin            string `json:"plugin"`
	Args              string `json:"args"`
}

// K8sArgs is the valid CNI_ARGS used for Kubernetes
type K8sArgs struct {
	types.CommonArgs
	IP                         net.IP
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

//taken from cni/plugins/meta/flannel/flannel.go
func isString(i interface{}) bool {
	_, ok := i.(string)
	return ok
}

func isBool(i interface{}) bool {
	_, ok := i.(bool)
	return ok
}

func loadNetConf(bytes []byte) (*NetConf, error) {
	netconf := &NetConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	if netconf.Kubeconfig != "" && netconf.Delegates != nil {
		defaultcninetwork = true
	}

	if netconf.Kubeconfig != "" && !defaultcninetwork {
		return netconf, nil
	}

	if len(netconf.Delegates) == 0 && !defaultcninetwork {
		return nil, fmt.Errorf(`delegates or kubeconfig option is must, refer README.md`)
	}

	if netconf.CNIDir == "" {
		netconf.CNIDir = defaultCNIDir
	}

	return netconf, nil
}

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

func getifname() (f func() string) {
	var interfaceIndex int
	f = func() string {
		ifname := fmt.Sprintf("net%d", interfaceIndex)
		interfaceIndex++
		return ifname
	}

	return
}

func saveDelegates(containerID, dataDir string, delegates []map[string]interface{}) error {
	delegatesBytes, err := json.Marshal(delegates)
	if err != nil {
		return fmt.Errorf("error serializing delegate netconf: %v", err)
	}

	if err = saveScratchNetConf(containerID, dataDir, delegatesBytes); err != nil {
		return fmt.Errorf("error in saving the  delegates : %v", err)
	}

	return err
}

func checkDelegate(netconf map[string]interface{}) error {
	if netconf["type"] == nil {
		return fmt.Errorf("delegate must have the field 'type'")
	}

	if !isString(netconf["type"]) {
		return fmt.Errorf("delegate field 'type' must be a string")
	}

	if netconf["masterplugin"] != nil {
		if !isBool(netconf["masterplugin"]) {
			return fmt.Errorf("delegate field 'masterplugin' must be a bool")
		}
	}

	if netconf["masterplugin"] != nil {
		if netconf["masterplugin"].(bool) != false && masterpluginEnabled != true {
			masterpluginEnabled = true
		} else if netconf["masterplugin"].(bool) != false && masterpluginEnabled == true {
			return fmt.Errorf("only one delegate can have 'masterplugin'")
		}
	}
	return nil
}

func isMasterplugin(netconf map[string]interface{}) bool {
	if netconf["masterplugin"] == nil {
		return false
	}

	if netconf["masterplugin"].(bool) == true {
		return true
	}

	return false
}

func delegateAdd(podif func() string, argif string, netconf map[string]interface{}, onlyMaster bool) (bool, error) {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return true, fmt.Errorf("Multus: error serializing multus delegate netconf: %v", err)
	}

	if isMasterplugin(netconf) != onlyMaster {
		return true, nil
	}

	if !isMasterplugin(netconf) {
		if os.Setenv("CNI_IFNAME", podif()) != nil {
			return true, fmt.Errorf("Multus: error in setting CNI_IFNAME")
		}
	} else {
		if os.Setenv("CNI_IFNAME", argif) != nil {
			return true, fmt.Errorf("Multus: error in setting CNI_IFNAME")
		}
	}

	result, err := invoke.DelegateAdd(netconf["type"].(string), netconfBytes)
	if err != nil {
		return true, fmt.Errorf("Multus: error in invoke Delegate add - %q: %v", netconf["type"].(string), err)
	}

	if !isMasterplugin(netconf) {
		return true, nil
	}

	return false, result.Print()
}

func delegateDel(podif func() string, argif string, netconf map[string]interface{}) error {
	netconfBytes, err := json.Marshal(netconf)
	if err != nil {
		return fmt.Errorf("Multus: error serializing multus delegate netconf: %v", err)
	}

	if !isMasterplugin(netconf) {
		if os.Setenv("CNI_IFNAME", podif()) != nil {
			return fmt.Errorf("Multus: error in setting CNI_IFNAME")
		}
	} else {
		if os.Setenv("CNI_IFNAME", argif) != nil {
			return fmt.Errorf("Multus: error in setting CNI_IFNAME")
		}
	}

	err = invoke.DelegateDel(netconf["type"].(string), netconfBytes)
	if err != nil {
		return fmt.Errorf("Multus: error in invoke Delegate del - %q: %v", netconf["type"].(string), err)
	}

	return err
}

func clearPlugins(mIdx int, pIdx int, argIfname string, delegates []map[string]interface{}) error {

	if os.Setenv("CNI_COMMAND", "DEL") != nil {
		return fmt.Errorf("Multus: error in setting CNI_COMMAND to DEL")
	}

	podifName := getifname()
	r := delegateDel(podifName, argIfname, delegates[mIdx])
	if r != nil {
		return r
	}

	for idx := 0; idx < pIdx && idx != mIdx; idx++ {
		r := delegateDel(podifName, argIfname, delegates[idx])
		if r != nil {
			return r
		}
	}

	return nil
}

func createK8sClient(kubeconfig string) (*kubernetes.Clientset, error) {

	// uses the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("createK8sClient: failed to get context for the kubeconfig %v, refer Multus README.md for the usage guide", kubeconfig)
	}

	// creates the clientset
	return kubernetes.NewForConfig(config)
}

func getPodNetworkAnnotation(client *kubernetes.Clientset, k8sArgs K8sArgs) (string, error) {
	var annot string
	var err error

	pod, err := client.Pods(string(k8sArgs.K8S_POD_NAMESPACE)).Get(fmt.Sprintf("%s", string(k8sArgs.K8S_POD_NAME)), metav1.GetOptions{})
	if err != nil {
		return annot, fmt.Errorf("getPodNetworkAnnotation: failed to query the pod %v in out of cluster comm", string(k8sArgs.K8S_POD_NAME))
	}

	return pod.Annotations["networks"], nil
}

func parsePodNetworkObject(podnetwork string) ([]map[string]interface{}, error) {
	var podNet []map[string]interface{}

	if podnetwork == "" {
		return nil, fmt.Errorf("parsePodNetworkObject: pod annotation not having \"network\" as key, refer Multus README.md for the usage guide")
	}

	if err := json.Unmarshal([]byte(podnetwork), &podNet); err != nil {
		return nil, fmt.Errorf("parsePodNetworkObject: failed to load pod network err: %v | pod network: %v", err, podnetwork)
	}

	return podNet, nil
}

func getpluginargs(name string, args string, primary bool) (string, error) {
	var netconf string
	var tmpargs []string

	if name == "" || args == "" {
		return "", fmt.Errorf("getpluginargs: plugin name/args can't be empty")
	}

	if primary != false {
		tmpargs = []string{`{"type": "`, name, `","masterplugin": true,`, args[strings.Index(args, "\"") : len(args)-1]}
	} else {
		tmpargs = []string{`{"type": "`, name, `",`, args[strings.Index(args, "\"") : len(args)-1]}
	}

	var str bytes.Buffer

	for _, a := range tmpargs {
		str.WriteString(a)
	}

	netconf = str.String()
	return netconf, nil

}

func getnetplugin(client *kubernetes.Clientset, networkname string, primary bool) (string, error) {
	if networkname == "" {
		return "", fmt.Errorf("getnetplugin: network name can't be empty")
	}

	tprclient := fmt.Sprintf("/apis/kubernetes.com/v1/namespaces/default/networks/%s", networkname)

	netobjdata, err := client.ExtensionsV1beta1().RESTClient().Get().AbsPath(tprclient).DoRaw()
	if err != nil {
		return "", fmt.Errorf("getnetplugin: failed to get TRP, refer Multus README.md for the usage guide: %v", err)
	}

	np := netplugin{}
	if err := json.Unmarshal(netobjdata, &np); err != nil {
		return "", fmt.Errorf("getnetplugin: failed to get the netplugin data: %v", err)
	}

	netargs, err := getpluginargs(np.Plugin, np.Args, primary)
	if err != nil {
		return "", err
	}

	return netargs, nil
}

func getPodNetworkObj(client *kubernetes.Clientset, netObjs []map[string]interface{}) (string, error) {

	var np string
	var err error

	var str bytes.Buffer
	str.WriteString("[")

	for index, net := range netObjs {
		var primary bool

		if index == 0 {
			primary = true
		}

		np, err = getnetplugin(client, net["name"].(string), primary)
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

func getMultusDelegates(delegate string) ([]map[string]interface{}, error) {
	tmpNetconf := &NetConf{}
	tmpDelegate := "{\"delegates\": " + delegate + "}"

	if delegate == "" {
		return nil, fmt.Errorf("getMultusDelegates: TPR network obj data can't be empty")
	}

	if err := json.Unmarshal([]byte(tmpDelegate), tmpNetconf); err != nil {
		return nil, fmt.Errorf("getMultusDelegates: failed to load netconf: %v", err)
	}

	if tmpNetconf.Delegates == nil {
		return nil, fmt.Errorf(`getMultusDelegates: "delegates" is must, refer Multus README.md for the usage guide`)
	}

	return tmpNetconf.Delegates, nil
}

func getK8sNetwork(args *skel.CmdArgs, kubeconfig string) ([]map[string]interface{}, error) {
	k8sArgs := K8sArgs{}
	var podNet []map[string]interface{}

	err := types.LoadArgs(args.Args, &k8sArgs)
	if err != nil {
		return podNet, err
	}

	k8sclient, err := createK8sClient(kubeconfig)
	if err != nil {
		return podNet, err
	}

	netAnnot, err := getPodNetworkAnnotation(k8sclient, k8sArgs)
	if err != nil {
		return podNet, err
	}

	if len(netAnnot) == 0 {
		return podNet, fmt.Errorf(`nonet`)
	}

	netObjs, err := parsePodNetworkObject(netAnnot)
	if err != nil {
		return podNet, err
	}

	multusDelegates, err := getPodNetworkObj(k8sclient, netObjs)
	if err != nil {
		return podNet, err
	}

	podNet, err = getMultusDelegates(multusDelegates)
	if err != nil {
		return podNet, err
	}

	return podNet, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	var result error
	var nopodnet bool
	n, err := loadNetConf(args.StdinData)
	if err != nil {
		return fmt.Errorf("err in loading netconf: %v", err)
	}

	if n.Kubeconfig != "" {
		podDelegate, r := getK8sNetwork(args, n.Kubeconfig)
		if r != nil && r.Error() == "nonet" {
			nopodnet = true
			if !defaultcninetwork {
				return fmt.Errorf("Multus: Err in getting k8s network from the pod spec annotation, check the pod spec or set delegate for the default network, Refer the README.md: %v", r)
			}
		}

		if r != nil && !defaultcninetwork {
			return fmt.Errorf("Multus: Err in getting k8s network from pod: %v", r)
		}

		if len(podDelegate) != 0 {
			n.Delegates = podDelegate
		}
	}

	for _, delegate := range n.Delegates {
		if err := checkDelegate(delegate); err != nil {
			return fmt.Errorf("Multus: Err in delegate conf: %v", err)
		}
	}

	if n.Kubeconfig == "" || nopodnet {
		if err := saveDelegates(args.ContainerID, n.CNIDir, n.Delegates); err != nil {
			return fmt.Errorf("Multus: Err in saving the delegates: %v", err)
		}
	}

	podifName := getifname()
	var mIndex int
	for index, delegate := range n.Delegates {
		err, r := delegateAdd(podifName, args.IfName, delegate, true)
		if err != true {
			result = r
			mIndex = index
		} else if (err != false) && r != nil {
			return r
		}
	}

	for index, delegate := range n.Delegates {
		err, r := delegateAdd(podifName, args.IfName, delegate, false)
		if err != true {
			result = r
		} else if (err != false) && r != nil {
			perr := clearPlugins(mIndex, index, args.IfName, n.Delegates)
			if perr != nil {
				return perr
			}
			return r
		}
	}

	return result
}

func cmdDel(args *skel.CmdArgs) error {
	var result error
	var nopodnet bool

	in, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if in.Kubeconfig != "" {
		podDelegate, r := getK8sNetwork(args, in.Kubeconfig)
		if r != nil && r.Error() == "nonet" {
			nopodnet = true
			if !defaultcninetwork {
				return fmt.Errorf("Multus: Err in getting k8s network from the poc spec, check the pod spec or set delegate for the default network, Refer the README.md: %v", r)
			}
		}

		if r != nil && !defaultcninetwork {
			return fmt.Errorf("Multus: Err in getting k8s network from pod: %v", r)
		}

		if len(podDelegate) != 0 {
			in.Delegates = podDelegate
		}
	}

	if in.Kubeconfig == "" || nopodnet {
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

	podifName := getifname()
	for _, delegate := range in.Delegates {
		r := delegateDel(podifName, args.IfName, delegate)
		if r != nil {
			return r
		}
		result = r
	}

	return result
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}
