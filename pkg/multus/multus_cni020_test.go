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

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/containernetworking/cni/pkg/skel"
	types020 "github.com/containernetworking/cni/pkg/types/020"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	testhelpers "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/testing"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("multus operations", func() {
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
		os.WriteFile("/opt/cni/bin/123456789", d1, 0644)

		err := deleteDelegates("123456789", "/opt/cni/bin")
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("multus operations cniVersion 0.2.0 config", func() {
	var testNS ns.NetNS
	var tmpDir string
	configPath := "/tmp/foo.multus.conf"

	BeforeEach(func() {
		// Create a new NetNS so we don't modify the host
		var err error
		testNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		os.Setenv("CNI_NETNS", testNS.Path())
		os.Setenv("CNI_PATH", "/some/path")

		tmpDir, err = os.MkdirTemp("", "multus_tmp")
		Expect(err).NotTo(HaveOccurred())

		// Touch the default network file.
		os.OpenFile(configPath, os.O_RDONLY|os.O_CREATE, 0755)
	})

	AfterEach(func() {
		// Cleanup default network file.
		if _, errStat := os.Stat(configPath); errStat == nil {
			errRemove := os.Remove(configPath)
			Expect(errRemove).NotTo(HaveOccurred())
		}

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

		fExec := newFakeExec()
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

		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes delegates (plugin without interface)", func() {
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

		fExec := newFakeExec()
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

		// other1 just returns empty result
		expectedResult2 := &types020.Result{
			CNIVersion: "0.2.0",
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "0.2.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin020(nil, "net1", expectedConf2, expectedResult2, nil)

		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
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

		fExec := newFakeExec()
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

		_, err := CmdAdd(args, fExec, nil)
		Expect(err).To(MatchError("[//:weave1]: error adding container to network \"weave1\": DelegateAdd: cannot set \"weave-net\" interface name to \"eth0\": validateIfName: no net namespace fsdadfad found: failed to Statfs \"fsdadfad\": no such file or directory"))
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

		fExec := newFakeExec()
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

		_, err := CmdAdd(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		err = CmdDel(args, fExec, nil)
		Expect(err).To(HaveOccurred())
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

		fExec := newFakeExec()
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

		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(2))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(MatchError("[//:other1]: error adding container to network \"other1\": expected plugin failure"))

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

		fExec := newFakeExec()
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

		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(1))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(HaveOccurred())
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

		fExec := newFakeExec()
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

		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		recorder := clientInfo.EventRecorder.(*record.FakeRecorder)
		events := collectEvents(recorder.Events)
		Expect(len(events)).To(Equal(3))
		Expect(events[0]).To(Equal("Normal AddedInterface Add eth0 [1.1.1.2/24] from weave1"))
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

		fExec := newFakeExec()
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
		_, err := fKubeClient.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())
		_, err = fKubeClient.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

		// set fKubeClient to nil to emulate no pod info
		err = fKubeClient.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		Expect(err).NotTo(HaveOccurred())

		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
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

		fExec := newFakeExec()
		fExec.addPlugin020(nil, "eth0", net1, expectedResult1, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(testhelpers.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*types020.Result)
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

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

		fExec := newFakeExec()
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

		fExec := newFakeExec()
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

		fExec := newFakeExec()
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

		fakeMultusNetConf := types.NetConf{
			BinDir: "/opt/cni/bin",
		}
		// use fExec for the exec param
		rawnetconflist := []byte(`{"cniVersion":"0.2.0","name":"weave1","type":"weave-net"}`)
		k8sargs, err := k8sclient.GetK8sArgs(args)
		n, err := types.LoadNetConf(args.StdinData)
		rt, _ := types.CreateCNIRuntimeConf(args, k8sargs, args.IfName, n.RuntimeConfig, nil)

		err = conflistDel(rt, rawnetconflist, &fakeMultusNetConf, fExec)
		Expect(err).To(HaveOccurred())
	})

})
