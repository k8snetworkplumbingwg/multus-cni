// Copyright (c) 2025 Multus Authors
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

package draclient

// disable dot-imports only for testing
//revive:disable:dot-imports
import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	resourcev1api "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

var _ = Describe("DRA Client operations", func() {

	Describe("NewClient", func() {
		It("should create a new DRA client successfully", func() {
			fakeClient := fake.NewSimpleClientset()
			client := NewClient(fakeClient.ResourceV1())
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("GetPodResourceMap", func() {
		var (
			fakeClient *fake.Clientset
			draClient  ClientInterace
		)

		BeforeEach(func() {
			fakeClient = fake.NewSimpleClientset()
			draClient = NewClient(fakeClient.ResourceV1())
		})

		Context("when pod has no resource claims", func() {
			It("should return empty resource map without error", func() {
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{},
					},
				}

				resourceMap := make(map[string]*types.ResourceInfo)
				err := draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})
		})

		Context("when resource claim exists with valid device allocation", func() {
			It("should successfully populate resource map with device IDs", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceIDValue,
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim with allocation
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  deviceName,
									},
								},
							},
						},
					},
				}

				// Create pod with resource claim status
				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add objects to fake client
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Execute
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				// Verify
				expectedKey := fmt.Sprintf("%s/%s", claimName, requestName)
				Expect(resourceMap).To(HaveKey(expectedKey))
				Expect(resourceMap[expectedKey].DeviceIDs).To(Equal([]string{deviceID}))
			})
		})

		Context("when multiple devices are allocated to the same claim/request", func() {
			It("should append all device IDs to the resource map", func() {
				claimName := "multi-device-claim"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				device1Name := "device-1"
				device2Name := "device-2"
				deviceID1 := "pci:0000:00:01.0"
				deviceID2 := "pci:0000:00:02.0"

				// Create ResourceSlice with multiple devices
				deviceID1Value := deviceID1
				deviceID2Value := deviceID2
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: device1Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceID1Value,
									},
								},
							},
							{
								Name: device2Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceID2Value,
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim with multiple device allocations
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  device1Name,
									},
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  device2Name,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add objects to fake client
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Execute
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				// Verify
				expectedKey := fmt.Sprintf("%s/%s", claimName, requestName)
				Expect(resourceMap).To(HaveKey(expectedKey))
				Expect(resourceMap[expectedKey].DeviceIDs).To(ConsistOf(deviceID1, deviceID2))
			})
		})

		Context("when resource claim does not exist", func() {
			It("should return an error", func() {
				claimName := "non-existent-claim"
				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				resourceMap := make(map[string]*types.ResourceInfo)
				err := draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		Context("when resource slice does not exist", func() {
			It("should return an error", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"

				// Create ResourceClaim but no ResourceSlice
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  deviceName,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add only resource claim
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Execute
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("expected 1 resource slice, got 0"))
			})
		})

		Context("when device does not have deviceID attribute", func() {
			It("should return an error", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"

				// Create ResourceSlice with device but NO deviceID attribute
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									// Missing k8s.cni.cncf.io/deviceID
									"some-other-attribute": {
										StringValue: func() *string { s := "value"; return &s }(),
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  deviceName,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add objects
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Execute
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found for claim resource"))
			})
		})

		Context("when device name in allocation does not match any device in slice", func() {
			It("should return an error", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				wrongDeviceName := "wrong-device"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceIDValue,
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim with WRONG device name
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  wrongDeviceName,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add objects
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Execute
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found for claim resource"))
			})
		})

		Context("when caching is working correctly", func() {
			It("should cache resource claims and slices", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceIDValue,
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  deviceName,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add objects to fake client
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// First call - should populate cache
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				expectedKey := fmt.Sprintf("%s/%s", claimName, requestName)
				Expect(resourceMap).To(HaveKey(expectedKey))
				Expect(resourceMap[expectedKey].DeviceIDs).To(Equal([]string{deviceID}))

				// Delete the objects from the API server
				err = fakeClient.ResourceV1().ResourceClaims("default").Delete(context.TODO(), claimName, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())
				err = fakeClient.ResourceV1().ResourceSlices().Delete(context.TODO(), resourceSlice.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Second call - should use cache and succeed even though objects are deleted
				resourceMap2 := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap2).To(HaveKey(expectedKey))
				Expect(resourceMap2[expectedKey].DeviceIDs).To(Equal([]string{deviceID}))
			})
		})

		Context("when pod has multiple different claims", func() {
			It("should populate resource map with all claims sequentially", func() {
				// Note: Due to fake client limitations with field selectors,
				// we test each claim separately to avoid conflicts

				claim1Name := "claim-1"
				device1Name := "device-1"
				driver1Name := "driver1.example.com"
				pool1Name := "pool-1"
				request1Name := "gpu"
				deviceID1 := "pci:0000:00:01.0"

				// First claim setup
				deviceID1Value := deviceID1
				resourceSlice1 := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice-1",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driver1Name,
						Pool: resourcev1api.ResourcePool{
							Name:               pool1Name,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: device1Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceID1Value,
									},
								},
							},
						},
					},
				}

				resourceClaim1 := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claim1Name,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: request1Name,
										Driver:  driver1Name,
										Pool:    pool1Name,
										Device:  device1Name,
									},
								},
							},
						},
					},
				}

				claim1NamePtr := claim1Name
				pod1 := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-1",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid-1"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claim1NamePtr,
							},
						},
					},
				}

				// Add first claim objects
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim1, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice1, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Test first claim
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod1, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				expectedKey1 := fmt.Sprintf("%s/%s", claim1Name, request1Name)
				Expect(resourceMap).To(HaveKey(expectedKey1))
				Expect(resourceMap[expectedKey1].DeviceIDs).To(Equal([]string{deviceID1}))

				// Now test second claim with a fresh client to avoid field selector issues
				claim2Name := "claim-2"
				device2Name := "device-2"
				driver2Name := "driver2.example.com"
				pool2Name := "pool-2"
				request2Name := "nic"
				deviceID2 := "pci:0000:00:02.0"

				fakeClient2 := fake.NewSimpleClientset()
				draClient2 := NewClient(fakeClient2.ResourceV1())

				deviceID2Value := deviceID2
				resourceSlice2 := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice-2",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driver2Name,
						Pool: resourcev1api.ResourcePool{
							Name:               pool2Name,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: device2Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceID2Value,
									},
								},
							},
						},
					},
				}

				resourceClaim2 := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claim2Name,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: request2Name,
										Driver:  driver2Name,
										Pool:    pool2Name,
										Device:  device2Name,
									},
								},
							},
						},
					},
				}

				claim2NamePtr := claim2Name
				pod2 := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-2",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid-2"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claim2NamePtr,
							},
						},
					},
				}

				// Add second claim objects
				_, err = fakeClient2.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient2.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Test second claim
				resourceMap2 := make(map[string]*types.ResourceInfo)
				err = draClient2.GetPodResourceMap(pod2, resourceMap2)
				Expect(err).NotTo(HaveOccurred())

				expectedKey2 := fmt.Sprintf("%s/%s", claim2Name, request2Name)
				Expect(resourceMap2).To(HaveKey(expectedKey2))
				Expect(resourceMap2[expectedKey2].DeviceIDs).To(Equal([]string{deviceID2}))
			})
		})

		Context("when resource map already has an entry for the claim/request", func() {
			It("should append device IDs to existing entry", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"
				existingDeviceID := "pci:0000:00:00.0"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceIDValue,
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  deviceName,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add objects
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Pre-populate resourceMap with existing entry
				resourceMap := make(map[string]*types.ResourceInfo)
				expectedKey := fmt.Sprintf("%s/%s", claimName, requestName)
				resourceMap[expectedKey] = &types.ResourceInfo{
					DeviceIDs: []string{existingDeviceID},
				}

				// Execute
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				// Verify device ID was appended
				Expect(resourceMap).To(HaveKey(expectedKey))
				Expect(resourceMap[expectedKey].DeviceIDs).To(Equal([]string{existingDeviceID, deviceID}))
			})
		})

		Context("when multiple resource slices exist for the same driver/pool", func() {
			It("should return an error", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"

				// Create TWO ResourceSlices with same driver/pool
				deviceIDValue := deviceID
				resourceSlice1 := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice-1",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceIDValue,
									},
								},
							},
						},
					},
				}

				resourceSlice2 := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-resource-slice-2",
					},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool: resourcev1api.ResourcePool{
							Name:               poolName,
							ResourceSliceCount: 1,
						},
						Devices: []resourcev1api.Device{
							{
								Name: "device-2",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									"k8s.cni.cncf.io/deviceID": {
										StringValue: &deviceIDValue,
									},
								},
							},
						},
					},
				}

				// Create ResourceClaim
				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{
										Request: requestName,
										Driver:  driverName,
										Pool:    poolName,
										Device:  deviceName,
									},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				// Add all objects - this creates ambiguity
				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Add both resource slices with same driver/pool
				var objects []runtime.Object
				objects = append(objects, resourceSlice1, resourceSlice2)
				fakeClient2 := fake.NewSimpleClientset(objects...)
				draClient2 := NewClient(fakeClient2.ResourceV1())

				// Also need to add the claim to the new client
				_, err = fakeClient2.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				// Execute - should fail because there are 2 slices
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient2.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("expected 1 resource slice, got 2"))
			})
		})
	})
})
