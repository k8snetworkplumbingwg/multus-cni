// Copyright (c) 2021 Multus Authors
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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

var _ = Describe("Multus basic operations", func() {
	const (
		namespace           = "default"
		numberOfWorkerNodes = 2
	)

	Context("Simple macvlan delegate", func() {
		const lowerDevice = "eth1"

		const (
			firstPodIP    = "10.1.1.11/24"
			firstPodName  = "macvlan1-worker1"
			secondPodIP   = "10.1.1.12/24"
			secondPodName = "macvlan1-worker2"
			networkName   = "macvlan1-config"
		)

		var (
			firstNode  *v1.Node
			secondNode *v1.Node

			firstPod  *v1.Pod
			secondPod *v1.Pod

			nad *nadv1.NetworkAttachmentDefinition
		)

		BeforeEach(func() {
			var err error
			nad, err = clientset.nadClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(namespace).Create(
				context.TODO(),
				newMacvlanNetworkAttachmentDefinitionSpec(
					namespace, networkName, lowerDevice),
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "should have been able to create the NET-ATTACH-DEF")

			nodes, _ := clientset.k8sClientSet.CoreV1().Nodes().List(
				context.TODO(),
				metav1.ListOptions{
					LabelSelector: "!node-role.kubernetes.io/control-plane",
				})
			Expect(nodes.Items).NotTo(BeEmpty())
			Expect(len(nodes.Items)).To(Equal(numberOfWorkerNodes))

			firstNode = &nodes.Items[0]
			secondNode = &nodes.Items[1]
		})

		BeforeEach(func() {
			firstNodeName := firstNode.GetName()
			firstPod = newPod(
				namespace,
				firstPodName,
				&firstNodeName,
				nadv1.NetworkSelectionElement{
					Name:             networkName,
					Namespace:        namespace,
					IPRequest:        []string{firstPodIP},
					InterfaceRequest: "net1",
				})

			secondNodeName := secondNode.GetName()
			secondPod = newPod(
				namespace,
				secondPodName,
				&secondNodeName,
				nadv1.NetworkSelectionElement{
					Name:      networkName,
					Namespace: namespace,
					IPRequest: []string{secondPodIP},
				})

			var err error
			for _, pod := range []*v1.Pod{firstPod, secondPod} {
				_, err = clientset.k8sClientSet.CoreV1().Pods(namespace).Create(
					context.TODO(), pod, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred(), "should have been able to create a pod")
			}
			for _, pod := range []*v1.Pod{firstPod, secondPod} {
				Eventually(func() bool {
					pod, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(
						context.TODO(), pod.GetName(), metav1.GetOptions{})
					Expect(err).NotTo(HaveOccurred(), "should have been able to retrieve pod")
					return pod.Status.Phase == v1.PodRunning
				}, 30*time.Second, time.Second).Should(BeTrue())
			}
		})

		AfterEach(func() {
			for _, pod := range []*v1.Pod{firstPod, secondPod} {
				Expect(clientset.k8sClientSet.CoreV1().Pods(namespace).Delete(
					context.TODO(), pod.GetName(), metav1.DeleteOptions{})).To(Succeed(), "should have been able to delete the pod")
			}
			for _, pod := range []*v1.Pod{firstPod, secondPod} {
				Eventually(func() bool {
					_, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(context.TODO(), pod.GetName(), metav1.GetOptions{})
					return errors.IsNotFound(err)
				}, 60*time.Second, 3*time.Second).Should(BeTrue())
			}
		})

		AfterEach(func() {
			Expect(clientset.nadClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(namespace).Delete(
				context.TODO(), nad.GetName(), metav1.DeleteOptions{})).NotTo(HaveOccurred(), "should have been able to delete the NET-ATTACH-DEF")
		})

		It("features the expected IP on its network status annotation", func() {
			firstPod, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(context.TODO(), firstPod.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "should have been able to retrieve pod")

			podNetStatus, found := firstPod.Annotations[nadv1.NetworkStatusAnnot]
			Expect(found).To(BeTrue(), "expected the pod to have a `networks-status` annotation")
			var netStatus []nadv1.NetworkStatus
			Expect(json.Unmarshal([]byte(podNetStatus), &netStatus)).To(Succeed())
			Expect(netStatus).NotTo(BeEmpty())

			nonDefaultNetworkStatus := filterNetworkStatus(netStatus, func(status nadv1.NetworkStatus) bool {
				return !status.Default
			})
			Expect(nonDefaultNetworkStatus).NotTo(BeNil())
			Expect(nonDefaultNetworkStatus.IPs).To(ConsistOf(ipFromCIDR(firstPodIP)))
		})

		Context("dynamic attachments", func() {
			const (
				newNetworkName      = "newnet2000"
				updateStatusTimeout = 15 * time.Second
			)

			var (
				newNetwork *nadv1.NetworkAttachmentDefinition
			)

			BeforeEach(func() {
				var err error
				newNetwork, err = clientset.nadClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(namespace).Create(
					context.TODO(),
					newMacvlanNetworkAttachmentDefinitionSpec(
						namespace, newNetworkName, lowerDevice),
					metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred(), "should have been able to create the NET-ATTACH-DEF")
			})

			AfterEach(func() {
				Expect(clientset.nadClientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions(namespace).Delete(
					context.TODO(), newNetwork.GetName(), metav1.DeleteOptions{})).NotTo(HaveOccurred(), "should have been able to delete the NET-ATTACH-DEF")
			})

			It("add a network to the pod", func() {
				firstPod, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(context.TODO(), firstPod.GetName(), metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "should have been able to retrieve pod")

				const (
					newIfaceName = "pluggediface1"
					newNetworkIP = "10.10.10.2/24"
				)
				Expect(addNetwork(firstPod, nadv1.NetworkSelectionElement{
					Name:             newNetworkName,
					Namespace:        namespace,
					IPRequest:        []string{newNetworkIP},
					InterfaceRequest: newIfaceName,
				})).To(Succeed())
				Expect(clientset.k8sClientSet.CoreV1().Pods(namespace).Update(context.TODO(), firstPod, metav1.UpdateOptions{})).NotTo(BeNil())
				Eventually(func() ([]nadv1.NetworkStatus, error) {
					pod, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(context.TODO(), firstPod.GetName(), metav1.GetOptions{})
					if err != nil {
						return nil, err
					}
					statuses, err := podNetworkStatus(pod)
					if err != nil {
						return nil, err
					}
					return statuses, nil
				}, updateStatusTimeout).Should(
					ContainElements(
						NetworkStatus(namespace, newNetworkName, newIfaceName, ipFromCIDR(newNetworkIP))))
			})

			It("remove a network from the pod", func() {
				firstPod, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(context.TODO(), firstPod.GetName(), metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "should have been able to retrieve pod")

				Expect(removeNetwork(firstPod, nadv1.NetworkSelectionElement{
					Name:             networkName,
					Namespace:        namespace,
					InterfaceRequest: "net1",
				})).To(Succeed())
				Expect(clientset.k8sClientSet.CoreV1().Pods(namespace).Update(context.TODO(), firstPod, metav1.UpdateOptions{})).NotTo(BeNil())

				Eventually(func() ([]nadv1.NetworkStatus, error) {
					pod, err := clientset.k8sClientSet.CoreV1().Pods(namespace).Get(context.TODO(), firstPod.GetName(), metav1.GetOptions{})
					if err != nil {
						return nil, err
					}
					statuses, err := podNetworkStatus(pod)
					if err != nil {
						return nil, err
					}
					return statuses, nil
				}, updateStatusTimeout).ShouldNot(
					ContainElements(
						NetworkStatus(namespace, newNetworkName, "net1", ipFromCIDR(firstPodIP))))
			})
		})
	})
})

func namespacedNetworkName(namespace, networkName string) string {
	return fmt.Sprintf("%s/%s", namespace, networkName)
}

func podNetworkStatus(pod *v1.Pod) ([]nadv1.NetworkStatus, error) {
	var podNetworks []nadv1.NetworkStatus

	networkList, wasFound := pod.Annotations[nadv1.NetworkStatusAnnot]
	if !wasFound {
		return podNetworks, fmt.Errorf("the pod is missing the status annotation")
	}
	if err := json.Unmarshal([]byte(networkList), &podNetworks); err != nil {
		return nil, err
	}
	return podNetworks, nil
}

func newMacvlanNetworkAttachmentDefinitionSpec(namespace string, networkName string, lowerDevice string) *nadv1.NetworkAttachmentDefinition {
	config := fmt.Sprintf(`{
            "cniVersion": "0.3.1",
            "plugins": [
                {
                    "type": "macvlan",
                    "capabilities": { "ips": true },
                    "master": "%s",
                    "mode": "bridge",
                    "ipam": {
                        "type": "static"
                    }
                }, {
                    "type": "tuning"
                } ]
        }`, lowerDevice)
	return &nadv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkName,
			Namespace: namespace,
		},
		Spec: nadv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}
}

func newPod(namespace string, podName string, nodeName *string, attachments ...nadv1.NetworkSelectionElement) *v1.Pod {
	const (
		containerImgName = "centos:8"
		hostSelectorKey  = "kubernetes.io/hostname"
	)
	privileged := true

	networkAnnotationsPayload, err := json.Marshal(attachments)
	if err != nil {
		return nil
	}
	podAnnotations := map[string]string{
		nadv1.NetworkAttachmentAnnot: string(networkAnnotationsPayload),
	}

	nodeSelector := map[string]string{}
	if nodeName != nil {
		nodeSelector[hostSelectorKey] = *nodeName
	}

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   namespace,
			Annotations: podAnnotations,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    podName,
					Image:   containerImgName,
					Command: []string{"/bin/sleep", "10000"},
					SecurityContext: &v1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
			NodeSelector: nodeSelector,
		},
	}
}

func filterNetworkStatus(networkStatuses []nadv1.NetworkStatus, predicate func(nadv1.NetworkStatus) bool) *nadv1.NetworkStatus {
	for i, networkStatus := range networkStatuses {
		if predicate(networkStatus) {
			return &networkStatuses[i]
		}
	}
	return nil
}

func ipFromCIDR(cidr string) string {
	return strings.Split(cidr, "/")[0]
}

func addNetwork(pod *v1.Pod, networks ...nadv1.NetworkSelectionElement) error {
	networkList, wasFound := pod.Annotations[nadv1.NetworkAttachmentAnnot]
	if !wasFound {
		return nil
	}

	var podNetworks []nadv1.NetworkSelectionElement
	if err := json.Unmarshal([]byte(networkList), &podNetworks); err != nil {
		return err
	}

	for _, net := range networks {
		podNetworks = append(podNetworks, net)
	}
	updatedNetworksAnnotation, err := json.Marshal(podNetworks)
	if err != nil {
		return err
	}

	pod.Annotations[nadv1.NetworkAttachmentAnnot] = string(updatedNetworksAnnotation)
	return nil
}

func removeNetwork(pod *v1.Pod, networks ...nadv1.NetworkSelectionElement) error {
	networkList, wasFound := pod.Annotations[nadv1.NetworkAttachmentAnnot]
	if !wasFound {
		return nil
	}

	var currentPodNetworks, updatedPodNetworks []nadv1.NetworkSelectionElement
	if err := json.Unmarshal([]byte(networkList), &currentPodNetworks); err != nil {
		return err
	}

	updatedPodNetworks = []nadv1.NetworkSelectionElement{}
	for _, net := range networks {
		for _, existingNet := range currentPodNetworks {
			if net.Name == existingNet.Name && net.Namespace == existingNet.Namespace {
				continue
			}
			updatedPodNetworks = append(updatedPodNetworks, existingNet)
		}
	}
	updatedNetworksAnnotation, err := json.Marshal(updatedPodNetworks)
	if err != nil {
		return err
	}

	pod.Annotations[nadv1.NetworkAttachmentAnnot] = string(updatedNetworksAnnotation)
	return nil
}

// NetworkStatus uses reflect.DeepEqual to compare actual with expected.  Equal is strict about
//types when performing comparisons.
//It is an error for both actual and expected to be nil.  Use BeNil() instead.
func NetworkStatus(namespace string, networkName string, ifaceName string, ip string) types.GomegaMatcher {
	return &NetworkStatusMatcher{
		Expected: &nadv1.NetworkStatus{
			Name:      namespacedNetworkName(namespace, networkName),
			Interface: ifaceName,
			IPs:       []string{ip},
			Default:   false,
		},
	}
}

type NetworkStatusMatcher struct {
	Expected *nadv1.NetworkStatus
}

func (matcher *NetworkStatusMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil && matcher.Expected == nil {
		return false, fmt.Errorf("refusing to compare <nil> to <nil>.\nBe explicit and use BeNil() instead.  This is to avoid mistakes where both sides of an assertion are erroneously uninitialized")
	}

	actualNetStatus, didTypeCastSucceed := actual.(nadv1.NetworkStatus)
	if !didTypeCastSucceed {
		return false, fmt.Errorf("incorrect type passed")
	}
	return actualNetStatus.Name == matcher.Expected.Name &&
		reflect.DeepEqual(actualNetStatus.IPs, matcher.Expected.IPs) &&
		actualNetStatus.Interface == matcher.Expected.Interface, nil
}

func (matcher *NetworkStatusMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "Name, IPs, and InterfaceName to equal", matcher.Expected)
}

func (matcher *NetworkStatusMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "Name, IPs, and InterfaceName *not* to equal", matcher.Expected)
}
