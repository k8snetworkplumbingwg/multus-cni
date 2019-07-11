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
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	types020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	testhelpers "github.com/intel/multus-cni/testing"

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

		k8sArgs := &K8sArgs{K8S_POD_NAME: "dummy", K8S_POD_NAMESPACE: "namespacedummy", K8S_POD_INFRA_CONTAINER_ID: "123456789"}

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

		delegate, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0")
		Expect(err).NotTo(HaveOccurred())

		delegateNetStatus, err := LoadNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin)

		GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)

		Expect(err).NotTo(HaveOccurred())
	})

	It("cannot loadnetworkstatus given incompatible CNIVersion", func() {

		result := &Result{
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

		delegate, err := LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0")
		Expect(err).NotTo(HaveOccurred())
		fmt.Println("result.Version: ", result.Version())
		delegateNetStatus, err := LoadNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin)

		GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)

		Expect(err).To(HaveOccurred())
	})

})

type Result struct {
	CNIVersion string             `json:"cniVersion,omitempty"`
	IP4        *types020.IPConfig `json:"ip4,omitempty"`
	IP6        *types020.IPConfig `json:"ip6,omitempty"`
	DNS        types.DNS          `json:"dns,omitempty"`
}

func (r *Result) Version() string {
	return r.CNIVersion
}

func (r *Result) GetAsVersion(version string) (types.Result, error) {
	for _, supportedVersion := range types020.SupportedVersions {
		if version == supportedVersion {
			r.CNIVersion = version
			return r, nil
		}
	}
	return nil, fmt.Errorf("cannot convert version %q to %s", types020.SupportedVersions, version)
}

func (r *Result) Print() error {
	return r.PrintTo(os.Stdout)
}

func (r *Result) PrintTo(writer io.Writer) error {
	data, err := json.MarshalIndent(r, "", "    ")
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// String returns a formatted string in the form of "[IP4: $1,][ IP6: $2,] DNS: $3" where
// $1 represents the receiver's IPv4, $2 represents the receiver's IPv6 and $3 the
// receiver's DNS. If $1 or $2 are nil, they won't be present in the returned string.
func (r *Result) String() string {
	var str string
	if r.IP4 != nil {
		str = fmt.Sprintf("IP4:%+v, ", *r.IP4)
	}
	if r.IP6 != nil {
		str += fmt.Sprintf("IP6:%+v, ", *r.IP6)
	}
	return fmt.Sprintf("%sDNS:%+v", str, r.DNS)
}
