// Copyright (c) 2018 Intel Corporation
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

package checkpoint

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
)

const (
	fakeTempFile = "/tmp/kubelet_internal_checkpoint"
)

type fakeCheckpoint struct {
	fileName string
}

func (fc *fakeCheckpoint) WriteToFile(inBytes []byte) error {
	return os.WriteFile(fc.fileName, inBytes, 0600)
}

func (fc *fakeCheckpoint) DeleteFile() error {
	return os.Remove(fc.fileName)
}

func TestCheckpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Checkpoint")
}

var _ = BeforeSuite(func() {
	sampleData := `{
		"Data": {
			"PodDeviceEntries": [
			{
				"PodUID": "970a395d-bb3b-11e8-89df-408d5c537d23",
				"ContainerName": "appcntr1",
				"ResourceName": "intel.com/sriov_net_A",
				"DeviceIDs": {"-1": [
					"0000:03:02.3",
					"0000:03:02.0"
					]
				},
				"AllocResp": "CikKC3NyaW92X25ldF9BEhogMDAwMDowMzowMi4zIDAwMDA6MDM6MDIuMA=="
			}
			],
			"RegisteredDevices": {
			"intel.com/sriov_net_A": [
				"0000:03:02.1",
				"0000:03:02.2",
				"0000:03:02.3",
				"0000:03:02.0"
			],
			"intel.com/sriov_net_B": [
				"0000:03:06.3",
				"0000:03:06.0",
				"0000:03:06.1",
				"0000:03:06.2"
			]
			}
		},
		"Checksum": 229855270
		}`

	fakeCheckpoint := &fakeCheckpoint{fileName: fakeTempFile}
	err := fakeCheckpoint.WriteToFile([]byte(sampleData))
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Kubelet checkpoint data read operations", func() {
	Context("Using /tmp/kubelet_internal_checkpoint file", func() {
		var (
			cp            types.ResourceClient
			err           error
			resourceMap   map[string]*types.ResourceInfo
			resourceInfo  *types.ResourceInfo
			resourceAnnot = "intel.com/sriov_net_A"
		)

		It("should get a Checkpoint instance from file", func() {
			cp, err = getCheckpoint(fakeTempFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return a ResourceMap instance", func() {
			podUID := k8sTypes.UID("970a395d-bb3b-11e8-89df-408d5c537d23")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakePod",
					Namespace: "podNamespace",
					UID:       podUID,
				},
			}
			rmap, err := cp.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(rmap).NotTo(BeEmpty())
			resourceMap = rmap
		})

		It("resourceMap should have value for \"intel.com/sriov_net_A\"", func() {
			rInfo, ok := resourceMap[resourceAnnot]
			Expect(ok).To(BeTrue())
			resourceInfo = rInfo
		})

		It("should have 2 deviceIDs", func() {
			Expect(len(resourceInfo.DeviceIDs)).To(BeEquivalentTo(2))
		})

		// Single-container allocation is sorted; container order is preserved across entries.
		It("should have \"0000:03:02.0\" in deviceIDs[0] (per-container sorted)", func() {
			Expect(resourceInfo.DeviceIDs[0]).To(BeEquivalentTo("0000:03:02.0"))
		})

		It("should have \"0000:03:02.3\" in deviceIDs[1] (per-container sorted)", func() {
			Expect(resourceInfo.DeviceIDs[1]).To(BeEquivalentTo("0000:03:02.3"))
		})
	})

	Context("Using a checkpoint file with multiple containers sharing a resource name", func() {
		const multiContainerFile = "/tmp/kubelet_internal_checkpoint_multi_container"

		var (
			fakeCp      *fakeCheckpoint
			resourceMap map[string]*types.ResourceInfo
		)

		BeforeEach(func() {
			// appcntr1 and appcntr2 both get devices from intel.com/sriov_net_A,
			// and a third entry belongs to a different pod.
			sampleData := `{
				"Data": {
					"PodDeviceEntries": [
					{
						"PodUID": "8f5ba1b4-cc11-11e8-89df-408d5c537d23",
						"ContainerName": "appcntr1",
						"ResourceName": "intel.com/sriov_net_A",
						"DeviceIDs": {"-1": [
							"0000:03:02.3",
							"0000:03:02.0"
							]
						},
						"AllocResp": ""
					},
					{
						"PodUID": "8f5ba1b4-cc11-11e8-89df-408d5c537d23",
						"ContainerName": "appcntr2",
						"ResourceName": "intel.com/sriov_net_A",
						"DeviceIDs": {"-1": [
							"0000:03:02.5",
							"0000:03:02.1"
							]
						},
						"AllocResp": ""
					},
					{
						"PodUID": "8f5ba1b4-cc11-11e8-89df-408d5c537d23",
						"ContainerName": "appcntr2",
						"ResourceName": "intel.com/sriov_net_B",
						"DeviceIDs": {
							"1": ["0000:03:06.3"],
							"0": ["0000:03:06.0"]
						},
						"AllocResp": ""
					},
					{
						"PodUID": "970a395d-bb3b-11e8-89df-408d5c537d23",
						"ContainerName": "appcntr1",
						"ResourceName": "intel.com/sriov_net_A",
						"DeviceIDs": {"-1": [
							"0000:03:02.7"
							]
						},
						"AllocResp": ""
					}
					],
					"RegisteredDevices": {
					"intel.com/sriov_net_A": [
						"0000:03:02.0",
						"0000:03:02.1",
						"0000:03:02.3",
						"0000:03:02.5",
						"0000:03:02.7"
					],
					"intel.com/sriov_net_B": [
						"0000:03:06.0",
						"0000:03:06.3"
					]
					}
				},
				"Checksum": 0
				}`

			fakeCp = &fakeCheckpoint{fileName: multiContainerFile}
			Expect(fakeCp.WriteToFile([]byte(sampleData))).To(Succeed())

			cp, err := getCheckpoint(multiContainerFile)
			Expect(err).NotTo(HaveOccurred())

			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakePod",
					Namespace: "podNamespace",
					UID:       k8sTypes.UID("8f5ba1b4-cc11-11e8-89df-408d5c537d23"),
				},
			}
			resourceMap, err = cp.GetPodResourceMap(fakePod)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(fakeCp.DeleteFile()).To(Succeed())
		})

		// appcntr1's devices must stay ahead of appcntr2's so that each container's
		// network attachments get the devices allocated to that container.
		It("should sort within a container and keep containers in checkpoint order", func() {
			rInfo, ok := resourceMap["intel.com/sriov_net_A"]
			Expect(ok).To(BeTrue())
			Expect(rInfo.DeviceIDs).To(Equal([]string{
				"0000:03:02.0", "0000:03:02.3",
				"0000:03:02.1", "0000:03:02.5",
			}))
		})

		It("should sort the device IDs a container owns across NUMA nodes", func() {
			rInfo, ok := resourceMap["intel.com/sriov_net_B"]
			Expect(ok).To(BeTrue())
			Expect(rInfo.DeviceIDs).To(Equal([]string{"0000:03:06.0", "0000:03:06.3"}))
		})

		It("should not include device IDs allocated to another pod", func() {
			Expect(resourceMap["intel.com/sriov_net_A"].DeviceIDs).NotTo(ContainElement("0000:03:02.7"))
		})
	})

	Context("Using faulty or incompatible information", func() {
		var (
			cp  types.ResourceClient
			err error
		)

		It("should not get a Checkpoint instance from file given bad filepath", func() {
			_, err = getCheckpoint("invalid/file/path")
			Expect(err).To(HaveOccurred())
		})

		It("should not get a Checkpoint instance from file given bad json", func() {
			sampleData := `{
				"Data": {
					"PodDeviceEntries": [
					{
						"PodUID": "970a395d-bb3b-11e8-89df-408d5c537d23",
						"ContainerName": "appcntr1",
						"ResourceName": "intel.com/sriov_net_A",
						"DeviceIDs": { "-1": [
						"0000:03:02.3",
						"0000:03:02.0"
						] },
						"AllocResp": "CikKC3NyaW92X25ldF9BEhogMDAwMDowMzowMi4zIDAwMDA6MDM6MDIuMA=="
					}
					],
					"RegisteredDevices": {
					"intel.com/sriov_net_A": [
						"0000:03:02.1",
						"0000:03:02.2",
						"0000:03:02.3",
						"0000:03:02.0"
					],
					"intel.com/sriov_net_B": [
						"0000:03:06.3",
						"0000:03:06.0",
						"0000:03:06.1",
						"0000:03:06.2"
					]
					}
				},
				"Checksum": 229855270
				}`

			//missing a close bracket
			badSampleData := `BAD BAD DATA`

			fakeCheckpoint := &fakeCheckpoint{fileName: fakeTempFile}
			fakeCheckpoint.WriteToFile([]byte(badSampleData))
			_, err = getCheckpoint(fakeTempFile)
			Expect(err).To(HaveOccurred())
			fakeCheckpoint.WriteToFile([]byte(sampleData))
		})

		It("should not return a ResourceMap instance", func() {
			cp, err = getCheckpoint(fakeTempFile)
			podUID := k8sTypes.UID("")
			fakePod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fakePod",
					Namespace: "podNamespace",
					UID:       podUID,
				},
			}
			fmt.Println("fakePod-podID: ", fakePod.UID)
			rmap, err := cp.GetPodResourceMap(fakePod)
			Expect(err).To(HaveOccurred())
			Expect(rmap).To(BeEmpty())
		})
	})
})

var _ = AfterSuite(func() {
	fakeCheckpoint := &fakeCheckpoint{fileName: fakeTempFile}
	err := fakeCheckpoint.DeleteFile()
	Expect(err).NotTo(HaveOccurred())
})
