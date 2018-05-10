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

	k8s "github.com/Intel-Corp/multus-cni/k8sclient"
	"github.com/Intel-Corp/multus-cni/types"
	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/vishvananda/netlink"
)

const (
	defaultCNIDir  = "/var/lib/cni/multus"
	defaultConfDir = "/etc/cni/multus/net.d"
)

var masterpluginEnabled bool
var defaultcninetwork bool

//taken from cni/plugins/meta/flannel/flannel.go
func isString(i interface{}) bool {
	_, ok := i.(string)
	return ok
}

func isBool(i interface{}) bool {
	_, ok := i.(bool)
	return ok
}

func loadNetConf(bytes []byte) (*types.NetConf, error) {
	netconf := &types.NetConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	if netconf.Kubeconfig != "" && netconf.Delegates != nil {
		defaultcninetwork = true
	}

	if netconf.CNIDir == "" {
		netconf.CNIDir = defaultCNIDir
	}

	if netconf.ConfDir == "" {
		netconf.ConfDir = defaultConfDir
	}

	if netconf.Kubeconfig == "" || !defaultcninetwork {
		return nil, fmt.Errorf(`You must also set the delegates & the kubeconfig, refer to the README`)
	}

	if len(netconf.Delegates) == 0 && !defaultcninetwork {
		return nil, fmt.Errorf(`delegates or kubeconfig option is must, refer README.md`)
	}

	// default network in multus conf as master plugin
	netconf.Delegates[0]["masterplugin"] = true

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

func validateIfName(nsname string, ifname string) error {
	podNs, err := ns.GetNS(nsname)
	if err != nil {
		return fmt.Errorf("no netns: %v", err)
	}

	err = podNs.Do(func(_ ns.NetNS) error {
		_, err := netlink.LinkByName(ifname)
		if err != nil {
			if err.Error() == "Link not found" {
				return nil
			}
			return err
		}
		return fmt.Errorf("ifname %s is already exist", ifname)
	})

	return err
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

	if netconf["ifnameRequest"] != nil {
		if os.Setenv("CNI_IFNAME", netconf["ifnameRequest"].(string)) != nil {
			return true, fmt.Errorf("Multus: error in setting CNI_IFNAME")
		}
	}

	err = validateIfName(os.Getenv("CNI_NETNS"), os.Getenv("CNI_IFNAME"))
	if err != nil {
		return true, fmt.Errorf("cannot set %q ifname: %v", netconf["type"].(string), err)
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

func cmdAdd(args *skel.CmdArgs) error {
	var result error
	var nopodnet bool
	n, err := loadNetConf(args.StdinData)
	if err != nil {
		return fmt.Errorf("err in loading netconf: %v", err)
	}

	if n.Kubeconfig != "" {
		podDelegate, err := k8s.GetK8sNetwork(args, n.Kubeconfig, n.ConfDir)
		if err != nil {
			if _, ok := err.(*k8s.NoK8sNetworkError); ok {
				nopodnet = true
				if !defaultcninetwork {
					return fmt.Errorf("Multus: Err in getting k8s network from the pod spec annotation, check the pod spec or set delegate for the default network, Refer the README.md: %v", err)
				}
			} else if !defaultcninetwork {
				return fmt.Errorf("Multus: Err in getting k8s network from pod: %v", err)
			}
		}

		// If it's empty just leave it as the netconfig states (e.g. just default)
		if len(podDelegate) != 0 {
			// In the case that we force the default
			// We add the found configs from CRD
			for _, eachDelegate := range podDelegate {
				eachDelegate["masterplugin"] = false
				n.Delegates = append(n.Delegates, eachDelegate)
			}
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
		podDelegate, r := k8s.GetK8sNetwork(args, in.Kubeconfig, in.ConfDir)
		if r != nil {
			if _, ok := r.(*k8s.NoK8sNetworkError); ok {
				nopodnet = true
				// no network found from default and annotaed network,
				// we do nothing to remove network for the pod!
				if !defaultcninetwork {
					return fmt.Errorf("Multus: Err in getting k8s network from the poc spec, check the pod spec or set delegate for the default network, Refer the README.md: %v", r)
				}
			} else {
				return fmt.Errorf("Multus: Err in getting k8s network from pod: %v", r)
			}
		}

		if len(podDelegate) != 0 {
			// In the case that we force the default
			// We add the found configs from CRD (in reverse order)
			for i := len(podDelegate) - 1; i >= 0; i-- {
				podDelegate[i]["masterplugin"] = false
				in.Delegates = append(in.Delegates, podDelegate[i])
			}
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
