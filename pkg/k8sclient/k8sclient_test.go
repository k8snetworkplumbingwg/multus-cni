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

package k8sclient

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	types020 "github.com/containernetworking/cni/pkg/types/020"
	testutils "gopkg.in/intel/multus-cni.v3/pkg/testing"

	"github.com/containernetworking/cni/pkg/skel"
	"gopkg.in/intel/multus-cni.v3/pkg/types"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	netfake "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/fake"
	netutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"

	"k8s.io/client-go/kubernetes/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestK8sClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "k8sclient")
}

// NewFakeClientInfo returns fake client (just for testing)
func NewFakeClientInfo() *ClientInfo {
	return &ClientInfo{
		Client:    fake.NewSimpleClientset(),
		NetClient: netfake.NewSimpleClientset().K8sCniCncfIoV1(),
	}
}

var _ = Describe("k8sclient operations", func() {
	var tmpDir string
	var err error
	var genericConf string

	BeforeEach(func() {
		tmpDir, err = ioutil.TempDir("", "multus_tmp")
		Expect(err).NotTo(HaveOccurred())
		genericConf = `{
			"name":"node-cni-network",
			"type":"multus",
			"delegates": [{
			"name": "weave1",
				"cniVersion": "0.2.0",
				"type": "weave-net"
			}],
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
	})

	AfterEach(func() {
		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	It("retrieves delegates from kubernetes using simple format annotation", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
		net1 := `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`
		net2 := `{
	"name": "net2",
	"type": "mynet2",
	"cniVersion": "0.2.0"
}`
		net3 := `{
	"name": "net3",
	"type": "mynet3",
	"cniVersion": "0.2.0"
}`

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net2", net2))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net3", net3))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		Expect(err).NotTo(HaveOccurred())
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(delegates)).To(Equal(2))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
		Expect(delegates[0].MasterPlugin).To(BeFalse())
		Expect(delegates[1].Conf.Name).To(Equal("net2"))
		Expect(delegates[1].Conf.Type).To(Equal("mynet2"))
		Expect(delegates[1].MasterPlugin).To(BeFalse())
	})

	It("fails when the network does not exist", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
		net3 := `{
	"name": "net3",
	"type": "mynet3",
	"cniVersion": "0.2.0"
}`
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net3", net3))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		Expect(err).NotTo(HaveOccurred())
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(len(delegates)).To(Equal(0))
		Expect(err).To(MatchError("GetNetworkDelegates: failed getting the delegate: getKubernetesDelegate: cannot find a network-attachment-definition (net1) in namespace (test): network-attachment-definitions.k8s.cni.cncf.io \"net1\" not found"))
	})

	It("retrieves delegates from kubernetes using JSON format annotation", func() {
		fakePod := testutils.NewFakePod("testpod", `[
{"name":"net1"},
{
  "name":"net2",
  "ipRequest": "1.2.3.4",
  "macRequest": "aa:bb:cc:dd:ee:ff"
},
{
  "name":"net3",
  "namespace":"other-ns"
}
]`, "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net2", `{
			"name": "net2",
			"type": "mynet2",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("other-ns", "net3", `{
			"name": "net3",
			"type": "mynet3",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(delegates)).To(Equal(3))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
		Expect(delegates[1].Conf.Name).To(Equal("net2"))
		Expect(delegates[1].Conf.Type).To(Equal("mynet2"))
		Expect(delegates[2].Conf.Name).To(Equal("net3"))
		Expect(delegates[2].Conf.Type).To(Equal("mynet3"))
	})

	It("fails when the JSON format annotation is invalid", func() {
		fakePod := testutils.NewFakePod("testpod", "[adsfasdfasdfasf]", "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(len(networks)).To(Equal(0))
		Expect(err).To(MatchError("parsePodNetworkAnnotation: failed to parse pod Network Attachment Selection Annotation JSON format: invalid character 'a' looking for beginning of value"))
	})

	It("can set the default-gateway on an additional interface", func() {
		fakePod := testutils.NewFakePod("testpod", `[
{"name":"net1"},
{
  "name":"net2",
  "default-route": ["192.168.2.2"]
},
{
  "name":"net3",
  "namespace":"other-ns"
}
]`, "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net2", `{
			"name": "net2",
			"type": "mynet2",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("other-ns", "net3", `{
			"name": "net3",
			"type": "mynet3",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(delegates)).To(Equal(3))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
		Expect(delegates[1].Conf.Name).To(Equal("net2"))
		Expect(delegates[1].Conf.Type).To(Equal("mynet2"))
		Expect(delegates[2].Conf.Name).To(Equal("net3"))
		Expect(delegates[2].Conf.Type).To(Equal("mynet3"))
	})

	It("retrieves delegates from kubernetes using on-disk config files", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDefFile(fakePod.ObjectMeta.Namespace, "net1", net1Name, `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())

		net2Name := filepath.Join(tmpDir, "20-net2.conf")
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDefFile(fakePod.ObjectMeta.Namespace, "net2", net2Name, `{
			"name": "net2",
			"type": "mynet2",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(delegates)).To(Equal(2))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
		Expect(delegates[1].Conf.Name).To(Equal("net2"))
		Expect(delegates[1].Conf.Type).To(Equal("mynet2"))
	})

	It("injects network name into minimal thick plugin CNI config", func() {
		fakePod := testutils.NewFakePod("testpod", "net1", "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", "{\"type\": \"mynet\"}"))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(len(delegates)).To(Equal(1))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("fails when on-disk config file is not valid", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDefFile(fakePod.ObjectMeta.Namespace, "net1", net1Name, `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`))
		Expect(err).NotTo(HaveOccurred())
		net2Name := filepath.Join(tmpDir, "20-net2.conf")
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDefFile(fakePod.ObjectMeta.Namespace, "net2", net2Name, "asdfasdfasfdasfd"))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		netConf, err := types.LoadNetConf([]byte(genericConf))
		netConf.ConfDir = tmpDir
		delegates, err := GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)
		Expect(len(delegates)).To(Equal(0))
		Expect(err).To(MatchError(fmt.Sprintf("GetNetworkDelegates: failed getting the delegate: GetCNIConfig: err in GetCNIConfigFromFile: Error loading CNI config file %s: error parsing configuration: invalid character 'a' looking for beginning of value", net2Name)))
	})

	It("retrieves cluster network from CRD", func() {
		fakePod := testutils.NewFakePod("testpod", "", "")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "myCRD1",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "myCRD1", "{\"type\": \"mynet\"}"))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("myCRD1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("retrieves default networks from CRD", func() {
		fakePod := testutils.NewFakePod("testpod", "", "")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "myCRD1",
			"defaultNetworks": ["myCRD2"],
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "myCRD1", "{\"type\": \"mynet\"}"))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "myCRD2", "{\"type\": \"mynet2\"}"))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(2))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("myCRD1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet"))
		Expect(netConf.Delegates[1].Conf.Name).To(Equal("myCRD2"))
		Expect(netConf.Delegates[1].Conf.Type).To(Equal("mynet2"))
	})

	It("ignore default networks from CRD in case of kube-system namespace", func() {
		fakePod := testutils.NewFakePod("testpod", "", "")
		// overwrite namespace
		fakePod.ObjectMeta.Namespace = "kube-system"
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "myCRD1",
			"defaultNetworks": ["myCRD2"],
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "myCRD1", "{\"type\": \"mynet\"}"))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "myCRD2", "{\"type\": \"mynet2\"}"))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("myCRD1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("retrieves cluster network from file", func() {
		fakePod := testutils.NewFakePod("testpod", "", "")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "myFile1",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		netConf.ConfDir = tmpDir
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDefFile(fakePod.ObjectMeta.Namespace, "net1", net1Name, `{
				"name": "myFile1",
				"type": "mynet",
				"cniVersion": "0.2.0"
			}`))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("myFile1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("retrieves cluster network from path", func() {
		fakePod := testutils.NewFakePod("testpod", "", "")
		conf := fmt.Sprintf(`{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "%s",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`, tmpDir)
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDefFile(fakePod.ObjectMeta.Namespace, "10-net1", net1Name, `{
				"name": "net1",
				"type": "mynet",
				"cniVersion": "0.2.0"
			}`))
		Expect(err).NotTo(HaveOccurred())
		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("Error in case of CRD not found", func() {
		fakePod := testutils.NewFakePod("testpod", "", "")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "myCRD1",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).To(HaveOccurred())
	})

	It("overwrite cluster network when Pod annotation is set", func() {
		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "net2",
			"multusNamespace" : "kube-system",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "net1", "{\"type\": \"mynet1\"}"))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "net2", "{\"type\": \"mynet2\"}"))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net2"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet2"))

		numK8sDelegates, _, err := TryLoadPodDelegates(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(numK8sDelegates).To(Equal(0))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet1"))
	})

	It("fails with bad confdir", func() {
		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"clusterNetwork": "net2",
			"multusNamespace" : "kube-system",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", ""))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net2", ""))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
		Expect(err).To(HaveOccurred())

		netConf.ConfDir = "badfilepath"
		_, _, err = TryLoadPodDelegates(fakePod, netConf, clientInfo, nil)
		Expect(err).To(HaveOccurred())
	})

	It("overwrite multus config when Pod annotation is set", func() {

		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml",
			"delegates": [{
				"type": "mynet2",
				"name": "net2"
			}]
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net2"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet2"))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "net1", "{\"type\": \"mynet1\"}"))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		numK8sDelegates, _, err := TryLoadPodDelegates(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(numK8sDelegates).To(Equal(0))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet1"))
	})

	It("fails with no kubeclient and invalid kubeconfig", func() {
		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml",
			"delegates": [{
				"type": "mynet2",
				"name": "net2"
			}]
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net2"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet2"))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "net1", "{\"type\": \"mynet1\"}"))
		Expect(err).NotTo(HaveOccurred())

		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = TryLoadPodDelegates(fakePod, netConf, nil, nil)
		Expect(err).To(HaveOccurred())
	})

	It("fails with no kubeclient and no kubeconfig", func() {
		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"kubeconfig":"",
			"delegates": [{
				"type": "mynet2",
				"name": "net2"
			}]
		}`
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net2"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet2"))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "net1", "{\"type\": \"mynet1\"}"))
		Expect(err).NotTo(HaveOccurred())
		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = TryLoadPodDelegates(fakePod, netConf, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		// additionally, we expect the test to fail with no delegates, as at least one is always required.
		netConf.Delegates = nil
		_, _, err = TryLoadPodDelegates(fakePod, netConf, nil, nil)
		Expect(err).To(HaveOccurred())
	})

	It("uses cached delegates when an error in loading from pod annotation occurs", func() {
		dir, err := ioutil.TempDir("", "multus-test")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(dir) // clean up

		kubeletconf, err := os.Create(fmt.Sprintf("%s/kubelet.conf", dir))
		kubeletconfDef := `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5RENDQWJDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1EVXpNVEUxTVRRME1Gb1hEVEk1TURVeU9ERTFNVFEwTUZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBS3pLCnZQSHNxQWpMSHdxbDdMaDdQeTJuSndEdzAwTU4rZjFVTWtIS1BrOTVsTVRVRmgrMTVob05pWVJjaGt2d3VTRXMKRXNLYTJBRXpVeCtxR3hqTXptb01RdklMamNLbVdUS3ViNXZvZ29WV3l0N2ZzaW1jUlk4c2hWbmFSQ1pQZnptYgpUTVVRcFNHbUc5WWNGNlJhMnhwbTJ6Qm5aVWM0QjN6M1UxeHE3b1NRREp5ZVRna3VEZDB3SGhLay9tYWREYlgyCk5DU2ROVjRoUEFJcWY0MVZWc0hGV3c4WWpOUGllMVZBdWZQZDBRQlJuVFgraW50dm9weVpuWnJoVFJzVU5CanMKRWc0UGFEVXRMZDZsSUxqUnhEN2RkL1JYUWZaQ1E5azI3b0t1eU5RZUJMaWdhb052M3JPeExPWGxMaU1HUGZpVgpPTktxencwaGFDUlF1cHdoUlFVQ0F3RUFBYU1qTUNFd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0RRWUpLb1pJaHZjTkFRRUxCUUFEZ2dFQkFGU0MrYmJHaENZTyszOXA2QVJtOFBYbnZFTmIKTjdNd2VWSXVGazh3T3c2RWlHOTllZTJIb25KOCtxaEFLNXlCNVU0d3FEM2FkY0hNTHBGVUoxMVMrSVYra2hCYgo0SGYrcEtVZEJxM3MvYXAxMmppdUNZMUVDanpIVjZ5SDZLRGwrZEdibDR1dVJ6S016SkZteFpncEl4TUVqbEZ0CnByK2MzQmdaWWVVYlpTdDFhR2VObFlvdGxKZEFBbWlZcmkxcFAzOUxzb2xZbWJEOTNVb1NTYnhoR2lkSzBjNHcKbHV5aUxJQWJUVnJxMEo1UllCa05ZOGdMVTBRZVFJaVZTYWhkc3cvakgxL0dZY2J4alN6R1ZwOTAwb1ZMWUpsQQo5SkhVaUlTb3VHVTE1QVRpMklvN0tOa2NkdW1hMCtPbU1IcnM4RGE3UEo3VzBjSFJNYWZXVG1xSE1RST0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    server: https://invalid.local:6443
  name: kubernetes
contexts:
- context:
    cluster: kubernetes
    user: system:node:localhost.localdomain
  name: system:node:localhost.localdomain@kubernetes
current-context: system:node:localhost.localdomain@kubernetes
kind: Config
preferences: {}
users:
- name: system:node:localhost.localdomain
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lJWjd2bEJJYUF6V3d3RFFZSktvWklodmNOQVFFTEJRQXdGVEVUTUJFR0ExVUUKQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB4T1RBMU16RXhOVEUwTkRCYUZ3MHlNREExTXpBeE5URTBOREZhTUVNeApGVEFUQmdOVkJBb1RESE41YzNSbGJUcHViMlJsY3pFcU1DZ0dBMVVFQXhNaGMzbHpkR1Z0T201dlpHVTZiRzlqCllXeG9iM04wTG14dlkyRnNaRzl0WVdsdU1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0MKQVFFQTFMcmZlMWlzUGkzQ2J4OHh0Q1htUE5RNi9MV2FVRGtWUTVQZHRrUCt3VWJnSVBSemNtRzNEcnUya0kvagprMTBkM2lIMzlWZWQ5R1Y3M252clZuSHdKc2RBeUM1ZmJUU0FQVFVTSTlvdDFRRmxDUVBnQ2JLbjBRbmxUeUhXCjJjZ04rNkNwVitFOHRaVmoydnA2aEw5ZkdLNm1CQi9iOFRTd0ZmMUZrZ1gvYUxjdXpmQ2hmVFNDOWlRTk15cEEKY2pmWnRJM1orM2x1c0lQek1aQTU2T2VzaHc5Z0ZCR1JMN0c2R1pmSnBvTmhlcUw5Zmp3VFRkNURlbVZXcUxURwpOWTVoYVd4YnMvVW04bjNEL3ZVdUJFT2E2MUNnL3BpY1JFN1JteEphSUViYkJTU1dXZEgzWDlrem5RdHJrUXloCi9vMWZ6UldacXlhQmN4cUdRVjdCUksxbUp3SURBUUFCb3ljd0pUQU9CZ05WSFE4QkFmOEVCQU1DQmFBd0V3WUQKVlIwbEJBd3dDZ1lJS3dZQkJRVUhBd0l3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQUNEWGRPQ3FxVUx6b1FJRgpaWGpBNWxBMXArZG1KaUg2Q016N2taTWozazkzbGs3ZmZTWXZKQjB3MFY2UUZGcExQNUJON1pFRzNsTmxKdkZzCjFhR2M4Q0tvV3kzNzJvR1ZnRERlTUFZQ0dic0RLYnhTMHVETHcydGxuQkpoTTRFc1lldHBnNUE0dmpUa0g3SWkKVzljb1V4V2xWdjAvN2VPMW1sTzloYVhMQnJjS1l0eTNneEF6R2NJNW9wNnpYZjFLNk5UMWJaN2gya1dvdyt1MgpNZXNYZFFueHdVZ0YzdzNWMVREQSt1UGFFN0R2MmRvMnNTRTc4Mk9rRDhna0UzdytJSnhqOXVwSTMrcDVOVzBaCmtDMVk2b3NIN0pFT1RHQW8yeTZ0b2pFNngwdTJ6R0E3UVVpM1N4czNMcnhLcFFkSFN1aitZZnh0dkgvOWY4ak4KS1BMbFV5ST0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb2dJQkFBS0NBUUVBMUxyZmUxaXNQaTNDYng4eHRDWG1QTlE2L0xXYVVEa1ZRNVBkdGtQK3dVYmdJUFJ6CmNtRzNEcnUya0kvamsxMGQzaUgzOVZlZDlHVjczbnZyVm5Id0pzZEF5QzVmYlRTQVBUVVNJOW90MVFGbENRUGcKQ2JLbjBRbmxUeUhXMmNnTis2Q3BWK0U4dFpWajJ2cDZoTDlmR0s2bUJCL2I4VFN3RmYxRmtnWC9hTGN1emZDaApmVFNDOWlRTk15cEFjamZadEkzWiszbHVzSVB6TVpBNTZPZXNodzlnRkJHUkw3RzZHWmZKcG9OaGVxTDlmandUClRkNURlbVZXcUxUR05ZNWhhV3hicy9VbThuM0QvdlV1QkVPYTYxQ2cvcGljUkU3Um14SmFJRWJiQlNTV1dkSDMKWDlrem5RdHJrUXloL28xZnpSV1pxeWFCY3hxR1FWN0JSSzFtSndJREFRQUJBb0lCQUJ0bjA4QzFSTU5oNjhtYgpFREV3TE1BcmEwb0JMMWNrYzN2WVFkam9XNXFVd2UwYzhQNk1YaVAweE9sTTBEbTg1a3NtdnlZSldwMFFzZXVRCnRWbldwZVNwQ015QlJPUHh2bytrRmFrdXczYk1qaktpSUN1L3EyVC96RjNzY3h4dGJIZTlVL094WGJ2YStobE0KNlpuT2ViYlpVU1A0NHNIcFVzSVNkZk1BK00ySmg1UFJibGZWaUFEY1hxNFR5RU1JaStzRkhOcFIrdmdWZzRFawp4RmFVaS83V0E2YUxWVzBUTzREdjMwbTJ0TVczWXN1bk1LTU0xOTNyUEZrU0dEdFpheWV2Z0JDeURXaFhOTEo2Clh1cTNxSUg4bFE2bzRBUjMvcDc1ZW9hOCtrVzVmT3o2UWF3WnpPYlBENkRCQlVOYVM1YklXaVV1dmx5L0JlM20KZnlxK3NRRUNnWUVBMW84R3l6ODk2bFhwdU1yVXVsb2orbGp5U0FaNkpLOCsyaFMvQnpyREx6NlpvY3FzKzg3awpVUkwzKy9LL1pja2pIMVVDeXROVVZ6Q3RKaG4zZmdLS2dpQWhsU0pNRzhqc05sbEkydFZSazNZZ1RCcUg2bXZxCit3citsTUxoUDZxbWFObUx3QXljY2lEanpMdXlRdjhVOFhKazJOdVFsQlFwbkt2eWJIRGdxSUVDZ1lFQS9kRnMKazNlYmRNNFAxV2psYXJoRTV5blpuRmdQbDg1L2Vudk4rQ1oxcStlMGxYendaUGswdWdJUWozYyt2UEpLWlh0OApLWk1HQjM0N2VLNlFIL3J1a2xRWXlLOStHeUV1YnRJQUZ2NWFrYXZxV1haR1p5ZC9QdDR1V09adXMrd3BnSG00CkxFY0lzZElsYkpFY2RJTzJyb3FaY0VNY3FEbGtXcTdwQWxqU2VxY0NnWUJYdUQ0RTFxUlByRFJVRXNrS0wxUksKUkJjNkR6dmN4N0VncEI2OXErNms0Q2tibHF0R2YvMmtqK2JISVNYVFRYcUlrczhEY1ljbjVvVEQ4UlhZZE4xLworZmNBNi9iRjNVMkZvdGRBY0xwYldZNDJ6eG9HWTN5OGluQXZEY1hkcTcxQlhML2dFc2ZiZVVycEowdm9URFdaCnlUVWwzQTZ1RzlndmI3VTdWS0xsQVFLQmdBTmNscmVOU2YzT0ROK2l1QWNsMGFQT0poZXdBdVRiMDB4bi8xNWUKQkFqMjFLbDJNaWprTkJLU25HMktBc2ExM3M1aFNFKzBwc3ZLbkRjSStOZXpseDFSQjlNQW9BYno5WTE2TW80YgphRSt0bXpqOEhBcVp0MUc1MTV0TjBnR0lDelNzYUFnT0dNdGlJU1RDOTBHRHpST2F1bFdHVGdiY1c3dm52U1pPCnp0clpBb0dBWmtIRWV5em16Z2cxR3dtTzN3bmljSHRMR1BQRFhiSW53NTdsdkIrY3lyd0FrVEs1MlFScTM0VkMKRDhnQWFwMTU2OWlWUER3YlgrNkpBQk1WQ2tNUmdxMjdHanUzN0pVY2Fib2g1YzJQeTBYNUlhUG8rek1hWHgvQwpqbjUvUW5YandjU1MrRU5hL1lXVWcxWEVjQjJYdEM0UExCdGUycitrUTVLbFNOREcxSTQ9Ci0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0tCg==`

		kubeletconf.Write([]byte(kubeletconfDef))
		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := fmt.Sprintf(`{
			"name":"node-cni-network",
			"type":"multus",
			"kubeconfig":"%s/kubelet.conf",
			"delegates": [{
				"type": "mynet2",
				"name": "net2"
			}]
		}`, dir)
		netConf, err := types.LoadNetConf([]byte(conf))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net2"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet2"))
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testutils.NewFakeNetAttachDef("kube-system", "net1", "{\"type\": \"mynet1\"}"))
		Expect(err).NotTo(HaveOccurred())
		_, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = TryLoadPodDelegates(fakePod, netConf, clientInfo, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Errors when namespace isolation is violated", func() {
		fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"delegates": [{
			"name": "weave1",
				"cniVersion": "0.2.0",
				"type": "weave-net"
			}],
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml",
			"namespaceIsolation": true
		}`

		Expect(err).NotTo(HaveOccurred())

		net1 := `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())

		netConf, err := types.LoadNetConf([]byte(conf))
		netConf.ConfDir = tmpDir
		_, err = GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("GetNetworkDelegates: namespace isolation enabled, annotation violates permission, pod is in namespace test but refers to target namespace kube-system"))

	})

	It("Properly allows a specified namespace reference when namespace isolation is enabled", func() {
		fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"delegates": [{
			"name": "weave1",
				"cniVersion": "0.2.0",
				"type": "weave-net"
			}],
			"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml",
			"namespaceIsolation": true,
			"globalNamespaces": "kube-system,donkey-kong"
		}`

		Expect(err).NotTo(HaveOccurred())

		net1 := `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`

		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		clientInfo := NewFakeClientInfo()
		_, err = clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())

		netConf, err := types.LoadNetConf([]byte(conf))
		netConf.ConfDir = tmpDir
		_, err = GetNetworkDelegates(clientInfo, pod, networks, netConf, nil)

		Expect(err).NotTo(HaveOccurred())

	})

	Context("Error function", func() {
		It("Returns proper error message", func() {
			err := &NoK8sNetworkError{"no kubernetes network found"}
			Expect(err.Error()).To(Equal("no kubernetes network found"))
		})
	})

	Context("getDefaultNetDelegateCRD", func() {
		It("fails when netConf contains bad confDir", func() {
			fakePod := testutils.NewFakePod("testpod", "", "net1")
			conf := `{
				"name":"node-cni-network",
				"type":"multus",
				"clusterNetwork": "net2",
				"multusNamespace" : "kube-system",
				"kubeconfig":"/etc/kubernetes/node-kubeconfig.yaml"
			}`
			netConf, err := types.LoadNetConf([]byte(conf))
			Expect(err).NotTo(HaveOccurred())

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err = clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", ""))
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net2", ""))
			Expect(err).NotTo(HaveOccurred())

			_, err = GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			netConf.ConfDir = "garbage value"
			_, err = GetDefaultNetworks(fakePod, netConf, clientInfo, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetK8sArgs", func() {
		It("fails when provided with bad format", func() {
			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")
			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME:%s;K8S_POD_NAMESPACE:%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}
			// using colon instead of equals sign makes an invalid CmdArgs

			_, err := GetK8sArgs(args)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("getKubernetesDelegate", func() {
		It("failed to get a ResourceClient instance", func() {
			fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
			net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.2.0"
	}`
			net2 := `{
		"name": "net2",
		"type": "mynet2",
		"cniVersion": "0.2.0"
	}`
			net3 := `{
		"name": "net3",
		"type": "mynet3",
		"cniVersion": "0.2.0"
	}`
			// args := &skel.CmdArgs{
			// 	Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			// }

			clientInfo := NewFakeClientInfo()
			_, err := clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(
				testutils.NewFakeNetAttachDefAnnotation(fakePod.ObjectMeta.Namespace, "net1", net1))
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(
				testutils.NewFakeNetAttachDefAnnotation(fakePod.ObjectMeta.Namespace, "net2", net2))
			Expect(err).NotTo(HaveOccurred())
			// net3 is not used; make sure it's not accessed
			_, err = clientInfo.AddNetAttachDef(
				testutils.NewFakeNetAttachDefAnnotation(fakePod.ObjectMeta.Namespace, "net3", net3))
			Expect(err).NotTo(HaveOccurred())

			networks, err := GetPodNetwork(fakePod)
			Expect(err).NotTo(HaveOccurred())

			netConf, err := types.LoadNetConf([]byte(genericConf))
			netConf.ConfDir = tmpDir
			_, err = GetNetworkDelegates(clientInfo, fakePod, networks, netConf, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("parsePodNetworkObjectName", func() {
		It("fails to get podnetwork given bad annotation values", func() {
			fakePod := testutils.NewFakePod("testpod", "net1", "")
			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err := clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())

			_, err = clientInfo.AddNetAttachDef(
				testutils.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", "{\"type\": \"mynet\"}"))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())
			pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))

			// invalid case 1 - can't have more than 2 items separated by "/"
			pod.Annotations[networkAttachmentAnnot] = "root@someIP/root@someOtherIP/root@thirdIP"
			_, err = GetPodNetwork(pod)
			Expect(err).To(HaveOccurred())

			// invalid case 2 - can't have more than 2 items separated by "@"
			pod.Annotations[networkAttachmentAnnot] = "root@someIP/root@someOtherIP@garbagevalue"
			_, err = GetPodNetwork(pod)
			Expect(err).To(HaveOccurred())

			// invalid case 3 - not matching comma-delimited format
			pod.Annotations[networkAttachmentAnnot] = "root@someIP/root@someOtherIP"
			_, err = GetPodNetwork(pod)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("setPodNetworkAnnotation", func() {
		It("Sets pod network annotations without error", func() {
			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

			net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.2.0"
	}`

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err := clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())

			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
			Expect(err).NotTo(HaveOccurred())

			fakeStatus := []nettypes.NetworkStatus{
				{
					Name:      "cbr0",
					Interface: "eth0",
					IPs:       []string{"10.244.1.2"},
					Mac:       "92:79:27:01:7c:ce",
				},
				{
					Name:      "test-net-attach-def-1",
					Interface: "net1",
					IPs:       []string{"1.1.1.1"},
					Mac:       "ea:0e:fa:63:95:f9",
				},
			}
			err = netutils.SetNetworkStatus(clientInfo.Client, pod, fakeStatus)
			Expect(err).NotTo(HaveOccurred())
		})

		// TODO Still figuring this next one out. deals with exponentialBackoff
		// It("Fails to set pod network annotations without error", func() {
		// 	fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

		// 	net1 := `{
		// 	"name": "net1",
		// 	"type": "mynet",
		// 	"cniVersion": "0.2.0"
		// }`

		// 	args := &skel.CmdArgs{
		// 		Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		// 	}

		//	clientInfo := NewFakeClientInfo()
		//	_, err := clientInfo.AddPod(fakePod)
		//	Expect(err).NotTo(HaveOccurred())
		//	_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
		//	Expect(err).NotTo(HaveOccurred())

		// 	k8sArgs, err := GetK8sArgs(args)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		// 	Expect(err).NotTo(HaveOccurred())

		// 	networkstatus := "test status"
		// 	_, err = setPodNetworkAnnotation(clientInfo, "test", pod, networkstatus)
		// 	Expect(err).NotTo(HaveOccurred())
		// })
	})

	Context("setPodNetworkAnnotation", func() {
		It("Sets pod network annotations without error", func() {
			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

			net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.2.0"
	}`

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err := clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())

			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
			Expect(err).NotTo(HaveOccurred())

			fakeStatus := []nettypes.NetworkStatus{
				{
					Name:      "cbr0",
					Interface: "eth0",
					IPs:       []string{"10.244.1.2"},
					Mac:       "92:79:27:01:7c:ce",
				},
				{
					Name:      "test-net-attach-def-1",
					Interface: "net1",
					IPs:       []string{"1.1.1.1"},
					Mac:       "ea:0e:fa:63:95:f9",
				},
			}
			err = netutils.SetNetworkStatus(clientInfo.Client, pod, fakeStatus)
			Expect(err).NotTo(HaveOccurred())
		})

		// TODO Still figuring this next one out. deals with exponentialBackoff
		// It("Fails to set pod network annotations without error", func() {
		// 	fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

		// 	net1 := `{
		// 	"name": "net1",
		// 	"type": "mynet",
		// 	"cniVersion": "0.2.0"
		// }`

		// 	args := &skel.CmdArgs{
		// 		Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		// 	}

		//	clientInfo := NewFakeClientInfo()
		//	_, err := clientInfo.AddPod(fakePod)
		//	Expect(err).NotTo(HaveOccurred())
		//	_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
		//	Expect(err).NotTo(HaveOccurred())

		// 	k8sArgs, err := GetK8sArgs(args)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	pod, err := clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		// 	Expect(err).NotTo(HaveOccurred())

		// 	networkstatus := "test status"
		// 	_, err = setPodNetworkAnnotation(clientInfo, "test", pod, networkstatus)
		// 	Expect(err).NotTo(HaveOccurred())
		// })
	})

	Context("SetNetworkStatus", func() {
		It("Sets network status without error", func() {
			result := &types020.Result{
				CNIVersion: "0.2.0",
				IP4: &types020.IPConfig{
					IP: *testutils.EnsureCIDR("1.1.1.2/24"),
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

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0", "")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := netutils.CreateNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin, nil)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []nettypes.NetworkStatus{*delegateNetStatus}

			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

			netConf, err := types.LoadNetConf([]byte(conf))
			Expect(err).NotTo(HaveOccurred())

			net1 := `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err = clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(clientInfo, k8sArgs, netstatus, netConf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Sets network status with kubeclient built from kubeconfig and attempts to connect", func() {
			kubeletconf, err := os.Create("/etc/kubernetes/kubelet.conf")
			kubeletconfDef := `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5RENDQWJDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1EVXpNVEUxTVRRME1Gb1hEVEk1TURVeU9ERTFNVFEwTUZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBS3pLCnZQSHNxQWpMSHdxbDdMaDdQeTJuSndEdzAwTU4rZjFVTWtIS1BrOTVsTVRVRmgrMTVob05pWVJjaGt2d3VTRXMKRXNLYTJBRXpVeCtxR3hqTXptb01RdklMamNLbVdUS3ViNXZvZ29WV3l0N2ZzaW1jUlk4c2hWbmFSQ1pQZnptYgpUTVVRcFNHbUc5WWNGNlJhMnhwbTJ6Qm5aVWM0QjN6M1UxeHE3b1NRREp5ZVRna3VEZDB3SGhLay9tYWREYlgyCk5DU2ROVjRoUEFJcWY0MVZWc0hGV3c4WWpOUGllMVZBdWZQZDBRQlJuVFgraW50dm9weVpuWnJoVFJzVU5CanMKRWc0UGFEVXRMZDZsSUxqUnhEN2RkL1JYUWZaQ1E5azI3b0t1eU5RZUJMaWdhb052M3JPeExPWGxMaU1HUGZpVgpPTktxencwaGFDUlF1cHdoUlFVQ0F3RUFBYU1qTUNFd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0RRWUpLb1pJaHZjTkFRRUxCUUFEZ2dFQkFGU0MrYmJHaENZTyszOXA2QVJtOFBYbnZFTmIKTjdNd2VWSXVGazh3T3c2RWlHOTllZTJIb25KOCtxaEFLNXlCNVU0d3FEM2FkY0hNTHBGVUoxMVMrSVYra2hCYgo0SGYrcEtVZEJxM3MvYXAxMmppdUNZMUVDanpIVjZ5SDZLRGwrZEdibDR1dVJ6S016SkZteFpncEl4TUVqbEZ0CnByK2MzQmdaWWVVYlpTdDFhR2VObFlvdGxKZEFBbWlZcmkxcFAzOUxzb2xZbWJEOTNVb1NTYnhoR2lkSzBjNHcKbHV5aUxJQWJUVnJxMEo1UllCa05ZOGdMVTBRZVFJaVZTYWhkc3cvakgxL0dZY2J4alN6R1ZwOTAwb1ZMWUpsQQo5SkhVaUlTb3VHVTE1QVRpMklvN0tOa2NkdW1hMCtPbU1IcnM4RGE3UEo3VzBjSFJNYWZXVG1xSE1RST0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    server: https://invalid.local:6443
  name: kubernetes
contexts:
- context:
    cluster: kubernetes
    user: system:node:localhost.localdomain
  name: system:node:localhost.localdomain@kubernetes
current-context: system:node:localhost.localdomain@kubernetes
kind: Config
preferences: {}
users:
- name: system:node:localhost.localdomain
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lJWjd2bEJJYUF6V3d3RFFZSktvWklodmNOQVFFTEJRQXdGVEVUTUJFR0ExVUUKQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB4T1RBMU16RXhOVEUwTkRCYUZ3MHlNREExTXpBeE5URTBOREZhTUVNeApGVEFUQmdOVkJBb1RESE41YzNSbGJUcHViMlJsY3pFcU1DZ0dBMVVFQXhNaGMzbHpkR1Z0T201dlpHVTZiRzlqCllXeG9iM04wTG14dlkyRnNaRzl0WVdsdU1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0MKQVFFQTFMcmZlMWlzUGkzQ2J4OHh0Q1htUE5RNi9MV2FVRGtWUTVQZHRrUCt3VWJnSVBSemNtRzNEcnUya0kvagprMTBkM2lIMzlWZWQ5R1Y3M252clZuSHdKc2RBeUM1ZmJUU0FQVFVTSTlvdDFRRmxDUVBnQ2JLbjBRbmxUeUhXCjJjZ04rNkNwVitFOHRaVmoydnA2aEw5ZkdLNm1CQi9iOFRTd0ZmMUZrZ1gvYUxjdXpmQ2hmVFNDOWlRTk15cEEKY2pmWnRJM1orM2x1c0lQek1aQTU2T2VzaHc5Z0ZCR1JMN0c2R1pmSnBvTmhlcUw5Zmp3VFRkNURlbVZXcUxURwpOWTVoYVd4YnMvVW04bjNEL3ZVdUJFT2E2MUNnL3BpY1JFN1JteEphSUViYkJTU1dXZEgzWDlrem5RdHJrUXloCi9vMWZ6UldacXlhQmN4cUdRVjdCUksxbUp3SURBUUFCb3ljd0pUQU9CZ05WSFE4QkFmOEVCQU1DQmFBd0V3WUQKVlIwbEJBd3dDZ1lJS3dZQkJRVUhBd0l3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQUNEWGRPQ3FxVUx6b1FJRgpaWGpBNWxBMXArZG1KaUg2Q016N2taTWozazkzbGs3ZmZTWXZKQjB3MFY2UUZGcExQNUJON1pFRzNsTmxKdkZzCjFhR2M4Q0tvV3kzNzJvR1ZnRERlTUFZQ0dic0RLYnhTMHVETHcydGxuQkpoTTRFc1lldHBnNUE0dmpUa0g3SWkKVzljb1V4V2xWdjAvN2VPMW1sTzloYVhMQnJjS1l0eTNneEF6R2NJNW9wNnpYZjFLNk5UMWJaN2gya1dvdyt1MgpNZXNYZFFueHdVZ0YzdzNWMVREQSt1UGFFN0R2MmRvMnNTRTc4Mk9rRDhna0UzdytJSnhqOXVwSTMrcDVOVzBaCmtDMVk2b3NIN0pFT1RHQW8yeTZ0b2pFNngwdTJ6R0E3UVVpM1N4czNMcnhLcFFkSFN1aitZZnh0dkgvOWY4ak4KS1BMbFV5ST0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb2dJQkFBS0NBUUVBMUxyZmUxaXNQaTNDYng4eHRDWG1QTlE2L0xXYVVEa1ZRNVBkdGtQK3dVYmdJUFJ6CmNtRzNEcnUya0kvamsxMGQzaUgzOVZlZDlHVjczbnZyVm5Id0pzZEF5QzVmYlRTQVBUVVNJOW90MVFGbENRUGcKQ2JLbjBRbmxUeUhXMmNnTis2Q3BWK0U4dFpWajJ2cDZoTDlmR0s2bUJCL2I4VFN3RmYxRmtnWC9hTGN1emZDaApmVFNDOWlRTk15cEFjamZadEkzWiszbHVzSVB6TVpBNTZPZXNodzlnRkJHUkw3RzZHWmZKcG9OaGVxTDlmandUClRkNURlbVZXcUxUR05ZNWhhV3hicy9VbThuM0QvdlV1QkVPYTYxQ2cvcGljUkU3Um14SmFJRWJiQlNTV1dkSDMKWDlrem5RdHJrUXloL28xZnpSV1pxeWFCY3hxR1FWN0JSSzFtSndJREFRQUJBb0lCQUJ0bjA4QzFSTU5oNjhtYgpFREV3TE1BcmEwb0JMMWNrYzN2WVFkam9XNXFVd2UwYzhQNk1YaVAweE9sTTBEbTg1a3NtdnlZSldwMFFzZXVRCnRWbldwZVNwQ015QlJPUHh2bytrRmFrdXczYk1qaktpSUN1L3EyVC96RjNzY3h4dGJIZTlVL094WGJ2YStobE0KNlpuT2ViYlpVU1A0NHNIcFVzSVNkZk1BK00ySmg1UFJibGZWaUFEY1hxNFR5RU1JaStzRkhOcFIrdmdWZzRFawp4RmFVaS83V0E2YUxWVzBUTzREdjMwbTJ0TVczWXN1bk1LTU0xOTNyUEZrU0dEdFpheWV2Z0JDeURXaFhOTEo2Clh1cTNxSUg4bFE2bzRBUjMvcDc1ZW9hOCtrVzVmT3o2UWF3WnpPYlBENkRCQlVOYVM1YklXaVV1dmx5L0JlM20KZnlxK3NRRUNnWUVBMW84R3l6ODk2bFhwdU1yVXVsb2orbGp5U0FaNkpLOCsyaFMvQnpyREx6NlpvY3FzKzg3awpVUkwzKy9LL1pja2pIMVVDeXROVVZ6Q3RKaG4zZmdLS2dpQWhsU0pNRzhqc05sbEkydFZSazNZZ1RCcUg2bXZxCit3citsTUxoUDZxbWFObUx3QXljY2lEanpMdXlRdjhVOFhKazJOdVFsQlFwbkt2eWJIRGdxSUVDZ1lFQS9kRnMKazNlYmRNNFAxV2psYXJoRTV5blpuRmdQbDg1L2Vudk4rQ1oxcStlMGxYendaUGswdWdJUWozYyt2UEpLWlh0OApLWk1HQjM0N2VLNlFIL3J1a2xRWXlLOStHeUV1YnRJQUZ2NWFrYXZxV1haR1p5ZC9QdDR1V09adXMrd3BnSG00CkxFY0lzZElsYkpFY2RJTzJyb3FaY0VNY3FEbGtXcTdwQWxqU2VxY0NnWUJYdUQ0RTFxUlByRFJVRXNrS0wxUksKUkJjNkR6dmN4N0VncEI2OXErNms0Q2tibHF0R2YvMmtqK2JISVNYVFRYcUlrczhEY1ljbjVvVEQ4UlhZZE4xLworZmNBNi9iRjNVMkZvdGRBY0xwYldZNDJ6eG9HWTN5OGluQXZEY1hkcTcxQlhML2dFc2ZiZVVycEowdm9URFdaCnlUVWwzQTZ1RzlndmI3VTdWS0xsQVFLQmdBTmNscmVOU2YzT0ROK2l1QWNsMGFQT0poZXdBdVRiMDB4bi8xNWUKQkFqMjFLbDJNaWprTkJLU25HMktBc2ExM3M1aFNFKzBwc3ZLbkRjSStOZXpseDFSQjlNQW9BYno5WTE2TW80YgphRSt0bXpqOEhBcVp0MUc1MTV0TjBnR0lDelNzYUFnT0dNdGlJU1RDOTBHRHpST2F1bFdHVGdiY1c3dm52U1pPCnp0clpBb0dBWmtIRWV5em16Z2cxR3dtTzN3bmljSHRMR1BQRFhiSW53NTdsdkIrY3lyd0FrVEs1MlFScTM0VkMKRDhnQWFwMTU2OWlWUER3YlgrNkpBQk1WQ2tNUmdxMjdHanUzN0pVY2Fib2g1YzJQeTBYNUlhUG8rek1hWHgvQwpqbjUvUW5YandjU1MrRU5hL1lXVWcxWEVjQjJYdEM0UExCdGUycitrUTVLbFNOREcxSTQ9Ci0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0tCg==`

			kubeletconf.Write([]byte(kubeletconfDef))

			result := &types020.Result{
				CNIVersion: "0.2.0",
				IP4: &types020.IPConfig{
					IP: *testutils.EnsureCIDR("1.1.1.2/24"),
				},
			}

			conf := `{
			"name": "node-cni-network",
			"type": "multus",
			"kubeconfig": "/etc/kubernetes/kubelet.conf",
			"delegates": [{
				"type": "weave-net"
			}],
		  "runtimeConfig": {
			  "portMappings": [
				{"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			  ]
			}
		}`

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0", "")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := netutils.CreateNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin, nil)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []nettypes.NetworkStatus{*delegateNetStatus}

			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

			netConf, err := types.LoadNetConf([]byte(conf))
			Expect(err).NotTo(HaveOccurred())

			net1 := `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err = clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(nil, k8sArgs, netstatus, netConf)
			Expect(err).To(HaveOccurred())
		})

		It("Fails to set network status without kubeclient or kubeconfig", func() {
			result := &types020.Result{
				CNIVersion: "0.2.0",
				IP4: &types020.IPConfig{
					IP: *testutils.EnsureCIDR("1.1.1.2/24"),
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
			// note that the provided kubeconfig is invalid

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "", "")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := netutils.CreateNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin, nil)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []nettypes.NetworkStatus{*delegateNetStatus}

			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

			netConf, err := types.LoadNetConf([]byte(conf))
			Expect(err).NotTo(HaveOccurred())

			net1 := `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err = clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(nil, k8sArgs, netstatus, netConf)
			Expect(err).To(HaveOccurred())
		})

		It("Skips network status given no config", func() {
			result := &types020.Result{
				CNIVersion: "0.2.0",
				IP4: &types020.IPConfig{
					IP: *testutils.EnsureCIDR("1.1.1.2/24"),
				},
			}

			conf := `{
			"name": "node-cni-network",
			"type": "multus",
			"kubeconfig": "",
			"delegates": [{
				"type": "weave-net"
			}],
		  "runtimeConfig": {
			  "portMappings": [
				{"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			  ]
			}
		}`

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0", "")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := netutils.CreateNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin, nil)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []nettypes.NetworkStatus{*delegateNetStatus}

			fakePod := testutils.NewFakePod("testpod", "kube-system/net1", "")

			netConf, err := types.LoadNetConf([]byte(conf))
			Expect(err).NotTo(HaveOccurred())

			net1 := `{
			"name": "net1",
			"type": "mynet",
			"cniVersion": "0.2.0"
		}`

			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			clientInfo := NewFakeClientInfo()
			_, err = clientInfo.AddPod(fakePod)
			Expect(err).NotTo(HaveOccurred())
			_, err = clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("kube-system", "net1", net1))
			Expect(err).NotTo(HaveOccurred())

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(nil, k8sArgs, netstatus, netConf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
