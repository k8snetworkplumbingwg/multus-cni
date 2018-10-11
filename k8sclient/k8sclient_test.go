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

	testutils "github.com/intel/multus-cni/testing"

	"github.com/containernetworking/cni/pkg/skel"

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
		fakePod := testutils.NewFakePod("testpod", "net1,net2")
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
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
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
		fakePod := testutils.NewFakePod("testpod", "net1,net2")
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
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
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
]`)
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
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
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
		fakePod := testutils.NewFakePod("testpod", "[adsfasdfasdfasf]")
		args := &skel.CmdArgs{
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
		}

		fKubeClient := testutils.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)

		kubeClient, err := GetK8sClient("", fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		k8sArgs, err := GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
		Expect(len(delegates)).To(Equal(0))
		Expect(err).To(MatchError("parsePodNetworkAnnotation: failed to parse pod Network Attachment Selection Annotation JSON format: invalid character 'a' looking for beginning of value"))
	})

	It("retrieves delegates from kubernetes using on-disk config files", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2")
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
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
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
		fakePod := testutils.NewFakePod("testpod", "net1")
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
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(fKubeClient.PodCount).To(Equal(1))
		Expect(fKubeClient.NetCount).To(Equal(1))

		Expect(len(delegates)).To(Equal(1))
		Expect(delegates[0].Conf.Name).To(Equal("net1"))
		Expect(delegates[0].Conf.Type).To(Equal("mynet"))
	})

	It("fails when on-disk config file is not valid", func() {
		fakePod := testutils.NewFakePod("testpod", "net1,net2")
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
		delegates, err := GetPodNetwork(kubeClient, k8sArgs, tmpDir)
		Expect(len(delegates)).To(Equal(0))
		Expect(err).To(MatchError(fmt.Sprintf("GetPodNetwork: failed getting the delegate: cniConfigFromNetworkResource: err in getCNIConfigFromFile: Error loading CNI config file %s: error parsing configuration: invalid character 'a' looking for beginning of value", net2Name)))
	})
})
