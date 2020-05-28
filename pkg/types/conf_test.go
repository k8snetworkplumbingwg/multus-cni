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
//

package types

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	types020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	testhelpers "gopkg.in/intel/multus-cni.v3/pkg/testing"
	netutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestConf(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "conf")
}

var _ = Describe("config operations", func() {
	var testNS ns.NetNS
	var tmpDir string

	BeforeEach(func() {
		// Create a new NetNS so we don't modify the host
		var err error
		testNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("CNI_NETNS", testNS.Path())
		os.Setenv("CNI_PATH", "/some/path")

		tmpDir, err = ioutil.TempDir("", "multus_tmp")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(testNS.Close()).To(Succeed())
		os.Unsetenv("CNI_PATH")
		os.Unsetenv("CNI_ARGS")
		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("parses a valid multus configuration", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "type": "weave-net"
    }],
	"runtimeConfig": {
      "portMappings": [
        {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
      ]
    }

}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("weave-net"))
		Expect(netConf.Delegates[0].MasterPlugin).To(BeTrue())
		Expect(len(netConf.RuntimeConfig.PortMaps)).To(Equal(1))
	})

	It("fails to load invalid multus configuration (bad json)", func() {
		conf := `{
	  "name": "node-cni-network",
	  "type": "multus",
	  "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	  "delegates": [{
	      "type": "weave-net"
	  }],
	"runtimeConfig": {
	    "portMappings": [
	      {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
	    ]
		}`
		// Error in conf json: missing end bracket

		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())
		_, err = LoadDelegateNetConf([]byte(conf), nil, "", "")
		Expect(err).To(HaveOccurred())
		err = LoadDelegateNetConfList([]byte(conf), &DelegateNetConf{})
		Expect(err).To(HaveOccurred())
		_, err = addDeviceIDInConfList([]byte(conf), "")
		Expect(err).To(HaveOccurred())
		_, err = delegateAddDeviceID([]byte(conf), "")
		Expect(err).To(HaveOccurred())
	})

	It("checks if logFile and logLevel are set correctly", func() {
		conf := `{
	    "name": "node-cni-network",
			"type": "multus",
			"logLevel": "debug",
			"logFile": "/var/log/multus.log",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "delegates": [{
	        "type": "weave-net"
	    }],
		"runtimeConfig": {
	      "portMappings": [
	        {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
	      ]
	    }

	}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(netConf.LogLevel).To(Equal("debug"))
		Expect(netConf.LogFile).To(Equal("/var/log/multus.log"))
	})

	It("properly sets namespace isolation using the default namespace", func() {
		conf := `{
	    "name": "node-cni-network",
	    "type": "multus",
	    "logLevel": "debug",
	    "logFile": "/var/log/multus.log",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "namespaceIsolation": true,
	    "delegates": [{
	        "type": "weave-net"
	    }]
	}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(netConf.NamespaceIsolation).To(Equal(true))
		Expect(len(netConf.NonIsolatedNamespaces)).To(Equal(1))
		Expect(netConf.NonIsolatedNamespaces[0]).To(Equal("default"))
	})

	It("properly sets namespace isolation using custom namespaces", func() {
		conf := `{
	    "name": "node-cni-network",
	    "type": "multus",
	    "logLevel": "debug",
	    "logFile": "/var/log/multus.log",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "namespaceIsolation": true,
	    "globalNamespaces": " foo,bar ,default",
	    "delegates": [{
	        "type": "weave-net"
	    }]
	}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(netConf.NamespaceIsolation).To(Equal(true))
		Expect(len(netConf.NonIsolatedNamespaces)).To(Equal(3))
		Expect(netConf.NonIsolatedNamespaces[0]).To(Equal("foo"))
		Expect(netConf.NonIsolatedNamespaces[1]).To(Equal("bar"))
		Expect(netConf.NonIsolatedNamespaces[2]).To(Equal("default"))
	})

	It("prevResult with no errors", func() {
		conf := `{
	    "name": "node-cni-network",
			"type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
			"prevResult": {
				"ips": [
					{
						"version": "4",
						"address": "10.0.0.5/32",
						"interface": 2
					}
			]},
			"delegates": [{
	        "type": "weave-net"
			}],
		"runtimeConfig": {
	      "portMappings": [
	        {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
	      ]
	    }

	}`
		_, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
	})

	It("succeeds if only delegates are set", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "delegates": [{
        "type": "weave-net"
    },{
        "type": "foobar"
    }]
}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(2))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("weave-net"))
		Expect(netConf.Delegates[0].MasterPlugin).To(BeTrue())
		Expect(netConf.Delegates[1].Conf.Type).To(Equal("foobar"))
		Expect(netConf.Delegates[1].MasterPlugin).To(BeFalse())
	})

	It("fails if no kubeconfig or delegates are set", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus"
}`
		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())
	})

	It("fails if kubeconfig is present but no delegates are set", func() {
		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml"
}`
		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())
	})

	It("fails when delegate field exists but fields are named incorrectly", func() {
		conf := `{
	"name": "node-cni-network",
		"type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
		"prevResult": {
			"ips": [
				{
					"version": "4",
					"address": "10.0.0.5/32",
					"interface": 2
				}
			]
		},
		"delegates": [{
			"_not_type": "weave-net"
		}],
	"runtimeConfig": {
		"portMappings": [
			{"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
	    ]
	}
}`
		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())
	})

	It("has defaults set for network readiness", func() {
		conf := `{
    "name": "defaultnetwork",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/kubelet.conf",
    "delegates": [{
      "cniVersion": "0.3.0",
      "name": "defaultnetwork",
      "type": "flannel",
      "isDefaultGateway": true
    }]
}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(netConf.ReadinessIndicatorFile).To(Equal(""))
	})

	It("honors overrides for network readiness", func() {
		conf := `{
    "name": "defaultnetwork",
    "type": "multus",
    "readinessindicatorfile": "/etc/cni/net.d/foo",
    "kubeconfig": "/etc/kubernetes/kubelet.conf",
    "delegates": [{
      "cniVersion": "0.3.0",
      "name": "defaultnetwork",
      "type": "flannel",
      "isDefaultGateway": true
    }]
}`
		netConf, err := LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())
		Expect(netConf.ReadinessIndicatorFile).To(Equal("/etc/cni/net.d/foo"))
	})

	It("check CheckSystemNamespaces() works fine", func() {
		b1 := CheckSystemNamespaces("foobar", []string{"barfoo", "bafoo", "foobar"})
		Expect(b1).To(Equal(true))
		b2 := CheckSystemNamespaces("foobar1", []string{"barfoo", "bafoo", "foobar"})
		Expect(b2).To(Equal(false))
	})

	It("assigns deviceID in delegated conf", func() {
		conf := `{
    "name": "second-network",
    "type": "sriov"
}`
		type sriovNetConf struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			DeviceID string `json:"deviceID"`
		}
		sriovConf := &sriovNetConf{}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0", "")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &sriovConf)
		Expect(err).NotTo(HaveOccurred())
		Expect(sriovConf.DeviceID).To(Equal("0000:00:00.0"))
	})

	It("assigns deviceID in delegated conf list", func() {
		conf := `{
    "name": "second-network",
    "plugins": [
      {
        "type": "sriov"
      }
    ]
}`
		type sriovNetConf struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			DeviceID string `json:"deviceID"`
		}
		type sriovNetConfList struct {
			Plugins []*sriovNetConf `json:"plugins"`
		}
		sriovConfList := &sriovNetConfList{}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.1", "")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &sriovConfList)
		Expect(err).NotTo(HaveOccurred())
		Expect(sriovConfList.Plugins[0].DeviceID).To(Equal("0000:00:00.1"))
	})

	It("assigns deviceID in delegated conf list multiple plugins", func() {
		conf := `{
    "name": "second-network",
    "plugins": [
      {
        "type": "sriov"
      },
      {
        "type": "other-cni"
      }
    ]
}`
		type sriovNetConf struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			DeviceID string `json:"deviceID"`
		}
		type sriovNetConfList struct {
			Plugins []*sriovNetConf `json:"plugins"`
		}
		sriovConfList := &sriovNetConfList{}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.1", "")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &sriovConfList)
		Expect(err).NotTo(HaveOccurred())
		for _, plugin := range sriovConfList.Plugins {
			Expect(plugin.DeviceID).To(Equal("0000:00:00.1"))
		}
	})

	It("assigns pciBusID in delegated conf", func() {
		conf := `{
    "name": "second-network",
    "type": "host-device"
}`
		type hostDeviceNetConf struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			PCIBusID string `json:"pciBusID"`
		}
		hostDeviceConf := &hostDeviceNetConf{}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.2", "")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &hostDeviceConf)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostDeviceConf.PCIBusID).To(Equal("0000:00:00.2"))
	})

	It("assigns pciBusID in delegated conf list", func() {
		conf := `{
    "name": "second-network",
    "plugins": [
      {
        "type": "host-device"
      }
    ]
}`
		type hostDeviceNetConf struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			PCIBusID string `json:"pciBusID"`
		}
		type hostDeviceNetConfList struct {
			Plugins []*hostDeviceNetConf `json:"plugins"`
		}
		hostDeviceConfList := &hostDeviceNetConfList{}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.3", "")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &hostDeviceConfList)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostDeviceConfList.Plugins[0].PCIBusID).To(Equal("0000:00:00.3"))
	})

	It("assigns pciBusID in delegated conf list multiple plugins", func() {
		conf := `{
    "name": "second-network",
    "plugins": [
      {
        "type": "host-device"
      },
      {
        "type": "other-cni"
      }
    ]
}`
		type hostDeviceNetConf struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			PCIBusID string `json:"pciBusID"`
		}
		type hostDeviceNetConfList struct {
			Plugins []*hostDeviceNetConf `json:"plugins"`
		}
		hostDeviceConfList := &hostDeviceNetConfList{}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.3", "")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &hostDeviceConfList)
		Expect(err).NotTo(HaveOccurred())
		for _, plugin := range hostDeviceConfList.Plugins {
			Expect(plugin.PCIBusID).To(Equal("0000:00:00.3"))
		}
	})

	It("add cni-args in config", func() {
		var args map[string]interface{}
		conf := `{
    "name": "second-network",
    "type": "bridge"
}`
		cniArgs := `{
    "args1": "val1"
}`
		type bridgeNetConf struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Args struct {
				CNI map[string]string `json:"cni"`
			} `json:"args"`
		}

		err := json.Unmarshal([]byte(cniArgs), &args)
		Expect(err).NotTo(HaveOccurred())
		net := &NetworkSelectionElement{
			Name:    "test-elem",
			CNIArgs: &args,
		}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), net, "", "")
		Expect(err).NotTo(HaveOccurred())
		bridgeConf := &bridgeNetConf{}
		err = json.Unmarshal(delegateNetConf.Bytes, bridgeConf)
		Expect(bridgeConf.Args.CNI["args1"]).To(Equal("val1"))
	})

	It("add cni-args in config which has cni args already (merge case)", func() {
		var args map[string]interface{}
		conf := `{
    "name": "second-network",
    "type": "bridge",
    "args": {
       "cni": {
         "args0": "val0",
         "args1": "val1"
       }
    }
}`
		cniArgs := `{
    "args1": "val1a"
}`
		type bridgeNetConf struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Args struct {
				CNI map[string]string `json:"cni"`
			} `json:"args"`
		}

		err := json.Unmarshal([]byte(cniArgs), &args)
		Expect(err).NotTo(HaveOccurred())
		net := &NetworkSelectionElement{
			Name:    "test-elem",
			CNIArgs: &args,
		}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), net, "", "")
		Expect(err).NotTo(HaveOccurred())
		bridgeConf := &bridgeNetConf{}
		err = json.Unmarshal(delegateNetConf.Bytes, bridgeConf)
		Expect(bridgeConf.Args.CNI["args0"]).To(Equal("val0"))
		Expect(bridgeConf.Args.CNI["args1"]).To(Equal("val1a"))
	})

	It("add cni-args in conflist", func() {
		var args map[string]interface{}
		conf := `{
    "name": "second-network",
    "plugins": [
      {
        "type": "bridge"
      }
    ]
}`
		cniArgs := `{
    "args1": "val1"
}`
		type bridgeNetConf struct {
			Type string `json:"type"`
			Args struct {
				CNI map[string]string `json:"cni"`
			} `json:"args"`
		}
		type bridgeNetConfList struct {
			Name    string           `json:"name"`
			Plugins []*bridgeNetConf `json:"plugins"`
		}

		err := json.Unmarshal([]byte(cniArgs), &args)
		Expect(err).NotTo(HaveOccurred())
		net := &NetworkSelectionElement{
			Name:    "test-elem",
			CNIArgs: &args,
		}
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), net, "", "")
		Expect(err).NotTo(HaveOccurred())
		bridgeConflist := &bridgeNetConfList{}
		err = json.Unmarshal(delegateNetConf.Bytes, bridgeConflist)
		Expect(bridgeConflist.Plugins[0].Args.CNI["args1"]).To(Equal("val1"))
	})

	It("creates a valid CNI runtime config", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
    "name": "node-cni-network",
    "type": "multus",
    "defaultnetworkfile": "/tmp/foo.multus.conf",
    "defaultnetworkwaitseconds": 3,
    "delegates": [{
        "name": "weave1",
        "cniVersion": "0.2.0",
        "type": "weave-net"
    },{
        "name": "other1",
        "cniVersion": "0.2.0",
        "type": "other-plugin"
    }]
}`),
		}

		k8sArgs := &K8sArgs{K8S_POD_NAME: "dummy", K8S_POD_NAMESPACE: "namespacedummy", K8S_POD_INFRA_CONTAINER_ID: "123456789"}

		rc := &RuntimeConfig{}
		rc.PortMaps = make([]*PortMapEntry, 2)

		rc.PortMaps[0] = &PortMapEntry{
			HostPort:      0,
			ContainerPort: 1,
			Protocol:      "sampleProtocol",
			HostIP:        "sampleHostIP",
		}
		rc.PortMaps[1] = &PortMapEntry{
			HostPort:      1,
			ContainerPort: 2,
			Protocol:      "anotherSampleProtocol",
			HostIP:        "anotherSampleHostIP",
		}

		rt, _ := CreateCNIRuntimeConf(args, k8sArgs, "", rc, nil)
		fmt.Println("rt.ContainerID: ", rt.ContainerID)
		Expect(rt.ContainerID).To(Equal("123456789"))
		Expect(rt.NetNS).To(Equal(args.Netns))
		Expect(rt.IfName).To(Equal(""))
		Expect(rt.CapabilityArgs["portMappings"]).To(Equal(rc.PortMaps))
	})

	It("can loadnetworkstatus", func() {
		result := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}

		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "type": "weave-net"
    }],
  "runtimeConfig": {
      "portMappings": [
        {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
      ]
    }
}`

		delegate, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0", "")
		Expect(err).NotTo(HaveOccurred())

		delegateNetStatus, err := netutils.CreateNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin, nil)

		GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)

		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot loadnetworkstatus given incompatible CNIVersion", func() {

		result := &testhelpers.Result{
			CNIVersion: "1.2.3",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}

		conf := `{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "type": "weave-net"
    }],
  "runtimeConfig": {
      "portMappings": [
        {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
      ]
    }
}`

		delegate, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0", "")
		Expect(err).NotTo(HaveOccurred())
		fmt.Println("result.Version: ", result.Version())
		delegateNetStatus, err := netutils.CreateNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin, nil)

		GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)

		Expect(err).To(HaveOccurred())
	})

	It("verify the network selection elements goes into delegateconf", func() {
		cniConfig := `{
        "name": "weave1",
        "cniVersion": "0.2.0",
        "type": "weave-net"
    }`
		bandwidthEntry1 := &BandwidthEntry{
			IngressRate:  100,
			IngressBurst: 200,
			EgressRate:   100,
			EgressBurst:  200,
		}

		portMapEntry1 := &PortMapEntry{
			HostPort:      8080,
			ContainerPort: 80,
			Protocol:      "tcp",
			HostIP:        "10.0.0.1",
		}

		networkSelection := &NetworkSelectionElement{
			Name:                  "testname",
			InterfaceRequest:      "testIF1",
			MacRequest:            "c2:11:22:33:44:66",
			InfinibandGUIDRequest: "24:8a:07:03:00:8d:ae:2e",
			IPRequest:             []string{"10.0.0.1/24"},
			BandwidthRequest:      bandwidthEntry1,
			PortMappingsRequest:   []*PortMapEntry{portMapEntry1},
		}

		delegateConf, err := LoadDelegateNetConf([]byte(cniConfig), networkSelection, "", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(delegateConf.IfnameRequest).To(Equal(networkSelection.InterfaceRequest))
		Expect(delegateConf.MacRequest).To(Equal(networkSelection.MacRequest))
		Expect(delegateConf.InfinibandGUIDRequest).To(Equal(networkSelection.InfinibandGUIDRequest))
		Expect(delegateConf.IPRequest).To(Equal(networkSelection.IPRequest))
		Expect(delegateConf.BandwidthRequest).To(Equal(networkSelection.BandwidthRequest))
		Expect(delegateConf.PortMappingsRequest).To(Equal(networkSelection.PortMappingsRequest))
	})

	It("test mergeCNIRuntimeConfig with masterPlugin", func() {
		conf := `{
			"name": "node-cni-network",
			"type": "multus",
			"kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
			"delegates": [{
				"type": "weave-net"
			}],
		  "runtimeConfig": {
			  "portMappings": [
				{"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			  ]
			}
		}`
		bandwidthEntry1 := &BandwidthEntry{
			IngressRate:  100,
			IngressBurst: 200,
			EgressRate:   100,
			EgressBurst:  200,
		}
		portMapEntry1 := &PortMapEntry{
			HostPort:      8080,
			ContainerPort: 80,
			Protocol:      "tcp",
			HostIP:        "10.0.0.1",
		}

		networkSelection := &NetworkSelectionElement{
			Name:                  "testname",
			InterfaceRequest:      "testIF1",
			MacRequest:            "c2:11:22:33:44:66",
			InfinibandGUIDRequest: "24:8a:07:03:00:8d:ae:2e",
			IPRequest:             []string{"10.0.0.1/24"},
			BandwidthRequest:      bandwidthEntry1,
			PortMappingsRequest:   []*PortMapEntry{portMapEntry1},
		}
		delegate, err := LoadDelegateNetConf([]byte(conf), networkSelection, "", "")
		delegate.MasterPlugin = true
		Expect(err).NotTo(HaveOccurred())
		runtimeConf := mergeCNIRuntimeConfig(&RuntimeConfig{}, delegate)
		Expect(runtimeConf.PortMaps).To(BeNil())
		Expect(runtimeConf.Bandwidth).To(BeNil())
		Expect(runtimeConf.InfinibandGUID).To(Equal(""))
	})

	It("test mergeCNIRuntimeConfig with delegate plugin", func() {
		conf := `{
			"name": "weave1",
			"cniVersion": "0.2.0",
			"type": "weave-net"
		}`
		bandwidthEntry1 := &BandwidthEntry{
			IngressRate:  100,
			IngressBurst: 200,
			EgressRate:   100,
			EgressBurst:  200,
		}
		portMapEntry1 := &PortMapEntry{
			HostPort:      8080,
			ContainerPort: 80,
			Protocol:      "tcp",
			HostIP:        "10.0.0.1",
		}

		networkSelection := &NetworkSelectionElement{
			Name:                  "testname",
			InterfaceRequest:      "testIF1",
			MacRequest:            "c2:11:22:33:44:66",
			InfinibandGUIDRequest: "24:8a:07:03:00:8d:ae:2e",
			IPRequest:             []string{"10.0.0.1/24"},
			BandwidthRequest:      bandwidthEntry1,
			PortMappingsRequest:   []*PortMapEntry{portMapEntry1},
		}
		delegate, err := LoadDelegateNetConf([]byte(conf), networkSelection, "", "")
		Expect(err).NotTo(HaveOccurred())
		runtimeConf := mergeCNIRuntimeConfig(&RuntimeConfig{}, delegate)
		Expect(runtimeConf.PortMaps).NotTo(BeNil())
		Expect(len(runtimeConf.PortMaps)).To(BeEquivalentTo(1))
		Expect(runtimeConf.PortMaps[0]).To(Equal(portMapEntry1))
		Expect(runtimeConf.Bandwidth).To(Equal(bandwidthEntry1))
		Expect(len(runtimeConf.IPs)).To(BeEquivalentTo(1))
		Expect(runtimeConf.Mac).To(Equal("c2:11:22:33:44:66"))
		Expect(runtimeConf.InfinibandGUID).To(Equal("24:8a:07:03:00:8d:ae:2e"))
	})
})
