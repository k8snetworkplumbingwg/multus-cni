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
	testhelpers "github.com/intel/multus-cni/testing"
	testutils "github.com/intel/multus-cni/testing"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/intel/multus-cni/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestK8sClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "k8sclient")
}

var _ = Describe("k8sclient operations", func() {
	var tmpDir string
	var err error

	BeforeEach(func() {
		tmpDir, err = ioutil.TempDir("", "multus_tmp")
		Expect(err).NotTo(HaveOccurred())
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net1", net1)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net2", net2)
		// net3 is not used; make sure it's not accessed
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net3", net3)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetNetworkDelegates(kubeClient, pod, networks, tmpDir, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(fKubeClient.PodCount).To(Equal(1))
		Expect(fKubeClient.NetCount).To(Equal(2))

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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net3", net3)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetNetworkDelegates(kubeClient, pod, networks, tmpDir, false)
		Expect(len(delegates)).To(Equal(0))
		Expect(err).To(MatchError("GetPodNetwork: failed getting the delegate: getKubernetesDelegate: failed to get network resource, refer Multus README.md for the usage guide: resource not found"))
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net1", `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net2", `{
	"name": "net2",
	"type": "mynet2",
	"cniVersion": "0.2.0"
}`)
		fKubeClient.AddNetConfig("other-ns", "net3", `{
	"name": "net3",
	"type": "mynet3",
	"cniVersion": "0.2.0"
}`)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetNetworkDelegates(kubeClient, pod, networks, tmpDir, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(fKubeClient.PodCount).To(Equal(1))
		Expect(fKubeClient.NetCount).To(Equal(3))

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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(len(networks)).To(Equal(0))
		Expect(err).To(MatchError("parsePodNetworkAnnotation: failed to parse pod Network Attachment Selection Annotation JSON format: invalid character 'a' looking for beginning of value"))
	})

	It("retrieves delegates from kubernetes using on-disk config files", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		fKubeClient.AddNetFile(fakePod.ObjectMeta.Namespace, "net1", net1Name, `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`)
		net2Name := filepath.Join(tmpDir, "20-net2.conf")
		fKubeClient.AddNetFile(fakePod.ObjectMeta.Namespace, "net2", net2Name, `{
	"name": "net2",
	"type": "mynet2",
	"cniVersion": "0.2.0"
}`)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetNetworkDelegates(kubeClient, pod, networks, tmpDir, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(fKubeClient.PodCount).To(Equal(1))
		Expect(fKubeClient.NetCount).To(Equal(2))

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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net1", "{\"type\": \"mynet\"}")

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetNetworkDelegates(kubeClient, pod, networks, tmpDir, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(fKubeClient.PodCount).To(Equal(1))
		Expect(fKubeClient.NetCount).To(Equal(1))

		Expect(len(delegates)).To(Equal(1))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("fails when on-disk config file is not valid", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		fKubeClient.AddNetFile(fakePod.ObjectMeta.Namespace, "net1", net1Name, `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`)
		net2Name := filepath.Join(tmpDir, "20-net2.conf")
		fKubeClient.AddNetFile(fakePod.ObjectMeta.Namespace, "net2", net2Name, "asdfasdfasfdasfd")

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetNetworkDelegates(kubeClient, pod, networks, tmpDir, false)
		Expect(len(delegates)).To(Equal(0))
		Expect(err).To(MatchError(fmt.Sprintf("GetPodNetwork: failed getting the delegate: cniConfigFromNetworkResource: err in getCNIConfigFromFile: Error loading CNI config file %s: error parsing configuration: invalid character 'a' looking for beginning of value", net2Name)))
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddNetConfig("kube-system", "myCRD1", "{\"type\": \"mynet\"}")
		fKubeClient.AddPod(fakePod)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddNetConfig("kube-system", "myCRD1", "{\"type\": \"mynet\"}")
		fKubeClient.AddNetConfig("kube-system", "myCRD2", "{\"type\": \"mynet2\"}")
		fKubeClient.AddPod(fakePod)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddNetConfig("kube-system", "myCRD1", "{\"type\": \"mynet\"}")
		fKubeClient.AddNetConfig("kube-system", "myCRD2", "{\"type\": \"mynet2\"}")
		fKubeClient.AddPod(fakePod)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		fKubeClient.AddNetFile(fakePod.ObjectMeta.Namespace, "net1", net1Name, `{
	"name": "myFile1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		net1Name := filepath.Join(tmpDir, "10-net1.conf")
		fKubeClient.AddNetFile(fakePod.ObjectMeta.Namespace, "10-net1", net1Name, `{
	"name": "net1",
	"type": "mynet",
	"cniVersion": "0.2.0"
}`)
		fKubeClient.AddPod(fakePod)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddNetConfig("kube-system", "net1", "{\"type\": \"mynet1\"}")
		fKubeClient.AddNetConfig("kube-system", "net2", "{\"type\": \"mynet2\"}")
		fKubeClient.AddPod(fakePod)
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		err = GetDefaultNetworks(k8sArgs, netConf, kubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(netConf.Delegates)).To(Equal(1))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net2"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet2"))

		numK8sDelegates, _, err := TryLoadPodDelegates(k8sArgs, netConf, kubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(numK8sDelegates).To(Equal(0))
		Expect(netConf.Delegates[0].Conf.Name).To(Equal("net1"))
		Expect(netConf.Delegates[0].Conf.Type).To(Equal("mynet1"))
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig("kube-system", "net1", "{\"type\": \"mynet1\"}")
		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		numK8sDelegates, _, err := TryLoadPodDelegates(k8sArgs, netConf, kubeClient)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig("kube-system", "net1", "{\"type\": \"mynet1\"}")
		_, err = GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = TryLoadPodDelegates(k8sArgs, netConf, nil)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig("kube-system", "net1", "{\"type\": \"mynet1\"}")
		_, err = GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = TryLoadPodDelegates(k8sArgs, netConf, nil)
		Expect(err).NotTo(HaveOccurred())

		// additionally, we expect the test to fail with no delegates, as at least one is always required.
		netConf.Delegates = nil
		_, _, err = TryLoadPodDelegates(k8sArgs, netConf, nil)
		Expect(err).To(HaveOccurred())
	})

	It("uses cached delegates when an error in loading from pod annotation occurs", func() {
		//something about cached delegates, line 438
		fakePod := testutils.NewFakePod("testpod", "", "net1")
		conf := `{
			"name":"node-cni-network",
			"type":"multus",
			"kubeconfig":"/etc/kubernetes/kubelet.conf",
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig("kube-system", "net1", "{\"type\": \"mynet1\"}")
		_, err = GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		_, _, err = TryLoadPodDelegates(k8sArgs, netConf, nil)
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

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig("kube-system", "net1", net1)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())

		pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		networks, err := GetPodNetwork(pod)
		Expect(err).NotTo(HaveOccurred())
		_, err = GetNetworkDelegates(kubeClient, pod, networks, tmpDir, netConf.NamespaceIsolation)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("GetPodNetwork: namespace isolation violation: podnamespace: test / target namespace: kube-system"))

	})

	Context("Error function", func() {
		It("Returns proper error message", func() {
			err := &NoK8sNetworkError{"no kubernetes network found"}
			Expect(err.Error()).To(Equal("no kubernetes network found"))
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

	// can't get this test working yet - metadata annotation issue
	// 	Context("getKubernetesDelegate", func() {
	// 		It("reaches that big chunk of untested code, better test name is a work in progress", func() {
	// 			/*
	// 				func call [networks] -> getnetworkdelegates [net] -> getkubernetesdelegate
	// 			*/
	// 			fmt.Println("BLAH!!!")
	// 			fakePod := testutils.NewFakePod("testpod", "net1,net2", "")
	// 			net1 := `{
	// 	"name": "net1",
	// 	"type": "mynet",
	// 	"cniVersion": "0.2.0"
	// }`
	// 			net2 := `{
	// 	"name": "net2",
	// 	"type": "mynet2",
	// 	"cniVersion": "0.2.0"
	// }`
	// 			net3 := `{
	// 	"name": "net3",
	// 	"type": "mynet3",
	// 	"cniVersion": "0.2.0"
	// }`
	// 			// args := &skel.CmdArgs{
	// 			// 	Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
	// 			// }

	// 			fKubeClient := testutils.NewFakeKubeClient()
	// 			fKubeClient.AddPod(fakePod)
	// 			fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net1", net1)
	// 			fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net2", net2)
	// 			// net3 is not used; make sure it's not accessed
	// 			fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net3", net3)

	// 			kubeClient, err := GetK8sClient("", fKubeClient)
	// 			Expect(err).NotTo(HaveOccurred())
	// 			// k8sArgs, err := GetK8sArgs(args)
	// 			Expect(err).NotTo(HaveOccurred())
	// 			// pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
	// 			networks, err := GetPodNetwork(fakePod)
	// 			Expect(err).NotTo(HaveOccurred())

	// 			meme, err := kubeClient.GetRawWithPath("/apis/k8s.cni.cncf.io/v1/namespaces/test/network-attachment-definitions/net1")
	// 			fmt.Println("MEME: ", meme)

	// 			_, err = GetNetworkDelegates(kubeClient, fakePod, networks, tmpDir, false)
	// 			Expect(err).NotTo(HaveOccurred())
	// 			// Expect(fKubeClient.PodCount).To(Equal(1))
	// 			// Expect(fKubeClient.NetCount).To(Equal(2))

	// 			// Expect(len(delegates)).To(Equal(2))
	// 			// Expect(delegates[0].Conf.Name).To(Equal("net1"))
	// 			// Expect(delegates[0].Conf.Type).To(Equal("mynet"))
	// 			// Expect(delegates[0].MasterPlugin).To(BeFalse())
	// 			// Expect(delegates[1].Conf.Name).To(Equal("net2"))
	// 			// Expect(delegates[1].Conf.Type).To(Equal("mynet2"))
	// 			// Expect(delegates[1].MasterPlugin).To(BeFalse())
	// 		})
	// 	})

	Context("parsePodNetworkObjectName", func() {
		It("fails to get podnetwork given bad annotation values", func() {
			fakePod := testutils.NewFakePod("testpod", "net1", "")
			args := &skel.CmdArgs{
				Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			}

			fKubeClient := testutils.NewFakeKubeClient()
			fKubeClient.AddPod(fakePod)
			fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net1", "{\"type\": \"mynet\"}")

			kubeClient, err := GetK8sClient("", fKubeClient)
			Expect(err).NotTo(HaveOccurred())
			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())
			pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))

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

			fKubeClient := testutils.NewFakeKubeClient()
			fKubeClient.AddPod(fakePod)
			fKubeClient.AddNetConfig("kube-system", "net1", net1)

			kubeClient, err := GetK8sClient("", fKubeClient)
			Expect(err).NotTo(HaveOccurred())
			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
			Expect(err).NotTo(HaveOccurred())

			networkstatus := "test status"
			_, err = setPodNetworkAnnotation(kubeClient, "test", pod, networkstatus)
			Expect(err).NotTo(HaveOccurred())
		})

		// Still figuring this next one out. deals with exponentialBackoff
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

		// 	fKubeClient := testutils.NewFakeKubeClient()
		// 	fKubeClient.AddPod(fakePod)
		// 	fKubeClient.AddNetConfig("kube-system", "net1", net1)

		// 	kubeClient, err := GetK8sClient("", fKubeClient)
		// 	Expect(err).NotTo(HaveOccurred())
		// 	k8sArgs, err := GetK8sArgs(args)
		// 	Expect(err).NotTo(HaveOccurred())

		// 	pod, err := kubeClient.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
		// 	Expect(err).NotTo(HaveOccurred())

		// 	networkstatus := "test status"
		// 	_, err = setPodNetworkAnnotation(kubeClient, "test", pod, networkstatus)
		// 	Expect(err).NotTo(HaveOccurred())
		// })
	})

	Context("SetNetworkStatus", func() {
		It("Sets network status without error", func() {
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

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := types.LoadNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []*types.NetworkStatus{delegateNetStatus}

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

			fKubeClient := testutils.NewFakeKubeClient()
			fKubeClient.AddPod(fakePod)
			fKubeClient.AddNetConfig("kube-system", "net1", net1)

			kubeClient, err := GetK8sClient("", fKubeClient)
			Expect(err).NotTo(HaveOccurred())
			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(kubeClient, k8sArgs, netstatus, netConf)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Sets network status with kubeclient built from kubeconfig and attempts to connect", func() {
			result := &types020.Result{
				CNIVersion: "0.2.0",
				IP4: &types020.IPConfig{
					IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
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

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := types.LoadNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []*types.NetworkStatus{delegateNetStatus}

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

			fKubeClient := testutils.NewFakeKubeClient()
			fKubeClient.AddPod(fakePod)
			fKubeClient.AddNetConfig("kube-system", "net1", net1)

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(nil, k8sArgs, netstatus, netConf)
			Expect(err).To(HaveOccurred())
		})

		It("Fails to set network status without kubeclient or kubeconfig", func() {
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
			// note that the provided kubeconfig is invalid

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := types.LoadNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []*types.NetworkStatus{delegateNetStatus}

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

			fKubeClient := testutils.NewFakeKubeClient()
			fKubeClient.AddPod(fakePod)
			fKubeClient.AddNetConfig("kube-system", "net1", net1)

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(nil, k8sArgs, netstatus, netConf)
			Expect(err).To(HaveOccurred())
		})

		It("Skips network status given no config", func() {
			result := &types020.Result{
				CNIVersion: "0.2.0",
				IP4: &types020.IPConfig{
					IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
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

			delegate, err := types.LoadDelegateNetConf([]byte(conf), nil, "0000:00:00.0")
			Expect(err).NotTo(HaveOccurred())

			delegateNetStatus, err := types.LoadNetworkStatus(result, delegate.Conf.Name, delegate.MasterPlugin)
			GinkgoT().Logf("delegateNetStatus %+v\n", delegateNetStatus)
			Expect(err).NotTo(HaveOccurred())

			netstatus := []*types.NetworkStatus{delegateNetStatus}

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

			fKubeClient := testutils.NewFakeKubeClient()
			fKubeClient.AddPod(fakePod)
			fKubeClient.AddNetConfig("kube-system", "net1", net1)

			k8sArgs, err := GetK8sArgs(args)
			Expect(err).NotTo(HaveOccurred())

			err = SetNetworkStatus(nil, k8sArgs, netstatus, netConf)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
