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

package multus

import (
	"bytes"
	"context"
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
	types020 "github.com/containernetworking/cni/pkg/types/020"
	current "github.com/containernetworking/cni/pkg/types/current"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"gopkg.in/intel/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/intel/multus-cni.v3/pkg/logging"
	testhelpers "gopkg.in/intel/multus-cni.v3/pkg/testing"
	"gopkg.in/intel/multus-cni.v3/pkg/types"
	netfake "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

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
	cniversion.PluginDecoder

	addIndex int
	delIndex int
	chkIndex int
	plugins  []*fakePlugin
}

func (f *fakeExec) addPlugin(expectedEnv []string, expectedIfname, expectedConf string, result *current.Result, err error) {
	f.plugins = append(f.plugins, &fakePlugin{
		expectedEnv:    expectedEnv,
		expectedConf:   expectedConf,
		expectedIfname: expectedIfname,
		result:         result,
		err:            err,
	})
}

func (f *fakeExec) addPlugin020(expectedEnv []string, expectedIfname, expectedConf string, result *types020.Result, err error) {
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

func (f *fakeExec) ExecPlugin(ctx context.Context, pluginPath string, stdinData []byte, environ []string) ([]byte, error) {
	cmd := os.Getenv("CNI_COMMAND")
	var index int
	var err error
	var resultJSON []byte

	switch cmd {
	case "ADD":
		Expect(len(f.plugins)).To(BeNumerically(">", f.addIndex))
		index = f.addIndex
		f.addIndex++
	case "CHECK":
		Expect(len(f.plugins)).To(BeNumerically("==", f.addIndex))
		index = f.chkIndex
		f.chkIndex++
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

	// strip prevResult from stdinData; tests don't need it
	var m map[string]interface{}
	reader := strings.NewReader(string(stdinData))
	writer := new(bytes.Buffer)

	dec := json.NewDecoder(reader)
	enc := json.NewEncoder(writer)
	err = dec.Decode(&m)
	Expect(err).NotTo(HaveOccurred())
	for k := range m {
		if k == "prevResult" {
			delete(m, k)
		}
	}
	err = enc.Encode(&m)
	Expect(err).NotTo(HaveOccurred())

	if plugin.expectedConf != "" {
		Expect(writer).To(MatchJSON(plugin.expectedConf))
	}
	if plugin.expectedIfname != "" {
		Expect(os.Getenv("CNI_IFNAME")).To(Equal(plugin.expectedIfname))
	}

	if len(plugin.expectedEnv) > 0 {
		cniEnv := gatherCNIEnv()
		for _, expectedCniEnvVar := range plugin.expectedEnv {
			Expect(cniEnv).Should(ContainElement(expectedCniEnvVar))
		}
	}

	if plugin.err != nil {
		return nil, plugin.err
	}

	resultJSON, err = json.Marshal(plugin.result)
	Expect(err).NotTo(HaveOccurred())
	return resultJSON, nil
}

func (f *fakeExec) FindInPath(plugin string, paths []string) (string, error) {
	Expect(len(paths)).To(BeNumerically(">", 0))
	return filepath.Join(paths[0], plugin), nil
}

// NewFakeClientInfo returns fake client (just for testing)
func NewFakeClientInfo() *k8sclient.ClientInfo {
	return &k8sclient.ClientInfo{
		Client:        fake.NewSimpleClientset(),
		NetClient:     netfake.NewSimpleClientset().K8sCniCncfIoV1(),
		EventRecorder: record.NewFakeRecorder(10),
	}
}

func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case ev := <-source:
			events = append(events, ev)
		default:
			done = true
		}
	}
	return events
}

var _ = Describe("multus operations cniVersion 0.2.0 config", func() {
	var testNS ns.NetNS
	var tmpDir string
	resultCNIVersion := "0.4.0"

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

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("executes delegates given faulty namespace", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       "fsdadfad",
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
		// Netns is given garbage value

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("returns the previous result using CmdCheck", func() {
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

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		// Check is not supported until v 0.4.0
		err = CmdCheck(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("executes delegates given faulty namespace", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       "fsdadfad",
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
		// Netns is given garbage value

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("returns the previous result using CmdCheck", func() {
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

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		// Check is not supported until v 0.4.0
		err = CmdCheck(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("executes delegates given faulty namespace", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       "fsdadfad",
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
		// Netns is given garbage value
		fmt.Println("args.Netns: ", args.Netns)

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("returns the previous result using CmdCheck", func() {
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

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		// Check is not supported until v 0.4.0
		err = CmdCheck(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("executes delegates given faulty namespace", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       "fsdadfad",
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
		// Netns is given garbage value

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("returns the previous result using CmdCheck", func() {
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

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		// Check is not supported until v 0.4.0
		err = CmdCheck(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("executes delegates given faulty namespace", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       "fsdadfad",
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
		// Netns is given garbage value

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("returns the previous result using CmdCheck", func() {
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

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		// Check is not supported until v 0.4.0
		err = CmdCheck(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("fails to load NetConf with bad json in CmdAdd/Del", func() {
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
	`),
		}
		// Missing close bracket in StdinData

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err := CmdAdd(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		err = CmdDel(args, fExec, nil)
		Expect(err).To(HaveOccurred())
	})

	It("fails to save NetConf with bad filepath", func() {
		meme := []byte(`meme`)
		err := saveScratchNetConf("123456789", "", meme)
		Expect(err).To(HaveOccurred())
	})

	It("fails to delete delegates with bad filepath", func() {
		err := deleteDelegates("123456789", "bad!file!~?Path$^")
		Expect(err).To(HaveOccurred())
	})

	It("delete delegates given good filepath", func() {
		os.MkdirAll("/opt/cni/bin", 0755)
		d1 := []byte("blah")
		ioutil.WriteFile("/opt/cni/bin/123456789", d1, 0644)

		err := deleteDelegates("123456789", "/opt/cni/bin")
		Expect(err).NotTo(HaveOccurred())
	})

	It("executes delegates and cleans up on failure", func() {
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.2.0",
	    "type": "weave-net"
	}`
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.2.0",
	    "type": "other-plugin"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "defaultnetworkfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [%s,%s]
	}`, expectedConf1, expectedConf2)),
		}

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

		// This plugin invocation should fail
		err := fmt.Errorf("expected plugin failure")
		fExec.addPlugin020(nil, "net1", expectedConf2, nil, err)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(2))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(MatchError("[/:other1]: error adding container to network \"other1\": expected plugin failure"))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}

	})

	It("executes delegates and cleans up on failure with missing name field", func() {
		expectedConf1 := `{
		    "name": "weave1",
		    "cniVersion": "0.2.0",
		    "type": "weave-net"
		}`
		expectedConf2 := `{
		    "name": "",
		    "cniVersion": "0.2.0",
		    "type": "other-plugin"
		}`
		// took out the name in expectedConf2, expecting a new value to be filled in by CmdAdd

		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(fmt.Sprintf(`{
		    "name": "node-cni-network",
		    "type": "multus",
		    "defaultnetworkfile": "/tmp/foo.multus.conf",
		    "defaultnetworkwaitseconds": 3,
		    "delegates": [%s,%s]
		}`, expectedConf1, expectedConf2)),
		}

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

		// This plugin invocation should fail
		err := fmt.Errorf("expected plugin failure")
		fExec.addPlugin020(nil, "net1", expectedConf2, nil, err)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(2))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(HaveOccurred())

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}

	})

	It("executes delegates with runtimeConfigs", func() {
		podNet := `[{"name":"net1",
                             "mac": "c2:11:22:33:44:66",
                             "ips": [ "10.0.0.1" ],
                             "bandwidth": {
				     "ingressRate": 2048,
				     "ingressBurst": 1600,
				     "egressRate": 4096,
				     "egressBurst": 1600
			     },
			     "portMappings": [
			     {
				     "hostPort": 8080, "containerPort": 80, "protocol": "tcp"
			     },
			     {
				     "hostPort": 8000, "containerPort": 8001, "protocol": "udp"
			     }]
		     }
	]`
		fakePod := testhelpers.NewFakePod("testpod", podNet, "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"capabilities": {"mac": true, "ips": true, "bandwidth": true, "portMappings": true},
		"cniVersion": "0.3.1"
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
	        "cniVersion": "0.3.1",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: resultCNIVersion,
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.3.1",
	    "type": "weave-net"
	}`
		expectedNet1 := `{
		"name": "net1",
		"type": "mynet",
		"capabilities": {
			"mac": true,
			"ips": true,
			"bandwidth": true,
			"portMappings": true
		},
		"runtimeConfig": {
			"ips": [ "10.0.0.1" ],
			"mac": "c2:11:22:33:44:66",
			"bandwidth": {
				"ingressRate": 2048,
				"ingressBurst": 1600,
				"egressRate": 4096,
				"egressBurst": 1600
			},
			"portMappings": [
			{
				"hostPort": 8080,
				"containerPort": 80,
				"protocol": "tcp"
			},
			{
				"hostPort": 8000,
				"containerPort": 8001,
				"protocol": "udp"
			}]
		},
		"cniVersion": "0.3.1"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", expectedNet1, &current.Result{
			CNIVersion: "0.3.1",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Create(
			context.TODO(), fakePod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*current.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())
	})

	It("executes delegates and kubernetes networks with events check", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1,net2", "")
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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin020(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)
		fExec.addPlugin020(nil, "net2", net2, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.4/24"),
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Create(
			context.TODO(), fakePod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net2", net2))
		Expect(err).NotTo(HaveOccurred())
		// net3 is not used; make sure it's not accessed
		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net3", net3))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		recorder := clientInfo.EventRecorder.(*record.FakeRecorder)
		events := collectEvents(recorder.Events)
		Expect(len(events)).To(Equal(3))
		Expect(events[0]).To(Equal("Normal AddedInterface Add eth0 [1.1.1.2/24]"))
		Expect(events[1]).To(Equal("Normal AddedInterface Add net1 [1.1.1.3/24] from test/net1"))
		Expect(events[2]).To(Equal("Normal AddedInterface Add net2 [1.1.1.4/24] from test/net2"))
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin020(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Create(
			context.TODO(), fakePod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")

		// delete pod to emulate no pod info
		clientInfo.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		nilPod, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Get(
			context.TODO(), fakePod.ObjectMeta.Name, metav1.GetOptions{})
		Expect(nilPod).To(BeNil())
		Expect(errors.IsNotFound(err)).To(BeTrue())

		err = CmdDel(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin020(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		// set fKubeClient to nil to emulate no pod info
		fKubeClient.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin020(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		// set fKubeClient to nil to emulate no pod info
		fKubeClient.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("ensure delegates get portmap runtime config", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "delegates": [{
	        "cniVersion": "0.3.1",
	        "name": "mynet-confList",
			"plugins": [
				{
					"type": "firstPlugin",
	                "capabilities": {"portMappings": true}
	            }
			]
		}],
		"runtimeConfig": {
	        "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`),
		}

		fExec := &fakeExec{}
		expectedConf1 := `{
	    "capabilities": {"portMappings": true},
		"name": "mynet-confList",
	    "cniVersion": "0.3.1",
	    "type": "firstPlugin",
	    "runtimeConfig": {
		    "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`
		fExec.addPlugin020(nil, "eth0", expectedConf1, nil, nil)
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("executes clusterNetwork delegate", func() {
		fakePod := testhelpers.NewFakePod("testpod", "", "kube-system/net1")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.2.0"
	}`
		expectedResult1 := &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
		}
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "defaultNetworks": [],
	    "clusterNetwork": "net1",
	    "delegates": []
	}`),
		}

		fExec := &fakeExec{}
		fExec.addPlugin020(nil, "eth0", net1, expectedResult1, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(testhelpers.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("Verify the cache is created in dataDir", func() {
		tmpCNIDir := tmpDir + "/cniData"
		err := os.Mkdir(tmpCNIDir, 0777)
		Expect(err).NotTo(HaveOccurred())

		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.2.0"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "cniDir": "%s",
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "0.2.0",
	        "type": "weave-net"
	    }]
	}`, tmpCNIDir)),
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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin020(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err = fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		By("Verify cache file existence")
		cacheFilePath := fmt.Sprintf("%s/%s", tmpCNIDir, "123456789")
		_, err = os.Stat(cacheFilePath)
		Expect(err).NotTo(HaveOccurred())

		By("Delete and check net count is not incremented")
		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("Delete pod without cache", func() {
		tmpCNIDir := tmpDir + "/cniData"
		err := os.Mkdir(tmpCNIDir, 0777)
		Expect(err).NotTo(HaveOccurred())

		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.2.0"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "cniDir": "%s",
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "0.2.0",
	        "type": "weave-net"
	    }]
	}`, tmpCNIDir)),
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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin020(nil, "net1", net1, &types020.Result{
			CNIVersion: "0.2.0",
			IP4: &types020.IPConfig{
				IP: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err = fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		By("Verify cache file existence")
		cacheFilePath := fmt.Sprintf("%s/%s", tmpCNIDir, "123456789")
		_, err = os.Stat(cacheFilePath)
		Expect(err).NotTo(HaveOccurred())

		err = os.Remove(cacheFilePath)
		Expect(err).NotTo(HaveOccurred())

		By("Delete and check pod/net count is incremented")
		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("fails to execute confListDel given no 'plugins' key", func() {
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

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

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
		fExec.addPlugin020(nil, "eth0", expectedConf1, expectedResult1, nil)

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
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")

		binDir := "/opt/cni/bin"
		// use fExec for the exec param
		rawnetconflist := []byte(`{"cniVersion":"0.2.0","name":"weave1","type":"weave-net"}`)
		k8sargs, err := k8sclient.GetK8sArgs(args)
		n, err := types.LoadNetConf(args.StdinData)
		rt, _ := types.CreateCNIRuntimeConf(args, k8sargs, args.IfName, n.RuntimeConfig, nil)

		err = conflistDel(rt, rawnetconflist, binDir, fExec)
		Expect(err).To(HaveOccurred())
	})

	It("executes confListDel without error", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "delegates": [{
	        "cniVersion": "0.3.1",
	        "name": "mynet-confList",
			"plugins": [
				{
					"type": "firstPlugin",
	                "capabilities": {"portMappings": true}
	            }
			]
		}],
		"runtimeConfig": {
	        "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`),
		}

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedConf1 := `{
	    "capabilities": {"portMappings": true},
		"name": "mynet-confList",
	    "cniVersion": "0.3.1",
	    "type": "firstPlugin",
	    "runtimeConfig": {
		    "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`
		fExec.addPlugin020(nil, "eth0", expectedConf1, nil, nil)
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("multus operations cniVersion 0.4.0 config", func() {
	var testNS ns.NetNS
	var tmpDir string
	resultCNIVersion := "0.4.0"

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

	It("executes delegates with CNI Check", func() {
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "0.4.0",
	        "type": "other-plugin"
	    }]
	}`),
		}

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.4.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "CHECK")
		err = CmdCheck(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("executes delegates given faulty namespace", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       "fsdadfad",
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "defaultnetworkfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "0.4.0",
	        "type": "other-plugin"
	    }]
	}`),
		}
		// Netns is given garbage value

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.4.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("returns the previous result using CmdCheck", func() {
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
	        "cniVersion": "0.4.0",
		"plugins": [{
	        "type": "weave-net",
	        "cniVersion": "0.4.0",
		"name": "weave-net-name"
		}]
	    },{
	        "name": "other1",
	        "cniVersion": "0.4.0",
		"plugins": [{
	        "type": "other-plugin",
	        "cniVersion": "0.4.0",
		"name": "other-name"
		}]
	    }]
	}`),
		}

		logging.SetLogLevel("verbose")

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.4.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "CHECK")
		err = CmdCheck(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}
	})

	It("fails to load NetConf with bad json in CmdAdd/Del", func() {
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "0.4.0",
	        "type": "other-plugin"
	    }]
	`),
		}
		// Missing close bracket in StdinData

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.4.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err := CmdAdd(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		err = CmdDel(args, fExec, nil)
		Expect(err).To(HaveOccurred())
	})

	It("executes delegates and cleans up on failure", func() {
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.4.0",
	    "type": "other-plugin"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "defaultnetworkfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [%s,%s]
	}`, expectedConf1, expectedConf2)),
		}

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		// This plugin invocation should fail
		err := fmt.Errorf("expected plugin failure")
		fExec.addPlugin(nil, "net1", expectedConf2, nil, err)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(2))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(MatchError("[/:other1]: error adding container to network \"other1\": expected plugin failure"))

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}

	})

	It("executes delegates and cleans up on failure with missing name field", func() {
		expectedConf1 := `{
		    "name": "weave1",
		    "cniVersion": "0.4.0",
		    "type": "weave-net"
		}`
		expectedConf2 := `{
		    "name": "",
		    "cniVersion": "0.4.0",
		    "type": "other-plugin"
		}`
		// took out the name in expectedConf2, expecting a new value to be filled in by CmdAdd

		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(fmt.Sprintf(`{
		    "name": "node-cni-network",
		    "type": "multus",
		    "defaultnetworkfile": "/tmp/foo.multus.conf",
		    "defaultnetworkwaitseconds": 3,
		    "delegates": [%s,%s]
		}`, expectedConf1, expectedConf2)),
		}

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		// This plugin invocation should fail
		err := fmt.Errorf("expected plugin failure")
		fExec.addPlugin(nil, "net1", expectedConf2, nil, err)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(2))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(HaveOccurred())

		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}

	})

	It("executes delegates with runtimeConfigs", func() {
		podNet := `[{"name":"net1",
                             "mac": "c2:11:22:33:44:66",
                             "ips": [ "10.0.0.1" ],
                             "bandwidth": {
				     "ingressRate": 2048,
				     "ingressBurst": 1600,
				     "egressRate": 4096,
				     "egressBurst": 1600
			     },
			     "portMappings": [
			     {
				     "hostPort": 8080, "containerPort": 80, "protocol": "tcp"
			     },
			     {
				     "hostPort": 8000, "containerPort": 8001, "protocol": "udp"
			     }]
		     }
	]`
		fakePod := testhelpers.NewFakePod("testpod", podNet, "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"capabilities": {"mac": true, "ips": true, "bandwidth": true, "portMappings": true},
		"cniVersion": "0.4.0"
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: resultCNIVersion,
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		expectedNet1 := `{
		"name": "net1",
		"type": "mynet",
		"capabilities": {
			"mac": true,
			"ips": true,
			"bandwidth": true,
			"portMappings": true
		},
		"runtimeConfig": {
			"ips": [ "10.0.0.1" ],
			"mac": "c2:11:22:33:44:66",
			"bandwidth": {
				"ingressRate": 2048,
				"ingressBurst": 1600,
				"egressRate": 4096,
				"egressBurst": 1600
			},
			"portMappings": [
			{
				"hostPort": 8080,
				"containerPort": 80,
				"protocol": "tcp"
			},
			{
				"hostPort": 8000,
				"containerPort": 8001,
				"protocol": "udp"
			}]
		},
		"cniVersion": "0.4.0"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", expectedNet1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Create(
			context.TODO(), fakePod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*current.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

	})

	It("executes delegates and kubernetes networks", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1,net2", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
	}`
		net2 := `{
		"name": "net2",
		"type": "mynet2",
		"cniVersion": "0.4.0"
	}`
		net3 := `{
		"name": "net3",
		"type": "mynet3",
		"cniVersion": "0.4.0"
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)
		fExec.addPlugin(nil, "net2", net2, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.4/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Create(
			context.TODO(), fakePod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net2", net2))
		Expect(err).NotTo(HaveOccurred())
		// net3 is not used; make sure it's not accessed
		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net3", net3))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.Client.CoreV1().Pods(fakePod.ObjectMeta.Namespace).Create(
			context.TODO(), fakePod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		// set fKubeClient to nil to emulate no pod info
		clientInfo.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		err = CmdDel(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		// set fKubeClient to nil to emulate no pod info
		fKubeClient.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		// set fKubeClient to nil to emulate no pod info
		fKubeClient.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("ensure delegates get portmap runtime config", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "delegates": [{
	        "cniVersion": "0.4.0",
	        "name": "mynet-confList",
			"plugins": [
				{
					"type": "firstPlugin",
	                "capabilities": {"portMappings": true}
	            }
			]
		}],
		"runtimeConfig": {
	        "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`),
		}

		fExec := &fakeExec{}
		expectedConf1 := `{
	    "capabilities": {"portMappings": true},
		"name": "mynet-confList",
	    "cniVersion": "0.4.0",
	    "type": "firstPlugin",
	    "runtimeConfig": {
		    "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, nil, nil)
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		_, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("executes clusterNetwork delegate", func() {
		fakePod := testhelpers.NewFakePod("testpod", "", "kube-system/net1")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
	}`
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "defaultNetworks": [],
	    "clusterNetwork": "net1",
	    "delegates": []
	}`),
		}

		fExec := &fakeExec{}
		fExec.addPlugin(nil, "eth0", net1, expectedResult1, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(testhelpers.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("Verify the cache is created in dataDir", func() {
		tmpCNIDir := tmpDir + "/cniData"
		err := os.Mkdir(tmpCNIDir, 0777)
		Expect(err).NotTo(HaveOccurred())

		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "cniDir": "%s",
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`, tmpCNIDir)),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err = fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		By("Verify cache file existence")
		cacheFilePath := fmt.Sprintf("%s/%s", tmpCNIDir, "123456789")
		_, err = os.Stat(cacheFilePath)
		Expect(err).NotTo(HaveOccurred())

		By("Delete and check net count is not incremented")
		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("Delete pod without cache", func() {
		tmpCNIDir := tmpDir + "/cniData"
		err := os.Mkdir(tmpCNIDir, 0777)
		Expect(err).NotTo(HaveOccurred())

		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "0.4.0"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			Args:        fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s", fakePod.ObjectMeta.Name, fakePod.ObjectMeta.Namespace),
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "kubeconfig": "/etc/kubernetes/node-kubeconfig.yaml",
	    "cniDir": "%s",
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    }]
	}`, tmpCNIDir)),
		}

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin(nil, "net1", net1, &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err = fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")
		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		By("Verify cache file existence")
		cacheFilePath := fmt.Sprintf("%s/%s", tmpCNIDir, "123456789")
		_, err = os.Stat(cacheFilePath)
		Expect(err).NotTo(HaveOccurred())

		err = os.Remove(cacheFilePath)
		Expect(err).NotTo(HaveOccurred())

		By("Delete and check pod/net count is incremented")
		os.Setenv("CNI_COMMAND", "DEL")
		os.Setenv("CNI_IFNAME", "eth0")
		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("fails to execute confListDel given no 'plugins' key", func() {
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
	        "cniVersion": "0.4.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "0.4.0",
	        "type": "other-plugin"
	    }]
	}`),
		}

		// Touch the default network file.
		configPath := "/tmp/foo.multus.conf"
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)

		fExec := &fakeExec{}
		expectedResult1 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "0.4.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &current.Result{
			CNIVersion: "0.4.0",
			IPs: []*current.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.4.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin(nil, "net1", expectedConf2, expectedResult2, nil)

		os.Setenv("CNI_COMMAND", "ADD")
		os.Setenv("CNI_IFNAME", "eth0")

		binDir := "/opt/cni/bin"
		// use fExec for the exec param
		rawnetconflist := []byte(`{"cniVersion":"0.4.0","name":"weave1","type":"weave-net"}`)
		k8sargs, err := k8sclient.GetK8sArgs(args)
		n, err := types.LoadNetConf(args.StdinData)
		rt, _ := types.CreateCNIRuntimeConf(args, k8sargs, args.IfName, n.RuntimeConfig, nil)

		err = conflistDel(rt, rawnetconflist, binDir, fExec)
		Expect(err).To(HaveOccurred())
	})
})
