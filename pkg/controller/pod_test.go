package controller

import (
	"context"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"path"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	v1coreinformerfactory "k8s.io/client-go/informers"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	nad "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	fakenadclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/fake"
	nadinformers "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/informers/externalversions"
	fakecniclient "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/cniclient/fake"
	fakecontainerruntime "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/containerruntimes/fake"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic network attachment controller suite")
}

var _ = Describe("Dynamic Attachment controller", func() {
	Context("with access to a proper multus configuration", func() {
		var cniConfigDir string

		BeforeEach(func() {
			const (
				configFilePermissions = 0755
				multusConfigPath      = "00-multus.conf"
			)

			var err error
			cniConfigDir, err = ioutil.TempDir("", "multus-config")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.MkdirAll(cniConfigDir, configFilePermissions)).To(Succeed())
			Expect(ioutil.WriteFile(
				path.Join(cniConfigDir, multusConfigPath),
				[]byte(dummyMultusConfig()), configFilePermissions)).To(Succeed())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(cniConfigDir)).To(Succeed())
		})

		Context("with an existing running pod", func() {
			const (
				cniVersion  = "0.3.0"
				namespace   = "default"
				networkName = "tiny-net"
				podName     = "tiny-winy-pod"
			)
			var (
				k8sClient     k8sclient.Interface
				pod           *v1.Pod
				secondNetName string
				stopChannel   chan struct{}
			)

			networkStatusNames := func(statuses []nad.NetworkStatus) []string {
				var names []string
				for _, status := range statuses {
					names = append(names, status.Name)
				}
				return names
			}

			BeforeEach(func() {
				pod = podSpec(podName, namespace, networkName)
				k8sClient = fake.NewSimpleClientset(pod)
				secondNetName = fmt.Sprintf("%s-2", networkName)
				nadClient, err := newFakeNetAttachDefClient(
					netAttachDef(networkName, namespace, dummyNetSpec(networkName, cniVersion)),
					netAttachDef(secondNetName, namespace, dummyNetSpec(secondNetName, cniVersion)))
				Expect(err).NotTo(HaveOccurred())
				stopChannel = make(chan struct{})
				eventRecorder := record.NewFakeRecorder(5)
				Expect(newDummyPodController(k8sClient, nadClient, stopChannel, eventRecorder, "", cniConfigDir)).NotTo(BeNil())
				Expect(func() []nad.NetworkStatus {
					pod, err := k8sClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
					if err != nil {
						return nil
					}
					status, err := networkStatus(pod.Annotations)
					if err != nil {
						return nil
					}
					return status
				}()).Should(
					And(
						WithTransform(networkStatusNames, ContainElements(namespacedName(namespace, networkName))),
						Not(WithTransform(networkStatusNames, ContainElements(namespacedName(namespace, secondNetName))))),
				)
			})

			AfterEach(func() {
				close(stopChannel)
			})

			When("an attachment is added to the pod's network annotations", func() {
				BeforeEach(func() {
					var err error
					_, err = k8sClient.CoreV1().Pods(namespace).UpdateStatus(
						context.TODO(),
						updatePodSpec(pod, networkName, secondNetName),
						metav1.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("the pod's network-status is updated with the new network", func() {
					Eventually(func() ([]nad.NetworkStatus, error) {
						newPod, err := k8sClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
						if err != nil {
							return nil, err
						}
						return networkStatus(newPod.Annotations)
					}).Should(
						WithTransform(
							networkStatusNames,
							ContainElements(namespacedName(namespace, secondNetName))))
				})
			})

			When("an attachment is removed from the pod's network annotations", func() {
				BeforeEach(func() {
					var err error
					_, err = k8sClient.CoreV1().Pods(namespace).UpdateStatus(
						context.TODO(),
						updatePodSpec(pod),
						metav1.UpdateOptions{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("that attachment is absent from the pod's network-status", func() {
					Eventually(func() ([]nad.NetworkStatus, error) {
						newPod, err := k8sClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
						if err != nil {
							return nil, err
						}
						return networkStatus(newPod.Annotations)
					}).ShouldNot(
						WithTransform(
							networkStatusNames,
							ContainElements(namespacedName(namespace, networkName))))
				})
			})
		})
	})
})

type dummyPodController struct {
	*PodNetworksController
	ipPoolCache  cache.Store
	networkCache cache.Store
	podCache     cache.Store
}

func newDummyPodController(
	k8sClient k8sclient.Interface,
	nadClient nadclient.Interface,
	stopChannel chan struct{},
	recorder record.EventRecorder,
	hostCNIDir string,
	cniConfigPath string) (*dummyPodController, error) {

	const noResyncPeriod = 0
	netAttachDefInformerFactory := nadinformers.NewSharedInformerFactory(nadClient, noResyncPeriod)
	podInformerFactory := v1coreinformerfactory.NewSharedInformerFactory(k8sClient, noResyncPeriod)

	fakeRuntime, err := generateContainerRuntimeForCurrentClient(k8sClient)
	if err != nil {
		return nil, err
	}

	podController, _ := NewPodNetworksController(
		podInformerFactory,
		netAttachDefInformerFactory,
		nil,
		recorder,
		fakecniclient.NewFakeCNI(hostCNIDir),
		cniConfigPath,
		k8sClient,
		nadClient,
		fakeRuntime)

	alwaysReady := func() bool { return true }
	podController.arePodsSynched = alwaysReady
	podController.areNetAttachDefsSynched = alwaysReady

	podInformerFactory.Start(stopChannel)
	netAttachDefInformerFactory.Start(stopChannel)

	controller := &dummyPodController{
		PodNetworksController: podController,
		networkCache:          podController.netAttachDefInformer.GetStore(),
		podCache:              podController.podsInformer.GetStore(),
	}

	if err := controller.initControllerCaches(k8sClient, nadClient); err != nil {
		return nil, err
	}
	go podController.Start(stopChannel)

	return controller, nil
}

func newFakeNetAttachDefClient(networkAttachments ...nad.NetworkAttachmentDefinition) (nadclient.Interface, error) {
	netAttachDefClient := fakenadclient.NewSimpleClientset()
	gvr := metav1.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}

	for _, networkAttachment := range networkAttachments {
		if err := netAttachDefClient.Tracker().Create(schema.GroupVersionResource(gvr), &networkAttachment, networkAttachment.GetNamespace()); err != nil {
			return nil, err
		}
	}
	return netAttachDefClient, nil
}

func (dpc *dummyPodController) initControllerCaches(k8sClient k8sclient.Interface, nadClient nadclient.Interface) error {
	if err := dpc.synchPods(k8sClient); err != nil {
		return err
	}
	if err := dpc.synchNetworkAttachments(nadClient); err != nil {
		return err
	}
	return nil
}

func (dpc *dummyPodController) synchNetworkAttachments(netAttachDefClient nadclient.Interface) error {
	const allNamespaces = ""

	networkAttachments, err := netAttachDefClient.K8sCniCncfIoV1().NetworkAttachmentDefinitions(allNamespaces).List(
		context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, network := range networkAttachments.Items {
		if err := dpc.networkCache.Add(&network); err != nil {
			return err
		}
	}
	return nil
}

func (dpc *dummyPodController) synchPods(k8sClient k8sclient.Interface) error {
	const allNamespaces = ""

	pods, err := k8sClient.CoreV1().Pods(allNamespaces).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if err := dpc.podCache.Add(&pod); err != nil {
			return err
		}
	}
	return nil
}

func dummyNetSpec(networkName string, cniVersion string) string {
	return fmt.Sprintf(`{
      "cniVersion": "%s",
      "name": "%s",
      "type": "macvlan",
      "master": "eth0",
      "mode": "bridge"
    }`, cniVersion, networkName)
}

func podSpec(name string, namespace string, networks ...string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: podNetworkConfig(networks...),
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: name,
				},
			},
		},
	}
}

func netAttachDef(netName string, namespace string, config string) nad.NetworkAttachmentDefinition {
	return nad.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      netName,
			Namespace: namespace,
		},
		Spec: nad.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}
}

func updatePodSpec(pod *v1.Pod, networkNames ...string) *v1.Pod {
	newPod := pod.DeepCopy()
	newPod.Annotations[nad.NetworkAttachmentAnnot] = generateNetworkSelectionAnnotation(
		"default", networkNames...)
	return newPod
}

// this should be used when "creating" a new pod - it sets the status.
func podNetworkConfig(networkNames ...string) map[string]string {
	return map[string]string{
		nad.NetworkAttachmentAnnot: generateNetworkSelectionAnnotation("default", networkNames...),
		nad.NetworkStatusAnnot:     podNetworkStatusAnnotations("default", networkNames...),
	}
}

func generateNetworkSelectionAnnotation(namespace string, networkNames ...string) string {
	var netSelectionElements []nad.NetworkSelectionElement
	for i, networkName := range networkNames {
		netSelectionElements = append(
			netSelectionElements,
			nad.NetworkSelectionElement{
				Name:             networkName,
				Namespace:        namespace,
				InterfaceRequest: fmt.Sprintf("net%d", i),
			})
	}
	serelizedNetSelectionElements, err := json.Marshal(netSelectionElements)
	if err != nil {
		return ""
	}
	return string(serelizedNetSelectionElements)
}

func podNetworkStatusAnnotations(namespace string, networkNames ...string) string {
	var netStatus []nad.NetworkStatus
	for i, networkName := range networkNames {
		netStatus = append(
			netStatus,
			nad.NetworkStatus{
				Name:      fmt.Sprintf("%s/%s", namespace, networkName),
				Interface: fmt.Sprintf("net%d", i),
			})
	}
	serelizedNetStatus, err := json.Marshal(netStatus)
	if err != nil {
		return ""
	}
	return string(serelizedNetStatus)
}

func generateContainerRuntimeForCurrentClient(client k8sclient.Interface) (*fakecontainerruntime.Runtime, error) {
	const allNamespaces = ""
	allPods, err := client.CoreV1().Pods(allNamespaces).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return fakecontainerruntime.NewFakeRuntime(allPods.Items...), nil
}

func dummyMultusConfig() string {
	return `{
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
}
