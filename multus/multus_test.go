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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"

	testhelpers "github.com/intel/multus-cni/testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMultus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "multus")
}

type fakePlugin struct {
	expectedEnv    []string
	expectedConf   string
	expectedIfname string
	result         cnitypes.Result
	err            error
}

type fakeExec struct {
	version.PluginDecoder

	addIndex int
	delIndex int
	plugins  []*fakePlugin
}

func (f *fakeExec) addPlugin(expectedEnv []string, expectedIfname, expectedConf string, result *types020.Result, err error) {
	f.plugins = append(f.plugins, &fakePlugin{
		expectedEnv:    expectedEnv,
		expectedConf:   expectedConf,
		expectedIfname: expectedIfname,
		result:         result,
		err:            err,
	})
}

func matchArray(a1, a2 []string) {
	Expect(len(a1)).To(Equal(len(a2)))
	for _, e1 := range a1 {
		found := ""
		for _, e2 := range a2 {
			if e1 == e2 {
				found = e2
				break
			}
		}
		// Compare element values for more descriptive test failure
		Expect(e1).To(Equal(found))
	}
}

// When faking plugin execution the ExecPlugin() call environ is not populated
// (while it would be for real exec). Filter the environment variables for
// CNI-specific ones that testcases will care about.
func gatherCNIEnv() []string {
	filtered := make([]string, 0)
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CNI_") {
			filtered = append(filtered, env)
		}
	}
	return filtered
}

func (f *fakeExec) ExecPlugin(pluginPath string, stdinData []byte, environ []string) ([]byte, error) {
	cmd := os.Getenv("CNI_COMMAND")
	var index int
	switch cmd {
	case "ADD":
		Expect(len(f.plugins)).To(BeNumerically(">", f.addIndex))
		index = f.addIndex
		f.addIndex++
	case "DEL":
		Expect(len(f.plugins)).To(BeNumerically(">", f.delIndex))
		index = len(f.plugins) - f.delIndex - 1
		f.delIndex++
	default:
		// Should never be reached
		Expect(false).To(BeTrue())
	}
	plugin := f.plugins[index]

	GinkgoT().Logf("[%s %d] exec plugin %q found %+v\n", cmd, index, pluginPath, plugin)

	if plugin.expectedConf != "" {
		Expect(string(stdinData)).To(MatchJSON(plugin.expectedConf))
	}
	if plugin.expectedIfname != "" {
		Expect(os.Getenv("CNI_IFNAME")).To(Equal(plugin.expectedIfname))
	}
	if len(plugin.expectedEnv) > 0 {
		matchArray(gatherCNIEnv(), plugin.expectedEnv)
	}

	if plugin.err != nil {
		return nil, plugin.err
	}

	resultJSON, err := json.Marshal(plugin.result)
	Expect(err).NotTo(HaveOccurred())
	return resultJSON, nil
}

func (f *fakeExec) FindInPath(plugin string, paths []string) (string, error) {
	Expect(len(paths)).To(BeNumerically(">", 0))
	return filepath.Join(paths[0], plugin), nil
}

var _ = Describe("multus operations", func() {
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

	It("executes delegates", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
    "name": "node-cni-network",
    "type": "multus",
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

		fExec := &fakeExec{}
		expectedResult1 := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}
		expectedConf1 := `{
    "name": "weave1",
    "cniVersion": "0.2.0",
    "type": "weave-net"
}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
		}
		expectedConf2 := `{
    "name": "other1",
    "cniVersion": "0.2.0",
    "type": "other-plugin"
}`
		fExec.addPlugin(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := cmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = cmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes delegates and kubernetes networks", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1,net2")
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
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(`{
    "name": "node-cni-network",
    "type": "multus",
    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
    "delegates": [{
        "name": "weave1",
        "cniVersion": "0.2.0",
        "type": "weave-net"
    }]
}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}
		expectedConf1 := `{
    "name": "weave1",
    "cniVersion": "0.2.0",
    "type": "weave-net"
}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)
		fExec.addPlugin(nil, "net2", net2, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.4/24"),
			},
		}, nil)

		fKubeClient := testhelpers.NewFakeKubeClient()
		fKubeClient.AddPod(fakePod)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net1", net1)
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net2", net2)
		// net3 is not used; make sure it's not accessed
		fKubeClient.AddNetConfig(fakePod.ObjectMeta.Namespace, "net3", net3)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := cmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		Expect(fKubeClient.PodCount).To(Equal(2))
		Expect(fKubeClient.NetCount).To(Equal(2))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())
	})
})
