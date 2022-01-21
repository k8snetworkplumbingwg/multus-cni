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
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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
					Name:      networkName,
					Namespace: namespace,
					IPRequest: []string{firstPodIP},
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
	})
})

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
