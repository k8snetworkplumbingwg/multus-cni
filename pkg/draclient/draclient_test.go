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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	resourcev1api "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

// enforceResourceSliceFieldSelectors makes the fake clientset honor the spec.nodeName and
// spec.driver field selectors on ResourceSlice list calls, like the real API server does.
// A spec.nodeName selector never returns slices that have no nodeName (AllNodes/NodeSelector).
func enforceResourceSliceFieldSelectors(fakeClient *fake.Clientset) {
	fakeClient.PrependReactor("list", "resourceslices", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction := action.(k8stesting.ListAction)
		obj, err := fakeClient.Tracker().List(
			resourcev1api.SchemeGroupVersion.WithResource("resourceslices"),
			resourcev1api.SchemeGroupVersion.WithKind("ResourceSlice"), "")
		if err != nil {
			return true, nil, err
		}
		all := obj.(*resourcev1api.ResourceSliceList)
		filtered := &resourcev1api.ResourceSliceList{}
		for _, s := range all.Items {
			nodeName := ""
			if s.Spec.NodeName != nil {
				nodeName = *s.Spec.NodeName
			}
			set := fields.Set{
				resourcev1api.ResourceSliceSelectorNodeName: nodeName,
				resourcev1api.ResourceSliceSelectorDriver:   s.Spec.Driver,
			}
			if listAction.GetListRestrictions().Fields.Matches(set) {
				filtered.Items = append(filtered.Items, s)
			}
		}
		return true, filtered, nil
	})
}

var _ = Describe("DRA Client operations", func() {

	Describe("NewClient", func() {
		It("should create a new DRA client successfully", func() {
			fakeClient := fake.NewClientset()
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
			fakeClient = fake.NewClientset()
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
				err := draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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

				// Create pod with resource claim status.
				// Spec.NodeName is set so ensureDriverCachePopulated takes the
				// node-scoped selector branch (spec.nodeName + spec.driver).
				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						UID:       k8sTypes.UID("test-uid"),
					},
					Spec: v1.PodSpec{
						NodeName: "test-node",
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				Expect(resourceMap).To(HaveKey(mapKey))
				Expect(resourceMap[mapKey].DeviceIDs).To(Equal([]string{deviceID}))
			})

			It("should not resolve a device that only exists in a slice on a different node", func() {
				// The claim allocates a device published only by a slice on another node. The
				// node field selector must keep that slice out of the cache, so the device ID
				// must not be resolved.
				enforceResourceSliceFieldSelectors(fakeClient)

				claimName := "other-node-claim"
				driverName := "test-driver.example.com"
				poolName := "other-pool"
				deviceID := "pci:0000:00:99.0"
				mapKey := "intel.com/sriov-vf"
				otherNodeName := "other-node"

				otherSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-other-node-only"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   driverName,
						NodeName: &otherNodeName,
						Pool:     resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{Name: "other-device", Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr:     {StringValue: &deviceID},
								multusResourceNameAttr: {StringValue: &mapKey},
							}},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{Allocation: &resourcev1api.AllocationResult{
						Devices: resourcev1api.DeviceAllocationResult{Results: []resourcev1api.DeviceRequestAllocationResult{
							{Request: "sriov", Driver: driverName, Pool: poolName, Device: "other-device"},
						}},
					}},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "other-node-pod", Namespace: "default"},
					Spec:       v1.PodSpec{NodeName: "test-node"},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claimName, ResourceClaimName: &claimNamePtr},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), otherSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).NotTo(HaveKey(mapKey))
			})

			It("should not include devices from a different node when the pod has a nodeName set", func() {
				// This test verifies node-scoping: when nodeName is non-empty the code
				// uses a spec.nodeName/spec.driver field selector. The fake API server
				// does not enforce server-side field selectors, but we can still confirm
				// that only the device actually allocated to the pod's claim appears in
				// the result — devices from a slice that is on a different node but
				// shares the same driver are a no-op because the claim does not reference
				// their device/pool.
				claimName := "node-scoped-claim"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				deviceName := "device-1"
				deviceID := "pci:0000:00:01.0"
				mapKey := "intel.com/sriov-vf"

				deviceIDValue := deviceID
				mapKeyValue := mapKey
				podNodeName := "test-node"
				otherNodeNameVal := "other-node"
				// Slice for the pod's node.
				sliceOnPodNode := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-test-node"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   driverName,
						NodeName: &podNodeName,
						Pool:     resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{Name: deviceName, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr:     {StringValue: &deviceIDValue},
								multusResourceNameAttr: {StringValue: &mapKeyValue},
							}},
						},
					},
				}

				otherDeviceID := "pci:0000:00:99.0"
				otherDeviceIDValue := otherDeviceID
				otherMapKeyValue := mapKey
				// Slice for a different node — should not produce entries for this pod's claim.
				sliceOnOtherNode := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-other-node"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   driverName,
						NodeName: &otherNodeNameVal,
						Pool:     resourcev1api.ResourcePool{Name: "other-pool", ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{Name: "other-device", Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr:     {StringValue: &otherDeviceIDValue},
								multusResourceNameAttr: {StringValue: &otherMapKeyValue},
							}},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "scoped-pod", Namespace: "default"},
					Spec:       v1.PodSpec{NodeName: "test-node"},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claimName, ResourceClaimName: &claimNamePtr},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{Allocation: &resourcev1api.AllocationResult{
						Devices: resourcev1api.DeviceAllocationResult{Results: []resourcev1api.DeviceRequestAllocationResult{
							{Request: "sriov", Driver: driverName, Pool: poolName, Device: deviceName},
						}},
					}},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), sliceOnPodNode, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), sliceOnOtherNode, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				// Only the device allocated by the claim (on "test-node") must appear.
				Expect(resourceMap).To(HaveKey(mapKey))
				Expect(resourceMap[mapKey].DeviceIDs).To(ConsistOf(deviceID))
				// The other-node device ID must not be present.
				Expect(resourceMap[mapKey].DeviceIDs).NotTo(ContainElement(otherDeviceID))
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(multusResourceNameAttr))
				Expect(err.Error()).To(ContainSubstring("expected.example.com/port"))
			})

			// Regression tests for https://github.com/k8snetworkplumbingwg/multus-cni/issues/1559:
			// a DRA-backed extended resource such as nvidia.com/gpu is not a network device and
			// its devices carry no k8s.cni.cncf.io/* attributes. Multus must ignore it instead of
			// failing CNI ADD (the plain-claim path already behaves this way).
			It("should ignore a non-network extended resource whose devices carry no Multus attributes", func() {
				claimName := "gpu-pod-extended-resources-abcde"
				driverName := "gpu.nvidia.com"
				poolName := "node-gpu-pool"
				podNodeName := "test-node"

				enforceResourceSliceFieldSelectors(fakeClient)

				gpuSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "gpu-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   driverName,
						NodeName: &podNodeName,
						Pool:     resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{Name: "gpu-0"},
							{Name: "gpu-1"},
						},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: "container-0-request-0", Driver: driverName, Pool: poolName, Device: "gpu-0"},
									{Request: "container-0-request-0", Driver: driverName, Pool: poolName, Device: "gpu-1"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod", Namespace: "default", UID: k8sTypes.UID("uid-gpu-only")},
					Spec:       v1.PodSpec{NodeName: podNodeName},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: "nvidia.com/gpu", RequestName: "container-0-request-0"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), gpuSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})

			It("should ignore a device that has a deviceID but no resourceName attribute", func() {
				claimName := "no-rn-extended-resources-abcde"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "pool-no-rn"
				deviceID := "0000:08:00.3"

				slice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-no-rn"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: "0000-08-00-3",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr: {StringValue: &deviceID},
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
									{Request: "req-0", Driver: driverName, Pool: poolName, Device: "0000-08-00-3"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-no-rn", Namespace: "default", UID: k8sTypes.UID("uid-no-rn")},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: "example.com/port", RequestName: "req-0"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), slice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})

			It("should find a network device published in an AllNodes slice when the list is node-filtered", func() {
				enforceResourceSliceFieldSelectors(fakeClient)

				claimName := "allnodes-extended-resources-abcde"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "shared-pool"
				deviceID := "0000:08:00.3"
				rn := "example.com/port"
				allNodes := true

				slice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-all-nodes"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   driverName,
						AllNodes: &allNodes,
						Pool:     resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: "shared-dev",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &deviceID},
									multusResourceNameAttr: {StringValue: &rn},
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
									{Request: "req-0", Driver: driverName, Pool: poolName, Device: "shared-dev"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-all-nodes", Namespace: "default", UID: k8sTypes.UID("uid-all-nodes")},
					Spec:       v1.PodSpec{NodeName: "test-node"},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: rn, RequestName: "req-0"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), slice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(HaveKey(rn))
				Expect(resourceMap[rn].DeviceIDs).To(Equal([]string{deviceID}))
			})

			It("should return an error when an allocated device is absent from every ResourceSlice", func() {
				claimName := "absent-device-extended-resources-abcde"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "pool-absent"
				deviceID := "0000:08:00.3"
				rn := "example.com/port"

				// The slice publishes a different device than the one allocated.
				slice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-absent"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: "other-device",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &deviceID},
									multusResourceNameAttr: {StringValue: &rn},
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
									{Request: "req-0", Driver: driverName, Pool: poolName, Device: "missing-device"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-absent", Namespace: "default", UID: k8sTypes.UID("uid-absent")},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: rn, RequestName: "req-0"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), slice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, errDeviceNotInAnySlice)).To(BeTrue())
				Expect(resourceMap).To(BeEmpty())
			})

			It("should return an error when a device without deviceID has a resourceName matching the mapping", func() {
				claimName := "match-no-id-extended-resources-abcde"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "pool-match-no-id"
				rn := "example.com/port"

				slice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-match-no-id"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: "dev-0",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusResourceNameAttr: {StringValue: &rn},
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
									{Request: "req-0", Driver: driverName, Pool: poolName, Device: "dev-0"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-match-no-id", Namespace: "default", UID: k8sTypes.UID("uid-match-no-id")},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: rn, RequestName: "req-0"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), slice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(multusDeviceIDAttr))
				Expect(resourceMap).To(BeEmpty())
			})

			It("should return an error when a device without deviceID has a resourceName that does not match the mapping", func() {
				claimName := "mismatch-no-id-extended-resources-abcde"
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "pool-mismatch-no-id"
				wrongRN := "wrong.example.com/port"

				slice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "slice-mismatch-no-id"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: "dev-0",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
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
									{Request: "req-0", Driver: driverName, Pool: poolName, Device: "dev-0"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "pod-mismatch-no-id", Namespace: "default", UID: k8sTypes.UID("uid-mismatch-no-id")},
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
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), slice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(multusResourceNameAttr))
				Expect(err.Error()).To(ContainSubstring("expected.example.com/port"))
			})

			It("should map network extended resources and ignore GPU extended resources in the same pod", func() {
				claimName := "mixed-extended-resources-abcde"
				netDriver := "sriovnetwork.k8snetworkplumbingwg.io"
				netPool := "net-pool"
				gpuDriver := "gpu.nvidia.com"
				gpuPool := "gpu-pool"
				deviceID := "0000:08:00.3"
				rn := "example.com/sriov-port1"
				podNodeName := "test-node"

				enforceResourceSliceFieldSelectors(fakeClient)

				netSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "mixed-net-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   netDriver,
						NodeName: &podNodeName,
						Pool:     resourcev1api.ResourcePool{Name: netPool, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: "0000-08-00-3",
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &deviceID},
									multusResourceNameAttr: {StringValue: &rn},
								},
							},
						},
					},
				}
				gpuSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "mixed-gpu-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:   gpuDriver,
						NodeName: &podNodeName,
						Pool:     resourcev1api.ResourcePool{Name: gpuPool, ResourceSliceCount: 1},
						Devices:  []resourcev1api.Device{{Name: "gpu-0"}},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: "container-0-request-0", Driver: gpuDriver, Pool: gpuPool, Device: "gpu-0"},
									{Request: "container-0-request-1", Driver: netDriver, Pool: netPool, Device: "0000-08-00-3"},
								},
							},
						},
					},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "mixed-pod", Namespace: "default", UID: k8sTypes.UID("uid-mixed")},
					Spec:       v1.PodSpec{NodeName: podNodeName},
					Status: v1.PodStatus{
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: claimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: "nvidia.com/gpu", RequestName: "container-0-request-0"},
								{ContainerName: "c", ResourceName: rn, RequestName: "container-0-request-1"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), netSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), gpuSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(HaveLen(1))
				Expect(resourceMap).To(HaveKey(rn))
				Expect(resourceMap[rn].DeviceIDs).To(Equal([]string{deviceID}))
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
				err := draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		Context("when resource slice does not exist", func() {
			It("should skip the claim and return empty resource map without error", func() {
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

				// Execute — no slice means List returns empty; device misses cache → errDeviceNotInAnySlice
				// → skipped silently (non-CNI claim graceful handling). No error, empty map.
				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})
		})

		Context("when device does not have deviceID attribute", func() {
			It("should warn and skip the claim without failing", func() {
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})
		})

		Context("when device has deviceID but missing resourceName attribute", func() {
			It("should warn and skip without failing when nothing maps to a resource name", func() {
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})
		})

		Context("when device name in allocation does not match any device in slice", func() {
			It("should warn and skip the claim without failing", func() {
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(BeEmpty())
			})

			It("should preserve existing kubelet map entries when the claim maps nothing", func() {
				claimName := "test-claim"
				deviceName := "device-1"
				wrongDeviceName := "wrong-device"
				driverName := "test-driver.example.com"
				poolName := "test-pool"
				requestName := "gpu"
				deviceID := "pci:0000:00:01.0"
				legacyKey := "example.com/legacy-vf"
				legacyPCI := "0000:8d:00.4"

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
									{Request: requestName, Driver: driverName, Pool: poolName, Device: wrongDeviceName},
								},
							},
						},
					},
				}
				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "hybrid-pod", Namespace: "default", UID: k8sTypes.UID("uid-hybrid")},
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

				resourceMap := map[string]*types.ResourceInfo{
					legacyKey: {DeviceIDs: []string{legacyPCI}},
				}
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap[legacyKey].DeviceIDs).To(Equal([]string{legacyPCI}))
			})

			It("should succeed when one allocation result is missing from slices but another resolves", func() {
				claimName := "multi-claim"
				driverSRIOV := "sriovnetwork.k8snetworkplumbingwg.io"
				driverGPU := "gpu.nvidia.com"
				poolName := "orch-dev-a100-002"
				deviceVF := "0000-8d-00-4"
				deviceGPU := "gpu-4"
				deviceIDVF := "pci:0000:8d:00.4"
				mapKey := "nvidia.com/port2"
				deviceIDVFVal := deviceIDVF
				mapKeyVal := mapKey

				// SR-IOV slice has the VF; empty GPU slice exists so List() finds gpu.nvidia.com/pool (real clusters
				// always publish a slice per driver/pool). Allocation still references gpu-4, which is not listed here.
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "sriov-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverSRIOV,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{
								Name: deviceVF,
								Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
									multusDeviceIDAttr:     {StringValue: &deviceIDVFVal},
									multusResourceNameAttr: {StringValue: &mapKeyVal},
								},
							},
						},
					},
				}
				gpuSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "gpu-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver:  driverGPU,
						Pool:    resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{},
					},
				}

				resourceClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{
						Allocation: &resourcev1api.AllocationResult{
							Devices: resourcev1api.DeviceAllocationResult{
								Results: []resourcev1api.DeviceRequestAllocationResult{
									{Request: "vf", Driver: driverSRIOV, Pool: poolName, Device: deviceVF},
									{Request: "gpu", Driver: driverGPU, Pool: poolName, Device: deviceGPU},
								},
							},
						},
					},
				}

				claimNamePtr := claimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "mixed-pod", Namespace: "default", UID: k8sTypes.UID("uid-mixed")},
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
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), gpuSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap[mapKey].DeviceIDs).To(Equal([]string{deviceIDVF}))
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap2).To(HaveKey(expectedKey))
				Expect(resourceMap2[expectedKey].DeviceIDs).To(Equal([]string{deviceID}))
			})
		})

		Context("when two claims share the same driver", func() {
			It("should populate the driver cache only once and resolve both devices", func() {
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "test-pool"
				device1Name := "dev-a"
				device2Name := "dev-b"
				deviceID1 := "pci:0000:01:00.0"
				deviceID2 := "pci:0000:02:00.0"
				mapKey1 := "sriov.example.com/net-a"
				mapKey2 := "sriov.example.com/net-b"
				claim1Name := "claim-a"
				claim2Name := "claim-b"

				dv1, dv2, mk1, mk2 := deviceID1, deviceID2, mapKey1, mapKey2
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "sriov-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{Name: device1Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &dv1}, multusResourceNameAttr: {StringValue: &mk1},
							}},
							{Name: device2Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &dv2}, multusResourceNameAttr: {StringValue: &mk2},
							}},
						},
					},
				}

				c1ptr, c2ptr := claim1Name, claim2Name
				resourceClaim1 := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claim1Name, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{Allocation: &resourcev1api.AllocationResult{
						Devices: resourcev1api.DeviceAllocationResult{Results: []resourcev1api.DeviceRequestAllocationResult{
							{Request: "req-a", Driver: driverName, Pool: poolName, Device: device1Name},
						}},
					}},
				}
				resourceClaim2 := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: claim2Name, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{Allocation: &resourcev1api.AllocationResult{
						Devices: resourcev1api.DeviceAllocationResult{Results: []resourcev1api.DeviceRequestAllocationResult{
							{Request: "req-b", Driver: driverName, Pool: poolName, Device: device2Name},
						}},
					}},
				}

				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "two-claim-pod", Namespace: "default", UID: k8sTypes.UID("uid-two")},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: claim1Name, ResourceClaimName: &c1ptr},
							{Name: claim2Name, ResourceClaimName: &c2ptr},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim1, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim2, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				Expect(resourceMap).To(HaveKey(mapKey1))
				Expect(resourceMap[mapKey1].DeviceIDs).To(Equal([]string{deviceID1}))
				Expect(resourceMap).To(HaveKey(mapKey2))
				Expect(resourceMap[mapKey2].DeviceIDs).To(Equal([]string{deviceID2}))

				// Verify the driver cache was populated after the first call: delete the slice from
				// the API server and call again — if cache is working, it must still resolve correctly.
				err = fakeClient.ResourceV1().ResourceSlices().Delete(context.TODO(), resourceSlice.Name, metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap2 := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap2[mapKey1].DeviceIDs).To(Equal([]string{deviceID1}))
				Expect(resourceMap2[mapKey2].DeviceIDs).To(Equal([]string{deviceID2}))
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
				err = draClient.GetPodResourceMap(context.TODO(), podA, m1)
				Expect(err).NotTo(HaveOccurred())
				Expect(m1[mapKey].DeviceIDs).To(Equal([]string{pciA}))

				m2 := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), podB, m2)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod1, resourceMap)
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

				fakeClient2 := fake.NewClientset()
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
				err = draClient2.GetPodResourceMap(context.TODO(), pod2, resourceMap2)
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
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
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
				fakeClient2 := fake.NewClientset(objects...)
				draClient2 := NewClient(fakeClient2.ResourceV1())

				_, err = fakeClient2.ResourceV1().ResourceClaims("default").Create(context.TODO(), resourceClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient2.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceMap).To(HaveKey(mapKey))
				Expect(resourceMap[mapKey].DeviceIDs).To(Equal([]string{deviceID}))
			})
		})

		Context("when pod has both regular ResourceClaimStatuses and ExtendedResourceClaimStatus", func() {
			It("should populate resource map from both paths independently", func() {
				driverName := "sriovnetwork.k8snetworkplumbingwg.io"
				poolName := "test-pool"

				// Regular claim: one VF mapped via ResourceClaimStatuses
				regularClaimName := "regular-claim"
				regularDeviceName := "vf-0"
				regularDeviceID := "pci:0000:01:00.0"
				regularMapKey := "sriov.example.com/vf-net"

				// Extended claim: two ports mapped via ExtendedResourceClaimStatus
				extClaimName := "extended-claim"
				extDevice1Name := "vf-1"
				extDevice2Name := "vf-2"
				extDeviceID1 := "pci:0000:02:00.0"
				extDeviceID2 := "pci:0000:03:00.0"
				extMapKey1 := "sriov.example.com/port-a"
				extMapKey2 := "sriov.example.com/port-b"

				rdID, rmk := regularDeviceID, regularMapKey
				ed1, ed2 := extDeviceID1, extDeviceID2
				emk1, emk2 := extMapKey1, extMapKey2

				// One ResourceSlice covers all devices for the driver
				resourceSlice := &resourcev1api.ResourceSlice{
					ObjectMeta: metav1.ObjectMeta{Name: "sriov-slice"},
					Spec: resourcev1api.ResourceSliceSpec{
						Driver: driverName,
						Pool:   resourcev1api.ResourcePool{Name: poolName, ResourceSliceCount: 1},
						Devices: []resourcev1api.Device{
							{Name: regularDeviceName, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &rdID}, multusResourceNameAttr: {StringValue: &rmk},
							}},
							{Name: extDevice1Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &ed1}, multusResourceNameAttr: {StringValue: &emk1},
							}},
							{Name: extDevice2Name, Attributes: map[resourcev1api.QualifiedName]resourcev1api.DeviceAttribute{
								multusDeviceIDAttr: {StringValue: &ed2}, multusResourceNameAttr: {StringValue: &emk2},
							}},
						},
					},
				}

				regularClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: regularClaimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{Allocation: &resourcev1api.AllocationResult{
						Devices: resourcev1api.DeviceAllocationResult{Results: []resourcev1api.DeviceRequestAllocationResult{
							{Request: "vf", Driver: driverName, Pool: poolName, Device: regularDeviceName},
						}},
					}},
				}

				extClaim := &resourcev1api.ResourceClaim{
					ObjectMeta: metav1.ObjectMeta{Name: extClaimName, Namespace: "default"},
					Status: resourcev1api.ResourceClaimStatus{Allocation: &resourcev1api.AllocationResult{
						Devices: resourcev1api.DeviceAllocationResult{Results: []resourcev1api.DeviceRequestAllocationResult{
							{Request: "req-port-a", Driver: driverName, Pool: poolName, Device: extDevice1Name},
							{Request: "req-port-b", Driver: driverName, Pool: poolName, Device: extDevice2Name},
						}},
					}},
				}

				regularClaimPtr := regularClaimName
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "combo-pod", Namespace: "default", UID: k8sTypes.UID("uid-combo")},
					Status: v1.PodStatus{
						ResourceClaimStatuses: []v1.PodResourceClaimStatus{
							{Name: regularClaimName, ResourceClaimName: &regularClaimPtr},
						},
						ExtendedResourceClaimStatus: &v1.PodExtendedResourceClaimStatus{
							ResourceClaimName: extClaimName,
							RequestMappings: []v1.ContainerExtendedResourceRequest{
								{ContainerName: "c", ResourceName: extMapKey1, RequestName: "req-port-a"},
								{ContainerName: "c", ResourceName: extMapKey2, RequestName: "req-port-b"},
							},
						},
					},
				}

				_, err := fakeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), regularClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				_, err = fakeClient.ResourceV1().ResourceClaims("default").Create(context.TODO(), extClaim, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				resourceMap := make(map[string]*types.ResourceInfo)
				err = draClient.GetPodResourceMap(context.TODO(), pod, resourceMap)
				Expect(err).NotTo(HaveOccurred())

				// Regular claim path
				Expect(resourceMap).To(HaveKey(regularMapKey))
				Expect(resourceMap[regularMapKey].DeviceIDs).To(Equal([]string{regularDeviceID}))

				// Extended resource claim path
				Expect(resourceMap).To(HaveKey(extMapKey1))
				Expect(resourceMap[extMapKey1].DeviceIDs).To(Equal([]string{extDeviceID1}))
				Expect(resourceMap).To(HaveKey(extMapKey2))
				Expect(resourceMap[extMapKey2].DeviceIDs).To(Equal([]string{extDeviceID2}))

				Expect(resourceMap).To(HaveLen(3))
			})
		})
	})
})
