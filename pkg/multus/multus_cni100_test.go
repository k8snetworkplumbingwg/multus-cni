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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	testhelpers "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/testing"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kapi "k8s.io/api/core/v1"
	informerfactory "k8s.io/client-go/informers"
	v1coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	netdefclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	netdefinformer "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions"
	netdefinformerv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions/k8s.cni.cncf.io/v1"
)

func newPodInformer(ctx context.Context, kclient kubernetes.Interface) cache.SharedIndexInformer {
	informerFactory := informerfactory.NewSharedInformerFactory(kclient, 0*time.Second)

	podInformer := informerFactory.InformerFor(&kapi.Pod{}, func(c kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		return v1coreinformers.NewFilteredPodInformer(
			c,
			kapi.NamespaceAll,
			resyncPeriod,
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
			nil)
	})

	informerFactory.Start(ctx.Done())

	waitCtx, waitCancel := context.WithTimeout(ctx, 20*time.Second)
	if !cache.WaitForCacheSync(waitCtx.Done(), podInformer.HasSynced) {
		logging.Errorf("failed to sync pod informer cache")
	}
	waitCancel()

	return podInformer
}

func newNetDefInformer(ctx context.Context, client netdefclient.Interface) cache.SharedIndexInformer {
	informerFactory := netdefinformer.NewSharedInformerFactory(client, 0*time.Second)

	netdefInformer := informerFactory.InformerFor(&netdefv1.NetworkAttachmentDefinition{}, func(client netdefclient.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
		return netdefinformerv1.NewNetworkAttachmentDefinitionInformer(
			client,
			kapi.NamespaceAll,
			resyncPeriod,
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	})

	informerFactory.Start(ctx.Done())

	waitCtx, waitCancel := context.WithTimeout(ctx, 20*time.Second)
	if !cache.WaitForCacheSync(waitCtx.Done(), netdefInformer.HasSynced) {
		logging.Errorf("failed to sync pod informer cache")
	}
	waitCancel()

	return netdefInformer
}

var _ = Describe("multus operations cniVersion 1.0.0 config", func() {
	var testNS ns.NetNS
	var tmpDir string
	resultCNIVersion := "1.0.0"
	configPath := "/tmp/foo.multus.conf"
	var ctx context.Context
	var cancel context.CancelFunc

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

		ctx, cancel = context.WithCancel(context.TODO())
	})

	AfterEach(func() {
		cancel()

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

	It("executes delegates with CNI Check", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "1.0.0",
	        "type": "other-plugin"
	    }]
	}`),
		}

		logging.SetLogLevel("verbose")

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &cni100.Result{
			CNIVersion: "0.4.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin100(nil, "net1", expectedConf2, expectedResult2, nil)

		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		err = CmdCheck(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

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
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "1.0.0",
	        "type": "other-plugin"
	    }]
	}`),
		}
		// Netns is given garbage value

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin100(nil, "net1", expectedConf2, expectedResult2, nil)

		_, err := CmdAdd(args, fExec, nil)
		Expect(err).To(MatchError("[//:weave1]: error adding container to network \"weave1\": DelegateAdd: cannot set \"weave-net\" interface name to \"eth0\": validateIfName: no net namespace fsdadfad found: failed to Statfs \"fsdadfad\": no such file or directory"))
	})

	It("executes delegates (plugin without interface)", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "1.0.0",
	        "type": "other-plugin"
	    }]
	}`),
		}

		logging.SetLogLevel("verbose")

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		// other1 just returns empty result
		expectedResult2 := &cni100.Result{
			CNIVersion: "0.4.0",
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin100(nil, "net1", expectedConf2, expectedResult2, nil)

		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		err = CmdCheck(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("returns the previous result using CmdCheck", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.0.0",
		"plugins": [{
	        "type": "weave-net",
	        "cniVersion": "1.0.0",
		"name": "weave-net-name"
		}]
	    },{
	        "name": "other1",
	        "cniVersion": "1.0.0",
		"plugins": [{
	        "type": "other-plugin",
	        "cniVersion": "1.0.0",
		"name": "other-name"
		}]
	    }]
	}`),
		}

		logging.SetLogLevel("verbose")

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin100(nil, "net1", expectedConf2, expectedResult2, nil)

		result, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		err = CmdCheck(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())

		err = CmdDel(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("fails to load NetConf with bad json in CmdAdd/Del", func() {
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "1.0.0",
	        "type": "other-plugin"
	    }]
	`),
		}
		// Missing close bracket in StdinData

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin100(nil, "net1", expectedConf2, expectedResult2, nil)

		_, err := CmdAdd(args, fExec, nil)
		Expect(err).To(HaveOccurred())

		err = CmdDel(args, fExec, nil)
		Expect(err).To(HaveOccurred())
	})

	It("executes delegates and cleans up on failure", func() {
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [%s,%s]
	}`, expectedConf1, expectedConf2)),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		// This plugin invocation should fail
		err := fmt.Errorf("expected plugin failure")
		fExec.addPlugin100(nil, "net1", expectedConf2, nil, err)

		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(2))
		Expect(fExec.delIndex).To(Equal(2))
		Expect(err).To(MatchError("[//:other1]: error adding container to network \"other1\": expected plugin failure"))
	})

	It("executes delegates and cleans up on failure with missing name field", func() {
		expectedConf1 := `{
		    "name": "weave1",
		    "cniVersion": "1.0.0",
		    "type": "weave-net"
		}`
		expectedConf2 := `{
		    "name": "",
		    "cniVersion": "1.0.0",
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
		    "readinessindicatorfile": "/tmp/foo.multus.conf",
		    "defaultnetworkwaitseconds": 3,
		    "delegates": [%s,%s]
		}`, expectedConf1, expectedConf2)),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		// This plugin invocation should fail
		err := fmt.Errorf("missing network name")
		fExec.addPlugin100(nil, "net1", expectedConf2, nil, err)

		_, err = CmdAdd(args, fExec, nil)
		Expect(fExec.addIndex).To(Equal(1))
		Expect(fExec.delIndex).To(Equal(1))
		Expect(err).To(HaveOccurred())
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
		"cniVersion": "1.0.0"
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
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: resultCNIVersion,
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
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
		"cniVersion": "1.0.0"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin100(nil, "net1", expectedNet1, &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		r := result.(*cni100.Result)
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(r, expectedResult1)).To(BeTrue())

	})

	It("executes delegates and kubernetes networks", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1,net2", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "1.0.0"
	}`
		net2 := `{
		"name": "net2",
		"type": "mynet2",
		"cniVersion": "1.0.0"
	}`
		net3 := `{
		"name": "net3",
		"type": "mynet3",
		"cniVersion": "1.0.0"
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
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin100(nil, "net1", net1, &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)
		fExec.addPlugin100(nil, "net2", net2, &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.4/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
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
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())
	})

	It("executes kubernetes networks and delete it after pod removal", func() {
		fakePod := testhelpers.NewFakePod("testpod", "net1", "")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "1.0.0"
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
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    }]
	}`),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin100(nil, "net1", net1, &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
			},
		}, nil)

		clientInfo := NewFakeClientInfo()
		_, err := clientInfo.AddPod(fakePod)
		Expect(err).NotTo(HaveOccurred())

		_, err = clientInfo.AddNetAttachDef(
			testhelpers.NewFakeNetAttachDef(fakePod.ObjectMeta.Namespace, "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		result, err := CmdAdd(args, fExec, clientInfo)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		// set fKubeClient to nil to emulate no pod info
		clientInfo.DeletePod(fakePod.ObjectMeta.Namespace, fakePod.ObjectMeta.Name)
		err = CmdDel(args, fExec, clientInfo)
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
	        "cniVersion": "1.0.0",
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

		fExec := newFakeExec()
		expectedConf1 := `{
	    "capabilities": {"portMappings": true},
		"name": "mynet-confList",
	    "cniVersion": "1.0.0",
	    "type": "firstPlugin",
	    "runtimeConfig": {
		    "portMappings": [
	            {"hostPort": 8080, "containerPort": 80, "protocol": "tcp"}
			]
	    }
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, nil, nil)
		_, err := CmdAdd(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("executes clusterNetwork delegate", func() {
		fakePod := testhelpers.NewFakePod("testpod", "", "kube-system/net1")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "1.0.0"
	}`
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
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

		fExec := newFakeExec()
		fExec.addPlugin100(nil, "eth0", net1, expectedResult1, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(testhelpers.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes clusterNetwork delegate with a shared informer", func() {
		fakePod := testhelpers.NewFakePod("testpod", "", "kube-system/net1")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "1.0.0"
	}`
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
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

		fExec := newFakeExec()
		fExec.addPlugin100(nil, "eth0", net1, expectedResult1, nil)

		fKubeClient := NewFakeClientInfo()
		fKubeClient.AddPod(fakePod)
		_, err := fKubeClient.AddNetAttachDef(testhelpers.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		podInformer := newPodInformer(ctx, fKubeClient.Client)
		netdefInformer := newNetDefInformer(ctx, fKubeClient.NetClient)
		fKubeClient.SetK8sClientInformers(podInformer, netdefInformer)

		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

		err = CmdDel(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.delIndex).To(Equal(len(fExec.plugins)))
	})

	It("executes clusterNetwork delegate with a shared informer if pod is not immediately found", func() {
		fakePod := testhelpers.NewFakePod("testpod", "", "kube-system/net1")
		net1 := `{
		"name": "net1",
		"type": "mynet",
		"cniVersion": "1.0.0"
	}`
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
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

		fExec := newFakeExec()
		fExec.addPlugin100(nil, "eth0", net1, expectedResult1, nil)

		fKubeClient := NewFakeClientInfo()
		_, err := fKubeClient.AddNetAttachDef(testhelpers.NewFakeNetAttachDef("kube-system", "net1", net1))
		Expect(err).NotTo(HaveOccurred())

		podInformer := newPodInformer(ctx, fKubeClient.Client)
		netdefInformer := newNetDefInformer(ctx, fKubeClient.NetClient)
		fKubeClient.SetK8sClientInformers(podInformer, netdefInformer)

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			wg.Done()
			time.Sleep(1 * time.Second)
			fKubeClient.AddPod(fakePod)
		}()
		wg.Wait()

		result, err := CmdAdd(args, fExec, fKubeClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(fExec.addIndex).To(Equal(len(fExec.plugins)))
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

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
		"cniVersion": "1.0.0"
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
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    }]
	}`, tmpCNIDir)),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin100(nil, "net1", net1, &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
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
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

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
		"cniVersion": "1.0.0"
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
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    }]
	}`, tmpCNIDir)),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)
		fExec.addPlugin100(nil, "net1", net1, &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.3/24"),
			},
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
		// plugin 1 is the masterplugin
		Expect(reflect.DeepEqual(result, expectedResult1)).To(BeTrue())

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
	    "readinessindicatorfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.0.0",
	        "type": "weave-net"
	    },{
	        "name": "other1",
	        "cniVersion": "1.0.0",
	        "type": "other-plugin"
	    }]
	}`),
		}

		fExec := newFakeExec()
		expectedResult1 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.2/24"),
			},
			},
		}
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.0.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "eth0", expectedConf1, expectedResult1, nil)

		expectedResult2 := &cni100.Result{
			CNIVersion: "1.0.0",
			IPs: []*cni100.IPConfig{{
				Address: *testhelpers.EnsureCIDR("1.1.1.5/24"),
			},
			},
		}
		expectedConf2 := `{
	    "name": "other1",
	    "cniVersion": "1.0.0",
	    "type": "other-plugin"
	}`
		fExec.addPlugin100(nil, "net1", expectedConf2, expectedResult2, nil)

		fakeMultusNetConf := types.NetConf{
			BinDir: "/opt/cni/bin",
		}
		// use fExec for the exec param
		rawnetconflist := []byte(`{"cniVersion":"1.0.0","name":"weave1","type":"weave-net"}`)
		k8sargs, err := k8sclient.GetK8sArgs(args)
		n, err := types.LoadNetConf(args.StdinData)
		rt, _ := types.CreateCNIRuntimeConf(args, k8sargs, args.IfName, n.RuntimeConfig, nil)

		err = conflistDel(rt, rawnetconflist, &fakeMultusNetConf, fExec)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("multus operations cniVersion 1.1.0 config", func() {
	var testNS ns.NetNS
	var tmpDir string
	configPath := "/tmp/foo.multus.conf"
	var cancel context.CancelFunc

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
		_, cancel = context.WithCancel(context.TODO())
	})

	AfterEach(func() {
		cancel()

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
	        "cniVersion": "1.1.0",
		"plugins": [{
	            "type": "weave-net"
	        }]
	    },{
	        "name": "other1",
	        "cniVersion": "1.1.0",
		"plugins": [{
	            "type": "other-plugin"
		}]
	    }]
	}`),
		}

		logging.SetLogLevel("verbose")

		fExec := newFakeExec()
		expectedConf1 := `{
	    "name": "weave1",
	    "cniVersion": "1.1.0",
	    "type": "weave-net"
	}`
		fExec.addPlugin100(nil, "", expectedConf1, nil, nil)

		err := CmdStatus(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		// we only execute once for cluster network, not additional one
		Expect(fExec.statusIndex).To(Equal(1))
	})

	It("executes delegates with CNI GC", func() {
		tmpCNIDir := tmpDir + "/cniData"
		err := os.Mkdir(tmpCNIDir, 0777)
		Expect(err).NotTo(HaveOccurred())

		cniCacheDir := filepath.Join(tmpCNIDir, "/results")
		err = os.Mkdir(cniCacheDir, 0777)
		Expect(err).NotTo(HaveOccurred())

		//create fake cniResult file
		err = os.WriteFile(filepath.Join(cniCacheDir, "cbr0-3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755-eth0"), []byte(`{"kind":"cniCacheV1","containerId":"3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755","config":"eyJjbmlWZXJzaW9uIjoiMC4zLjEiLCJuYW1lIjoiY2JyMCIsInBsdWdpbnMiOlt7ImNhcGFiaWxpdGllcyI6eyJpby5rdWJlcm5ldGVzLmNyaS5wb2QtYW5ub3RhdGlvbnMiOnRydWV9LCJkZWxlZ2F0ZSI6eyJoYWlycGluTW9kZSI6dHJ1ZSwiaXNEZWZhdWx0R2F0ZXdheSI6dHJ1ZX0sInR5cGUiOiJmbGFubmVsIn0seyJjYXBhYmlsaXRpZXMiOnsicG9ydE1hcHBpbmdzIjp0cnVlfSwidHlwZSI6InBvcnRtYXAifV19","ifName":"eth0","networkName":"cbr0","netns":"/var/run/netns/8b8677c8-8929-4746-8206-514069760f6e","cniArgs":[["IgnoreUnknown","true"],["K8S_POD_NAMESPACE","default"],["K8S_POD_NAME","macvlan"],["K8S_POD_INFRA_CONTAINER_ID","3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755"],["K8S_POD_UID","f0bfbd5b-096d-48ef-998c-da26743dd0cb"],["IgnoreUnknown","1"],["K8S_POD_NAMESPACE","default"],["K8S_POD_NAME","macvlan"],["K8S_POD_INFRA_CONTAINER_ID","3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755"],["K8S_POD_UID","f0bfbd5b-096d-48ef-998c-da26743dd0cb"]],"result":{"cniVersion":"0.3.1","dns":{},"interfaces":[{"mac":"ea:19:25:a2:a1:93","name":"cni0"},{"mac":"ba:76:61:2f:8b:ca","name":"vethc42d3d18"},{"mac":"7e:57:6a:9b:6b:b5","name":"eth0","sandbox":"/var/run/netns/8b8677c8-8929-4746-8206-514069760f6e"}],"ips":[{"address":"10.244.1.4/24","gateway":"10.244.1.1","interface":2,"version":"4"}],"routes":[{"dst":"10.244.0.0/16"},{"dst":"0.0.0.0/0","gw":"10.244.1.1"}]}}`), 0666)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile(filepath.Join(cniCacheDir, "macvlan-conf-1-3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755-net1"), []byte(`{"kind":"cniCacheV1","containerId":"3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755","config":"eyJjbmlWZXJzaW9uIjoiMC4zLjEiLCJpcGFtIjp7ImFkZHJlc3NlcyI6W3siYWRkcmVzcyI6IjEwLjEuMS4xMDEvMjQifV0sInR5cGUiOiJzdGF0aWMifSwibWFzdGVyIjoiZXRoMSIsIm1vZGUiOiJicmlkZ2UiLCJuYW1lIjoibWFjdmxhbi1jb25mLTEiLCJ0eXBlIjoibWFjdmxhbiJ9","ifName":"net1","networkName":"macvlan-conf-1","netns":"/var/run/netns/8b8677c8-8929-4746-8206-514069760f6e","cniArgs":[["IgnoreUnknown","true"],["K8S_POD_NAMESPACE","default"],["K8S_POD_NAME","macvlan"],["K8S_POD_INFRA_CONTAINER_ID","3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755"],["K8S_POD_UID","f0bfbd5b-096d-48ef-998c-da26743dd0cb"],["IgnoreUnknown","1"],["K8S_POD_NAMESPACE","default"],["K8S_POD_NAME","macvlan"],["K8S_POD_INFRA_CONTAINER_ID","3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755"],["K8S_POD_UID","f0bfbd5b-096d-48ef-998c-da26743dd0cb"]],"result":{"cniVersion":"0.3.1","dns":{},"interfaces":[{"mac":"36:b3:c5:29:ad:b8","name":"net1","sandbox":"/var/run/netns/8b8677c8-8929-4746-8206-514069760f6e"}],"ips":[{"address":"10.1.1.101/24","interface":0,"version":"4"}]}}`), 0666)
		Expect(err).NotTo(HaveOccurred())

		args := &skel.CmdArgs{
			ContainerID: "123456789",
			Netns:       testNS.Path(),
			IfName:      "eth0",
			StdinData: []byte(fmt.Sprintf(`{
	    "name": "node-cni-network",
	    "type": "multus",
	    "defaultnetworkfile": "/tmp/foo.multus.conf",
	    "defaultnetworkwaitseconds": 3,
	    "cniDir": "%s",
	    "delegates": [{
	        "name": "weave1",
	        "cniVersion": "1.1.0",
		"plugins": [{
	            "type": "weave-net"
	        }]
	    },{
	        "name": "other1",
	        "cniVersion": "1.1.0",
		"plugins": [{
	            "type": "other-plugin"
		}]
	    }]
	}`, tmpCNIDir)),
		}

		logging.SetLogLevel("verbose")

		fExec := newFakeExec()
		expectedConf1 := `{
			"cni.dev/valid-attachments": [
			{
				"containerID": "3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755",
				"ifname": "eth0"
			},
			{
				"containerID": "3f6940ab5ab43bc522569d15b23f8c1bbde1d7678b080398506924fc01d72755",
				"ifname": "net1"
			}
			],
			"name": "weave1",
			"cniVersion": "1.1.0",
			"type": "weave-net"
		}`
		fExec.addPlugin100(nil, "", expectedConf1, nil, nil)

		err = CmdGC(args, fExec, nil)
		Expect(err).NotTo(HaveOccurred())
		// we only execute once for cluster network, not additional one
		Expect(fExec.gcIndex).To(Equal(1))
		err = os.RemoveAll(tmpCNIDir)
		Expect(err).NotTo(HaveOccurred())
	})
})
