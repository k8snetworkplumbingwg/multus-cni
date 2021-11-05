package k8sclient

import (
	"fmt"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/containernetworking/cni/pkg/skel"
	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	testutils "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/testing"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

func TestNetconfGenerator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "k8sclient")
}

var _ = Describe("Test NetconfGeneration operations", func() {
	var clientInfo *ClientInfo
	var tmpDir string
	var args *skel.CmdArgs
	var k8sArgs *types.K8sArgs
	var pod *v1.Pod
	var podNetworks []*types.NetworkSelectionElement

	const fakePodName = "testPod"
	const newNetSpec = `
{
    "cniVersion": "0.3.1",
    "plugins": [
        {
            "type": "macvlan",
            "capabilities": { "ips": true },
            "master": "eth1",
            "mode": "bridge",
            "ipam": { "type": "static"}
        }, {
            "type": "tuning"
        }
    ]
}`

	BeforeEach(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "multus_tmp")
		Expect(err).NotTo(HaveOccurred())

		args = &skel.CmdArgs{
			// Values come from NewFakePod()
			Args: fmt.Sprintf("K8S_POD_NAME=%s;K8S_POD_NAMESPACE=%s;K8S_POD_UID=%s", fakePodName, "test", "testUID"),
		}
	})

	BeforeEach(func() {
		clientInfo = NewFakeClientInfo()
		_, err := clientInfo.AddNetAttachDef(testutils.NewFakeNetAttachDef("test", "newnet", newNetSpec))
		Expect(err).NotTo(HaveOccurred())

		k8sArgs, err = GetK8sArgs(args)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Context("focusing on the CNCF *network* annotation list - i.e. for hot-plugging new interfaces", func() {
		When("a pod's annotation's list is empty", func() {
			BeforeEach(func() {
				fakePod := testutils.NewFakePod(fakePodName, "", "")
				_, err := clientInfo.AddPod(fakePod)
				Expect(err).NotTo(HaveOccurred())

				pod, err = clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
				Expect(err).NotTo(HaveOccurred())
				podNetworks, err = GetPodNetwork(pod)
				Expect(err).To(HaveOccurred())
			})

			It("the netconfig generator cannot create a delegate", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface1"))
				Expect(err).To(MatchError(&NoK8sNetworkError{message: "could not find pod network matching the provided predicate"}))
				Expect(networks).To(BeNil())
			})
		})

		When("a pod's annotation's list features an element", func() {
			BeforeEach(func() {
				podsNetworks := `[{ "name": "newnet", "ips": [ "10.10.10.10/24" ], "interface": "pluggediface1", "namespace": "default" }]`
				fakePod := testutils.NewFakePod(fakePodName, podsNetworks, "")
				_, err := clientInfo.AddPod(fakePod)
				Expect(err).NotTo(HaveOccurred())

				pod, err = clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
				Expect(err).NotTo(HaveOccurred())
				podNetworks, err = GetPodNetwork(pod)
				Expect(err).NotTo(HaveOccurred())
			})

			It("the netconfig generator finds a NetworkSelectionElement when the correct network + iface name are requested", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(networks).To(Equal(&types.NetworkSelectionElement{
					Name:             "newnet",
					Namespace:        "default",
					IPRequest:        []string{"10.10.10.10/24"},
					InterfaceRequest: "pluggediface1",
				}))
			})

			It("the netconfig generator does not find a NetworkSelectionElement when the wrong *interface* name is requested", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface2"))
				Expect(err).To(MatchError(&NoK8sNetworkError{message: "could not find pod network matching the provided predicate"}))
				Expect(networks).To(BeNil())
			})

			It("the netconfig generator does not find a NetworkSelectionElement when the wrong *network* name is requested", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet2", "pluggediface1"))
				Expect(err).To(MatchError(&NoK8sNetworkError{message: "could not find pod network matching the provided predicate"}))
				Expect(networks).To(BeNil())
			})
		})

		When("a pod's annotation's list features multiple elements", func() {
			BeforeEach(func() {
				podsNetworks := `[
{ "name": "macvlan1-config", "ips": [ "10.1.1.11/24" ] },
{ "name": "newnet", "ips": [ "10.10.10.10/24" ], "interface": "pluggediface1", "namespace": "default" },
{ "name": "newnet", "ips": [ "10.10.10.11/24" ], "interface": "pluggediface2", "namespace": "default" }]`
				fakePod := testutils.NewFakePod(fakePodName, podsNetworks, "")
				_, err := clientInfo.AddPod(fakePod)
				Expect(err).NotTo(HaveOccurred())

				pod, err = clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
				Expect(err).NotTo(HaveOccurred())
				podNetworks, err = GetPodNetwork(pod)
				Expect(err).NotTo(HaveOccurred())
			})

			It("filters the *first* network + interface pair", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(networks).To(Equal(
					&types.NetworkSelectionElement{
						Name:             "newnet",
						Namespace:        "default",
						IPRequest:        []string{"10.10.10.10/24"},
						InterfaceRequest: "pluggediface1",
					}))
			})

			It("filters the *second* network + interface pair", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface2"))
				Expect(err).NotTo(HaveOccurred())
				Expect(networks).To(Equal(
					&types.NetworkSelectionElement{
						Name:             "newnet",
						Namespace:        "default",
						IPRequest:        []string{"10.10.10.11/24"},
						InterfaceRequest: "pluggediface2",
					}))
			})
		})
	})

	Context("focusing on the CNCF network *status* annotation list - i.e. for hot-unplugging existing interfaces", func() {
		const (
			netName = "newnet"
			nsName  = "ns1"
		)

		When("a pod's annotation's list is empty", func() {
			BeforeEach(func() {
				fakePod := testutils.NewFakePod(fakePodName, "", "")
				_, err := clientInfo.AddPod(fakePod)
				Expect(err).NotTo(HaveOccurred())

				pod, err = clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("a pod's annotation's list features an element", func() {
			BeforeEach(func() {
				fakePod := testutils.NewFakePod(fakePodName, "", "")
				netStatus := netv1.NetworkStatus{
					Name:      networkName(nsName, netName),
					Interface: "pluggediface1",
					IPs:       []string{"10.10.10.10/24"},
				}
				Expect(testutils.WithNetworkStatusAnnotation(netStatus)(fakePod)).To(Succeed())
				_, err := clientInfo.AddPod(fakePod)
				Expect(err).NotTo(HaveOccurred())

				pod, err = clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
				Expect(err).NotTo(HaveOccurred())
				podNetworks, err = GetPodNetworkFromStatus(pod)
				Expect(err).NotTo(HaveOccurred())
			})

			It("the netconfig generator finds a NetworkSelectionElement when the correct network + iface name are requested", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(networks).To(Equal(&types.NetworkSelectionElement{
					Name:             netName,
					Namespace:        nsName,
					IPRequest:        []string{"10.10.10.10/24"},
					InterfaceRequest: "pluggediface1",
				}))
			})

			It("the netconfig generator does not find a NetworkSelectionElement when the wrong *interface* name is requested", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface2"))
				Expect(err).To(MatchError(&NoK8sNetworkError{message: "could not find pod network matching the provided predicate"}))
				Expect(networks).To(BeNil())
			})

			It("the netconfig generator does not find a NetworkSelectionElement when the wrong *network* name is requested", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet2", "pluggediface1"))
				Expect(err).To(MatchError(&NoK8sNetworkError{message: "could not find pod network matching the provided predicate"}))
				Expect(networks).To(BeNil())
			})
		})

		When("a pod's annotation's list features multiple elements", func() {
			BeforeEach(func() {
				iface1NetStatus := netv1.NetworkStatus{
					Name:      networkName(nsName, netName),
					Interface: "pluggediface1",
					IPs:       []string{"10.10.10.10/24"},
				}
				iface2NetStatus := netv1.NetworkStatus{
					Name:      networkName(nsName, netName),
					Interface: "pluggediface2",
					IPs:       []string{"10.10.10.11/24"},
				}
				fakePod := testutils.NewFakePod(fakePodName, "", "")
				Expect(testutils.WithNetworkStatusAnnotation(iface1NetStatus, iface2NetStatus)(fakePod)).To(Succeed())
				_, err := clientInfo.AddPod(fakePod)
				Expect(err).NotTo(HaveOccurred())

				pod, err = clientInfo.GetPod(string(k8sArgs.K8S_POD_NAMESPACE), string(k8sArgs.K8S_POD_NAME))
				Expect(err).NotTo(HaveOccurred())
				podNetworks, err = GetPodNetworkFromStatus(pod)
				Expect(err).NotTo(HaveOccurred())
			})

			It("filters the *first* network + interface pair", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(networks).To(Equal(
					&types.NetworkSelectionElement{
						Name:             netName,
						Namespace:        nsName,
						IPRequest:        []string{"10.10.10.10/24"},
						InterfaceRequest: "pluggediface1",
					}))
			})

			It("filters the *second* network + interface pair", func() {
				networks, err := FilterNetwork(podNetworks, filterNetworkByNetAndIfaceName("newnet", "pluggediface2"))
				Expect(err).NotTo(HaveOccurred())
				Expect(networks).To(Equal(
					&types.NetworkSelectionElement{
						Name:             netName,
						Namespace:        nsName,
						IPRequest:        []string{"10.10.10.11/24"},
						InterfaceRequest: "pluggediface2",
					}))
			})
		})
	})
})

func networkName(namespace string, networkName string) string {
	return fmt.Sprintf("%s/%s", namespace, networkName)
}
