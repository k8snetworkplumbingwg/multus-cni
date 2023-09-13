// Copyright (c) 2022 Multus Authors
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

package server

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"

	"github.com/prometheus/client_golang/prometheus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

	netfake "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/fake"
	k8s "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/api"
	testhelpers "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/testing"
)

const suiteName = "Thick CNI architecture"

type fakeExec struct{}

// ExecPlugin executes the plugin
func (fe *fakeExec) ExecPlugin(_ context.Context, _ string, _ []byte, _ []string) ([]byte, error) {
	return []byte("{}"), nil
}

// FindInPath finds in path
func (fe *fakeExec) FindInPath(_ string, _ []string) (string, error) {
	return "", nil
}

// Decode decodes
func (fe *fakeExec) Decode(_ []byte) (version.PluginInfo, error) {
	return nil, nil
}

var _ = Describe(suiteName, func() {
	const thickCNISocketDirPath = "multus-cni-thick-arch-socket-path"

	var thickPluginRunDir string

	BeforeEach(func() {
		var err error
		thickPluginRunDir, err = os.MkdirTemp("", thickCNISocketDirPath)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(thickPluginRunDir)).To(Succeed())
	})

	Context("the directory does *not* exist", func() {
		It("", func() {
			Expect(FilesystemPreRequirements(thickPluginRunDir)).To(Succeed())
		})
	})

	Context("the directory exists beforehand with the correct permissions", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(thickPluginRunDir, 0700)).To(Succeed())
		})

		It("verifies the filesystem requirements of the socket dir", func() {
			Expect(FilesystemPreRequirements(thickPluginRunDir)).To(Succeed())
		})
	})

	Context("CNI operations started from the shim", func() {
		const (
			containerID = "123456789"
			ifaceName   = "eth0"
			podName     = "my-little-pod"
		)

		var (
			cniServer *Server
			K8sClient *k8s.ClientInfo
			netns     ns.NetNS
			ctx       context.Context
			cancel    context.CancelFunc
		)

		BeforeEach(func() {
			var err error
			K8sClient = fakeK8sClient()

			Expect(FilesystemPreRequirements(thickPluginRunDir)).To(Succeed())

			ctx, cancel = context.WithCancel(context.TODO())
			cniServer, err = startCNIServer(ctx, thickPluginRunDir, K8sClient, nil)
			Expect(err).NotTo(HaveOccurred())

			netns, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())

			// the namespace and podUID parameters below are hard-coded in the generation function
			Expect(prepareCNIEnv(netns.Path(), "test", podName, "testUID")).To(Succeed())
			Expect(createFakePod(K8sClient, podName)).To(Succeed())
		})

		AfterEach(func() {
			cancel()
			unregisterMetrics(cniServer)
			Expect(cniServer.Close()).To(Succeed())
			Expect(teardownCNIEnv()).To(Succeed())
			Expect(K8sClient.Client.CoreV1().Pods("test").Delete(
				context.TODO(), podName, metav1.DeleteOptions{}))
		})

		It("ADD/CHECK/DEL works successfully", func() {
			Expect(os.Setenv("CNI_COMMAND", "ADD")).NotTo(HaveOccurred())
			Expect(api.CmdAdd(cniCmdArgs(containerID, netns.Path(), ifaceName, referenceConfig(thickPluginRunDir)))).To(Succeed())

			Expect(os.Setenv("CNI_COMMAND", "CHECK")).NotTo(HaveOccurred())
			Expect(api.CmdCheck(cniCmdArgs(containerID, netns.Path(), ifaceName, referenceConfig(thickPluginRunDir)))).To(Succeed())

			Expect(os.Setenv("CNI_COMMAND", "DEL")).NotTo(HaveOccurred())
			Expect(api.CmdDel(cniCmdArgs(containerID, netns.Path(), ifaceName, referenceConfig(thickPluginRunDir)))).To(Succeed())
		})
	})

	Context("CNI operations started from the shim with CNI config override with server config", func() {
		const (
			containerID = "123456789"
			ifaceName   = "eth0"
			podName     = "my-little-pod"
		)

		var (
			cniServer *Server
			K8sClient *k8s.ClientInfo
			netns     ns.NetNS
			ctx       context.Context
			cancel    context.CancelFunc
		)

		BeforeEach(func() {
			var err error
			K8sClient = fakeK8sClient()

			dummyServerConfig := `{
				"dummy_key1": "dummy_val1",
				"dummy_key2": "dummy_val2"
			}`

			Expect(FilesystemPreRequirements(thickPluginRunDir)).To(Succeed())

			ctx, cancel = context.WithCancel(context.TODO())
			cniServer, err = startCNIServer(ctx, thickPluginRunDir, K8sClient, []byte(dummyServerConfig))
			Expect(err).NotTo(HaveOccurred())

			netns, err = testutils.NewNS()
			Expect(err).NotTo(HaveOccurred())

			// the namespace and podUID parameters below are hard-coded in the generation function
			Expect(prepareCNIEnv(netns.Path(), "test", podName, "testUID")).To(Succeed())
			Expect(createFakePod(K8sClient, podName)).To(Succeed())
		})

		AfterEach(func() {
			cancel()
			unregisterMetrics(cniServer)
			Expect(cniServer.Close()).To(Succeed())
			Expect(teardownCNIEnv()).To(Succeed())
			Expect(K8sClient.Client.CoreV1().Pods("test").Delete(
				context.TODO(), podName, metav1.DeleteOptions{}))
		})

		It("ADD/CHECK/DEL works successfully", func() {
			Expect(os.Setenv("CNI_COMMAND", "ADD")).NotTo(HaveOccurred())
			Expect(api.CmdAdd(cniCmdArgs(containerID, netns.Path(), ifaceName, referenceConfig(thickPluginRunDir)))).To(Succeed())

			Expect(os.Setenv("CNI_COMMAND", "CHECK")).NotTo(HaveOccurred())
			Expect(api.CmdCheck(cniCmdArgs(containerID, netns.Path(), ifaceName, referenceConfig(thickPluginRunDir)))).To(Succeed())

			Expect(os.Setenv("CNI_COMMAND", "DEL")).NotTo(HaveOccurred())
			Expect(api.CmdDel(cniCmdArgs(containerID, netns.Path(), ifaceName, referenceConfig(thickPluginRunDir)))).To(Succeed())

		})
	})
})

func fakeK8sClient() *k8s.ClientInfo {
	const magicNumber = 10
	return &k8s.ClientInfo{
		Client:        fake.NewSimpleClientset(),
		NetClient:     netfake.NewSimpleClientset().K8sCniCncfIoV1(),
		EventRecorder: record.NewFakeRecorder(magicNumber),
	}
}

func cniCmdArgs(containerID string, netnsPath string, ifName string, stdinData string) *skel.CmdArgs {
	return &skel.CmdArgs{
		ContainerID: containerID,
		Netns:       netnsPath,
		IfName:      ifName,
		StdinData:   []byte(stdinData)}
}

func prepareCNIEnv(netnsPath string, namespaceName string, podName string, podUID string) error {
	cniArgs := fmt.Sprintf("K8S_POD_NAMESPACE=%s;K8S_POD_NAME=%s;K8S_POD_INFRA_CONTAINER_ID=;K8S_POD_UID=%s", namespaceName, podName, podUID)
	if err := os.Setenv("CNI_CONTAINERID", "123456789"); err != nil {
		return err
	}
	if err := os.Setenv("CNI_NETNS", netnsPath); err != nil {
		return err
	}
	return os.Setenv("CNI_ARGS", cniArgs)
}

func teardownCNIEnv() error {
	if err := os.Unsetenv("CNI_COMMAND"); err != nil {
		return err
	}
	if err := os.Unsetenv("CNI_CONTAINERID"); err != nil {
		return err
	}
	if err := os.Unsetenv("CNI_NETNS"); err != nil {
		return err
	}
	return os.Unsetenv("CNI_ARGS")
}

func createFakePod(k8sClient *k8s.ClientInfo, podName string) error {
	var err error
	fakePod := testhelpers.NewFakePod(podName, "", "")
	_, err = k8sClient.Client.CoreV1().Pods(fakePod.GetNamespace()).Create(
		context.TODO(), fakePod, metav1.CreateOptions{})
	return err
}

func startCNIServer(ctx context.Context, runDir string, k8sClient *k8s.ClientInfo, servConfig []byte) (*Server, error) {
	const period = 0

	cniServer, err := newCNIServer(runDir, k8sClient, &fakeExec{}, servConfig, true)
	if err != nil {
		return nil, err
	}

	l, err := GetListener(api.SocketPath(runDir))
	if err != nil {
		return nil, fmt.Errorf("failed to start the CNI server using socket %s. Reason: %+v", api.SocketPath(runDir), err)
	}

	cniServer.Start(ctx, l)

	return cniServer, nil
}

// unregisterMetrics unregister registered metrics
// it requires only for unit-testing because unit-tests calls newCNIServer() multiple times
// in unit-testing.
func unregisterMetrics(server *Server) {
	ExpectWithOffset(1, prometheus.Unregister(server.metrics.requestCounter)).To(BeTrue())
}

func referenceConfig(thickPluginSocketDir string) string {
	const referenceConfigTemplate = `{
	"cniVersion": "0.4.0",
        "name": "node-cni-network",
        "type": "multus",
        "daemonSocketDir": "%s",
        "defaultnetworkfile": "/tmp/foo.multus.conf",
        "defaultnetworkwaitseconds": 3,
        "delegates": [{
            "name": "weave1",
            "cniVersion": "0.4.0",
            "type": "weave-net"
        }]}`
	return fmt.Sprintf(referenceConfigTemplate, thickPluginSocketDir)
}
