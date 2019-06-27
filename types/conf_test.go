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
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"

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
		_, err = LoadDelegateNetConf([]byte(conf), nil, "")
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
			]},
			"delegates": [{
	        "thejohn": "weave-net"
			}],
		"runtimeConfig": {
	      "portMappings": [
	        {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
	      ]
	    }

	}`
		// missing replaced delegate field "type" with "thejohn"
		_, err := LoadNetConf([]byte(conf))
		Expect(err).To(HaveOccurred())

		// This part of the test is not working.
		// conf = `{
		//     "name": "node-cni-network",
		// 		"type": "multus",
		//     "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
		// 		"prevResult": {
		// 			"ips": [
		// 				{
		// 					"version": "4",
		// 					"address": "10.0.0.5/32",
		// 					"interface": 2
		// 				}
		// 		]},
		// 		"delegates": [{
		//       "name": "meme1"
		//   	}],
		// 	"runtimeConfig": {
		//       "portMappings": [
		//         {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
		//       ]
		//     }

		// }`
		// fmt.Printf("\n\n\n\n\n YA YEET \n\n\n\n\n")
		// _, err = LoadNetConf([]byte(conf))
		// fmt.Printf("\n\n\n\n\n YEET YA \n\n\n\n\n")
		// Expect(err).NotTo(HaveOccurred())
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
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0")
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
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.1")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &sriovConfList)
		Expect(err).NotTo(HaveOccurred())
		Expect(sriovConfList.Plugins[0].DeviceID).To(Equal("0000:00:00.1"))
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
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.2")
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
		delegateNetConf, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.3")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(delegateNetConf.Bytes, &hostDeviceConfList)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostDeviceConfList.Plugins[0].PCIBusID).To(Equal("0000:00:00.3"))
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

		k8sArgs := &K8sArgs{K8S_POD_NAME: "dummy", K8S_POD_NAMESPACE: "dummythicc", K8S_POD_INFRA_CONTAINER_ID: "123456789"}

		rc := &RuntimeConfig{}
		meme := make([]PortMapEntry, 2)
		rc.PortMaps = meme

		rc.PortMaps[0].HostPort = 0
		rc.PortMaps[0].ContainerPort = 1
		rc.PortMaps[0].Protocol = "sampleProtocol"
		rc.PortMaps[0].HostIP = "sampleHostIP"
		rc.PortMaps[1].HostPort = 1
		rc.PortMaps[1].ContainerPort = 2
		rc.PortMaps[1].Protocol = "anotherSampleProtocol"
		rc.PortMaps[1].HostIP = "anotherSampleHostIP"

		rt := CreateCNIRuntimeConf(args, k8sArgs, "", rc)
		fmt.Println("rt.ContainerID: ", rt.ContainerID)
		Expect(rt.ContainerID).To(Equal("123456789"))
		Expect(rt.NetNS).To(Equal(args.Netns))
		Expect(rt.IfName).To(Equal(""))
		Expect(rt.CapabilityArgs["portMappings"]).To(Equal(rc.PortMaps))
	})

	It("creates a network status from valid CNI result", func() {
		conf := `{
    "name": "second-network",
    "type": "host-device"
}`

		tmpResult := dfalkfjsaf
		tmpNetName := "sampleNetName"
		tmpMasterPlugin := true

		LoadNetworkStatus(tmpResult, tmpNetName, tmpMasterPlugin)
		err = json.Unmarshal(delegateNetConf.Bytes, &hostDeviceConf)
		Expect(err).NotTo(HaveOccurred())
		Expect(hostDeviceConf.PCIBusID).To(Equal("0000:00:00.2"))
	})

})
