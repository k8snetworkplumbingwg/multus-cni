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

package multus

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cni020 "github.com/containernetworking/cni/pkg/types/020"
	cni040 "github.com/containernetworking/cni/pkg/types/040"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	cniversion "github.com/containernetworking/cni/pkg/version"
	netfake "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/fake"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

	. "github.com/onsi/ginkgo/v2"
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

	addIndex        int
	delIndex        int
	chkIndex        int
	expectedDelSkip int
	plugins         map[string]*fakePlugin
}

func newFakeExec() *fakeExec {
	return &fakeExec{
		plugins: map[string]*fakePlugin{},
	}
}

func (f *fakeExec) addPlugin100(expectedEnv []string, expectedIfname, expectedConf string, result *cni100.Result, err error) {
	f.plugins[expectedIfname] = &fakePlugin{
		expectedEnv:    expectedEnv,
		expectedConf:   expectedConf,
		expectedIfname: expectedIfname,
		result:         result,
		err:            err,
	}
	if err != nil && err.Error() == "missing network name" {
		f.expectedDelSkip++
	}
}

func (f *fakeExec) addPlugin040(expectedEnv []string, expectedIfname, expectedConf string, result *cni040.Result, err error) {
	f.plugins[expectedIfname] = &fakePlugin{
		expectedEnv:    expectedEnv,
		expectedConf:   expectedConf,
		expectedIfname: expectedIfname,
		result:         result,
		err:            err,
	}
	if err != nil && err.Error() == "missing network name" {
		f.expectedDelSkip++
	}
}

func (f *fakeExec) addPlugin020(expectedEnv []string, expectedIfname, expectedConf string, result *cni020.Result, err error) {
	f.plugins[expectedIfname] = &fakePlugin{
		expectedEnv:    expectedEnv,
		expectedConf:   expectedConf,
		expectedIfname: expectedIfname,
		result:         result,
		err:            err,
	}
	if err != nil && err.Error() == "missing network name" {
		f.expectedDelSkip++
	}
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
func gatherCNIEnv(environ []string) []string {
	filtered := make([]string, 0)
	for _, v := range environ {
		if strings.HasPrefix(v, "CNI_") {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func ParseEnvironment(environ []string) map[string]string {
	m := map[string]string{}

	for _, e := range environ {
		if e != "" {
			parts := strings.SplitN(e, "=", 2)
			ExpectWithOffset(2, len(parts)).To(Equal(2))
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func (f *fakeExec) ExecPlugin(_ context.Context, pluginPath string, stdinData []byte, environ []string) ([]byte, error) {
	envMap := ParseEnvironment(environ)
	cmd := envMap["CNI_COMMAND"]
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
		index = len(f.plugins) - f.expectedDelSkip - f.delIndex - 1
		f.delIndex++
	default:
		// Should never be reached
		Expect(false).To(BeTrue())
	}
	plugin := f.plugins[envMap["CNI_IFNAME"]]

	//GinkgoT().Logf("[%s %d] exec plugin %q found %+v\n", cmd, index, pluginPath, plugin)
	fmt.Printf("[%s %d] exec plugin %q found %+v\n", cmd, index, pluginPath, plugin)

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
		Expect(envMap["CNI_IFNAME"]).To(Equal(plugin.expectedIfname))
	}

	if len(plugin.expectedEnv) > 0 {
		cniEnv := gatherCNIEnv(environ)
		for _, expectedCniEnvVar := range plugin.expectedEnv {
			Expect(cniEnv).To(ContainElement(expectedCniEnvVar))
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
		Client:         fake.NewSimpleClientset(),
		WatchClient:    fake.NewSimpleClientset(),
		NetClient:      netfake.NewSimpleClientset(),
		NetWatchClient: netfake.NewSimpleClientset(),
		EventRecorder:  record.NewFakeRecorder(10),
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
