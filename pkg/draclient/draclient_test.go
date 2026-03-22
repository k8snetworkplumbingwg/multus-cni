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
			draClient  ClientInterface
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
				mapKey := "intel.com/gpu-vf"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				mapKeyValue := mapKey
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
									multusDeviceIDAttr: {
										StringValue: &deviceIDValue,
									},
									multusResourceNameAttr: {
										StringValue: &mapKeyValue,
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
								Name:              claimName, // spec ref name (pod.spec.resourceClaims[].name)
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

				Expect(resourceMap).To(HaveKey(mapKey))
				Expect(resourceMap[mapKey].DeviceIDs).To(Equal([]string{deviceID}))
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
				mapKey := "example.com/multi-gpu"

				// Create ResourceSlice with multiple devices (same resourceName → one NAD, multiple device IDs)
				deviceID1Value := deviceID1
				deviceID2Value := deviceID2
				mapKeyValue := mapKey
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
									multusDeviceIDAttr: {
										StringValue: &deviceID1Value,
									},
									multusResourceNameAttr: {StringValue: &mapKeyValue},
								},
							},
							{
								Name: device2Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr: {
										StringValue: &deviceID2Value,
									},
									multusResourceNameAttr: {StringValue: &mapKeyValue},
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
								Name:              claimName,
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

				Expect(resourceMap).To(HaveKey(mapKey))
				Expect(resourceMap[mapKey].DeviceIDs).To(ConsistOf(deviceID1, deviceID2))
			})
		})

		Context("when one claim allocates multiple devices with distinct resourceName attributes", func() {
			It("should populate separate resource map entries per network", func() {
				claimName := "dual-net-claim"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				device1Name := "dev-net-a"
				device2Name := "dev-net-b"
				deviceID1 := "pci:0000:00:01.0"
				deviceID2 := "pci:0000:00:02.0"
				keyA := "vendor.com/net-a"
				keyB := "vendor.com/net-b"

				d1, d2 := deviceID1, deviceID2
				kA, kB := keyA, keyB
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-dual"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: device1Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &d1},
									multusResourceNameAttr: {StringValue: &kA},
								},
							},
							{
								Name: device2Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &d2},
									multusResourceNameAttr: {StringValue: &kB},
								},
							},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: "req-a", Driver: driverName, Pool: poolName, Device: device1Name},
									{Request: "req-b", Driver: driverName, Pool: poolName, Device: device2Name},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-dual", Namespace: "default", UID: k8sTypes.UID("uid-dual")},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claimName, ResourceClaimName: &claimNamePtr},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				Expect(resourceMap).To(HaveKey(keyA))
				Expect(resourceMap[keyA].DeviceIDs).To(Equal([]string{deviceID1}))
				Expect(resourceMap).To(HaveKey(keyB))
				Expect(resourceMap[keyB].DeviceIDs).To(Equal([]string{deviceID2}))
			})
		})

		Context("when pod has ExtendedResourceClaimStatus (extended resource feature gate)", func() {
			It("should populate resource map using extended resource name as key", func() {
				claimName := "ext-resource-dual-port-extended-resources-4sd7g"
				device1Name := "0000-08-00-3"
				device2Name := "0000-08-02-2"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "c-234-183-40-044"
				deviceID1 := "0000:08:00.3"
				deviceID2 := "0000:08:02.2"

				deviceID1Value := deviceID1
				deviceID2Value := deviceID2
				rnPort1 := "example.com/sriov-port1"
				rnPort2 := "example.com/sriov-port2"
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
									multusDeviceIDAttr:     {StringValue: &deviceID1Value},
									multusResourceNameAttr: {StringValue: &rnPort1},
								},
							},
							{
								Name: device2Name,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &deviceID2Value},
									multusResourceNameAttr: {StringValue: &rnPort2},
								},
							},
						},
					},
				}

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
										Request: "container-0-request-0",
										Driver:  driverName,
										Pool:    poolName,
										Device:  device1Name,
									},
									{
										Request: "container-0-request-1",
										Driver:  driverName,
										Pool:    poolName,
										Device:  device2Name,
									},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ext-resource-dual-port",
						Namespace: "default",
						UID:       k8sTypes.UID("dc0b90ad-0ca2-4d83-bac9-61e053a86850"),
					},
					Status: v1.PodStatus{
						ResourceClaimStatuses: nil,
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{
									ContainerName: "test-container",
									ResourceName:  "example.com/sriov-port1",
									RequestName:   "container-0-request-0",
								},
								{
									ContainerName: "test-container",
									ResourceName:  "example.com/sriov-port2",
									RequestName:   "container-0-request-1",
								},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				Expect(resourceMap).To(HaveKey("example.com/sriov-port1"))
				Expect(resourceMap["example.com/sriov-port1"].DeviceIDs).To(Equal([]string{deviceID1}))
				Expect(resourceMap).To(HaveKey("example.com/sriov-port2"))
				Expect(resourceMap["example.com/sriov-port2"].DeviceIDs).To(Equal([]string{deviceID2}))
			})

			It("should populate resource map with multiple devices when request has count > 1", func() {
				claimName := "multus-dual-port-extended-resources-ftknr"
				device1Name := "0000-08-00-3"
				device2Name := "0000-08-00-4"
				device3Name := "0000-08-02-4"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "c-237-169-100-104"
				deviceID1 := "0000:08:00.3"
				deviceID2 := "0000:08:00.4"
				deviceID3 := "0000:08:02.4"
				rnP1 := "nvidia.com/sriov-port1"
				rnP2 := "nvidia.com/sriov-port2"

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
							{Name: device1Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &deviceID1}, multusResourceNameAttr: {StringValue: &rnP1},
							}},
							{Name: device2Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &deviceID2}, multusResourceNameAttr: {StringValue: &rnP1},
							}},
							{Name: device3Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &deviceID3}, multusResourceNameAttr: {StringValue: &rnP2},
							}},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      claimName,
						Namespace: "default",
					},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: "container-0-request-0", Driver: driverName, Pool: poolName, Device: device1Name},
									{Request: "container-0-request-0", Driver: driverName, Pool: poolName, Device: device2Name},
									{Request: "container-0-request-1", Driver: driverName, Pool: poolName, Device: device3Name},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multus-dual-port",
						Namespace: "default",
						UID:       k8sTypes.UID("895bdfd9-a532-4127-97b5-2a0aa5cc91a7"),
					},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "test-container", ResourceName: "nvidia.com/sriov-port1", RequestName: "container-0-request-0"},
								{ContainerName: "test-container", ResourceName: "nvidia.com/sriov-port2", RequestName: "container-0-request-1"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				Expect(resourceMap).To(HaveKey("nvidia.com/sriov-port1"))
				Expect(resourceMap["nvidia.com/sriov-port1"].DeviceIDs).To(ConsistOf(deviceID1, deviceID2))
				Expect(resourceMap).To(HaveKey("nvidia.com/sriov-port2"))
				Expect(resourceMap["nvidia.com/sriov-port2"].DeviceIDs).To(Equal([]string{deviceID3}))
			})

			It("should return an error when device resourceName attribute does not match extended mapping", func() {
				claimName := "ext-mismatch-claim"
				deviceName := "0000-08-00-3"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "pool-1"
				deviceID := "0000:08:00.3"
				wrongRN := "wrong.example.com/port"
				deviceIDVal := deviceID

				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-mismatch"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &deviceIDVal},
									multusResourceNameAttr: {StringValue: &wrongRN},
								},
							},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: "req-0", Driver: driverName, Pool: poolName, Device: deviceName},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-mismatch", Namespace: "default", UID: k8sTypes.UID("uid-mismatch")},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: "expected.example.com/port", RequestName: "req-0"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(multusResourceNameAttr))
				Expect(err.Error()).To(ContainSubstring("expected.example.com/port"))
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
								Name:              claimName,
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
								Name:              claimName,
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
				Expect(err.Error()).To(ContainSubstring("no resource slice found for driver/pool"))
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
									// Missing multusDeviceIDAttr
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
								Name:              claimName,
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

		Context("when device has deviceID but missing resourceName attribute", func() {
			It("should return an error", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"
				deviceIDValue := deviceID

				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "test-resource-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: deviceName,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr: {StringValue: &deviceIDValue},
								},
							},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: requestName, Driver: driverName, Pool: poolName, Device: deviceName},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default", UID: k8sTypes.UID("test-uid")},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claimName, ResourceClaimName: &claimNamePtr},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(multusResourceNameAttr))
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
									multusDeviceIDAttr: {
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
								Name:              claimName,
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
				mapKey := "cache-test.example.com/res"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				mapKeyValue := mapKey
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
									multusDeviceIDAttr: {
										StringValue: &deviceIDValue,
									},
									multusResourceNameAttr: {StringValue: &mapKeyValue},
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
								Name:              claimName,
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

				expectedKey := mapKey
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

		Context("when ResourceClaim names collide across namespaces", func() {
			It("should cache claims separately per namespace", func() {
				claimName := "same-claim-name"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				mapKey := "cross-ns.example.com/res"
				nsA, nsB := "team-a", "team-b"
				deviceA, deviceB := "dev-a", "dev-b"
				pciA, pciB := "pci:0000:01:00.0", "pci:0000:02:00.0"

				mapKeyVal := mapKey
				pciAVal, pciBVal := pciA, pciB

				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "shared-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: deviceA,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &pciAVal},
									multusResourceNameAttr: {StringValue: &mapKeyVal},
								},
							},
							{
								Name: deviceB,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &pciBVal},
									multusResourceNameAttr: {StringValue: &mapKeyVal},
								},
							},
						},
					},
				}

				claimA := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: nsA},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: requestName, Driver: driverName, Pool: poolName, Device: deviceA},
								},
							},
						},
					},
				}
				claimB := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: nsB},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: requestName, Driver: driverName, Pool: poolName, Device: deviceB},
								},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims(nsA).Create(context.TODO(), claimA, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceClaims(nsB).Create(context.TODO(), claimB, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				claimPtr := claimName
				podA := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: nsA, UID: k8sTypes.UID("uid-a")},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claimName, ResourceClaimName: &claimPtr},
						},
					},
				}
				podB := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: nsB, UID: k8sTypes.UID("uid-b")},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claimName, ResourceClaimName: &claimPtr},
						},
					},
				}

				m1 := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(podA, m1)
				Expect(err).NotTo(HaveOccurred())
				Expect(m1[mapKey].DeviceIDs).To(Equal([]string{pciA}))

				m2 := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(podB, m2)
				Expect(err).NotTo(HaveOccurred())
				Expect(m2[mapKey].DeviceIDs).To(Equal([]string{pciB}))
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
				mapKey1 := "driver1.example.com/gpu-net"

				// First claim setup
				deviceID1Value := deviceID1
				mapKey1Val := mapKey1
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
									multusDeviceIDAttr: {
										StringValue: &deviceID1Value,
									},
									multusResourceNameAttr: {StringValue: &mapKey1Val},
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
								Name:              claim1Name,
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

				Expect(resourceMap).To(HaveKey(mapKey1))
				Expect(resourceMap[mapKey1].DeviceIDs).To(Equal([]string{deviceID1}))

				// Now test second claim with a fresh client to avoid field selector issues
				claim2Name := "claim-2"
				device2Name := "device-2"
				driver2Name := "driver2.example.com"
				pool2Name := "pool-2"
				request2Name := "nic"
				deviceID2 := "pci:0000:00:02.0"
				mapKey2 := "driver2.example.com/nic-net"

				fakeClient2 := fake.NewSimpleClientset()
				draClient2 := NewClient(fakeClient2.ResourceV1())

				deviceID2Value := deviceID2
				mapKey2Val := mapKey2
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
									multusDeviceIDAttr: {
										StringValue: &deviceID2Value,
									},
									multusResourceNameAttr: {StringValue: &mapKey2Val},
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
								Name:              claim2Name,
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

				Expect(resourceMap2).To(HaveKey(mapKey2))
				Expect(resourceMap2[mapKey2].DeviceIDs).To(Equal([]string{deviceID2}))
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
				mapKey := "append-test.example.com/res"

				// Create ResourceSlice with device
				deviceIDValue := deviceID
				mapKeyValue := mapKey
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
									multusDeviceIDAttr: {
										StringValue: &deviceIDValue,
									},
									multusResourceNameAttr: {StringValue: &mapKeyValue},
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
								Name:              claimName,
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
				expectedKey := mapKey
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
			It("should find the allocated device by searching all slices", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"
				mapKey := "multi-slice.example.com/res"

				// DRA allows multiple ResourceSlices per driver/pool (e.g. per NUMA). Allocation references device-1
				// which lives only in the first slice; second slice holds another device.
				deviceIDValue := deviceID
				mapKeyValue := mapKey
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
									multusDeviceIDAttr: {
										StringValue: &deviceIDValue,
									},
									multusResourceNameAttr: {StringValue: &mapKeyValue},
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
									multusDeviceIDAttr: {
										StringValue: &deviceIDValue,
									},
									multusResourceNameAttr: {StringValue: &mapKeyValue},
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
								Name:              claimName,
								ResourceClaimName: &claimNamePtr,
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				var objects []runtime.Object
				objects = append(objects, resourceSlice1, resourceSlice2)
				fakeClient2 := fake.NewSimpleClientset(objects...)
				draClient2 := NewClient(fakeClient2.ResourceV1())

				_, err = fakeClient2.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient2.GetPodResourceMap(pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(HaveKey(mapKey))
				Expect(resourceMap[mapKey].DeviceIDs).To(Equal([]string{deviceID}))
			})
		})
	})
})
