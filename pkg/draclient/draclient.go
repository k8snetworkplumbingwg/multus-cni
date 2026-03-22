// Copyright (c) 2026 Multus Authors
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

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	resourcev1api "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	resourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

const (
	multusDeviceIDAttr     = "k8s.cni.cncf.io/deviceID"
	multusResourceNameAttr = "k8s.cni.cncf.io/resourceName"
)

// namespacedClaimCacheKey avoids cache collisions: ResourceClaim is namespaced.
func namespacedClaimCacheKey(namespace, claimName string) string {
	return namespace + "/" + claimName
}

type deviceInfo struct {
	DeviceID     string
	ResourceName string
}

type ClientInterface interface {
	GetPodResourceMap(pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error
}

type draClient struct {
	client resourcev1.ResourceV1Interface
	// One driver/pool may span multiple ResourceSlice objects (e.g. per NUMA zone).
	resourceSliceCache map[string][]*resourcev1api.ResourceSlice
	// Keys are namespace/claimName (ResourceClaim is namespaced).
	resourceClaimCache map[string]*resourcev1api.ResourceClaim
}

func NewClient(client resourcev1.ResourceV1Interface) ClientInterface {
	logging.Debugf("NewClient: creating new DRA client")
	return &draClient{
		client:             client,
		resourceSliceCache: make(map[string][]*resourcev1api.ResourceSlice),
		resourceClaimCache: make(map[string]*resourcev1api.ResourceClaim),
	}
}

func (d *draClient) GetPodResourceMap(pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error {
	logging.Verbosef("GetPodResourceMap: processing DRA resources for pod %s/%s", pod.Namespace, pod.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, claimResource := range pod.Status.ResourceClaimStatuses {
		if claimResource.ResourceClaimName == nil {
			logging.Errorf("GetPodResourceMap: resource claim status has nil ResourceClaimName")
			continue
		}
		claimName := *claimResource.ResourceClaimName
		claimCacheKey := namespacedClaimCacheKey(pod.Namespace, claimName)
		logging.Debugf("GetPodResourceMap: processing resource claim: %s (ref name: %s)", claimName, claimResource.Name)

		resourceClaim, ok := d.resourceClaimCache[claimCacheKey]
		if !ok {
			logging.Debugf("GetPodResourceMap: resource claim %s/%s not in cache, fetching from API", pod.Namespace, claimName)
			fetched, err := d.client.ResourceClaims(pod.Namespace).Get(ctx, *claimResource.ResourceClaimName, metav1.GetOptions{})
			if err != nil {
				logging.Errorf("GetPodResourceMap: failed to get resource claim %s: %v", claimName, err)
				return err
			}
			resourceClaim = fetched
			d.resourceClaimCache[claimCacheKey] = resourceClaim
			logging.Debugf("GetPodResourceMap: cached resource claim %s", claimCacheKey)
		} else {
			logging.Debugf("GetPodResourceMap: using cached resource claim %s", claimCacheKey)
		}

		if resourceClaim.Status.Allocation == nil || resourceClaim.Status.Allocation.Devices.Results == nil {
			logging.Errorf("GetPodResourceMap: claim %s has no device allocation", claimName)
			return fmt.Errorf("claim %s has no device allocation", claimName)
		}

		for _, result := range resourceClaim.Status.Allocation.Devices.Results {
			logging.Debugf("GetPodResourceMap: processing device allocation - driver: %s, pool: %s, device: %s, request: %s",
				result.Driver, result.Pool, result.Device, result.Request)

			info, err := d.getDeviceInfo(ctx, result)
			if err != nil {
				logging.Errorf("GetPodResourceMap: failed to get device info for claim %s: %v", claimName, err)
				return err
			}

			if info.ResourceName == "" {
				resErr := fmt.Errorf("device %s missing required attribute %s (must match NAD k8s.v1.cni.cncf.io/resourceName)", result.Device, multusResourceNameAttr)
				logging.Errorf("GetPodResourceMap: %v", resErr)
				return resErr
			}

			resourceMapKey := info.ResourceName
			if rInfo, ok := resourceMap[resourceMapKey]; ok {
				rInfo.DeviceIDs = append(rInfo.DeviceIDs, info.DeviceID)
				logging.Debugf("GetPodResourceMap: appended device ID %s to existing resource map entry %s", info.DeviceID, resourceMapKey)
			} else {
				resourceMap[resourceMapKey] = &types.ResourceInfo{DeviceIDs: []string{info.DeviceID}}
				logging.Debugf("GetPodResourceMap: created new resource map entry %s with device ID %s", resourceMapKey, info.DeviceID)
			}
		}
		logging.Debugf("GetPodResourceMap: successfully processed resource claim %s", claimName)
	}

	if pod.Status.ExtendedResourceClaimStatus != nil {
		if err := d.processExtendedResourceClaimStatus(ctx, pod, resourceMap); err != nil {
			return err
		}
	}

	logging.Verbosef("GetPodResourceMap: successfully processed all DRA resources for pod %s/%s, total resources: %d",
		pod.Namespace, pod.Name, len(resourceMap))
	return nil
}

// processExtendedResourceClaimStatus fills the resource map for pods that use
// the extended resource feature gate (pod.Status.ExtendedResourceClaimStatus).
// Keys come from requestMappings[].resourceName (same as NAD annotation).
func (d *draClient) processExtendedResourceClaimStatus(ctx context.Context, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error {
	extStatus := pod.Status.ExtendedResourceClaimStatus
	claimName := extStatus.ResourceClaimName
	claimCacheKey := namespacedClaimCacheKey(pod.Namespace, claimName)
	logging.Debugf("GetPodResourceMap: processing extended resource claim: %s/%s", pod.Namespace, claimName)

	resourceClaim, ok := d.resourceClaimCache[claimCacheKey]
	if !ok {
		fetched, err := d.client.ResourceClaims(pod.Namespace).Get(ctx, claimName, metav1.GetOptions{})
		if err != nil {
			logging.Errorf("GetPodResourceMap: failed to get extended resource claim %s/%s: %v", pod.Namespace, claimName, err)
			return err
		}
		resourceClaim = fetched
		d.resourceClaimCache[claimCacheKey] = resourceClaim
		logging.Debugf("GetPodResourceMap: cached extended resource claim %s", claimCacheKey)
	}

	if resourceClaim.Status.Allocation == nil || resourceClaim.Status.Allocation.Devices.Results == nil {
		logging.Errorf("GetPodResourceMap: claim %s has no device allocation", claimName)
		return fmt.Errorf("claim %s has no device allocation", claimName)
	}

	resultsByRequest := make(map[string][]resourcev1api.DeviceRequestAllocationResult)
	for _, result := range resourceClaim.Status.Allocation.Devices.Results {
		resultsByRequest[result.Request] = append(resultsByRequest[result.Request], result)
	}

	for _, mapping := range extStatus.RequestMappings {
		results, ok := resultsByRequest[mapping.RequestName]
		if !ok || len(results) == 0 {
			logging.Errorf("GetPodResourceMap: extended resource request %s not found in claim %s", mapping.RequestName, claimName)
			return fmt.Errorf("request %s not found in claim %s", mapping.RequestName, claimName)
		}

		resourceMapKey := mapping.ResourceName
		for _, result := range results {
			info, err := d.getDeviceInfo(ctx, result)
			if err != nil {
				logging.Errorf("GetPodResourceMap: failed to get device info for extended resource claim %s request %s: %v", claimName, mapping.RequestName, err)
				return err
			}

			if info.ResourceName == "" {
				resErr := fmt.Errorf("device %s missing required attribute %s (must match NAD k8s.v1.cni.cncf.io/resourceName and extended mapping %q)",
					result.Device, multusResourceNameAttr, resourceMapKey)
				logging.Errorf("GetPodResourceMap: %v", resErr)
				return resErr
			}
			if info.ResourceName != mapping.ResourceName {
				resErr := fmt.Errorf("device %s: %s is %q but extended resource mapping for request %q is %q",
					result.Device, multusResourceNameAttr, info.ResourceName, mapping.RequestName, mapping.ResourceName)
				logging.Errorf("GetPodResourceMap: %v", resErr)
				return resErr
			}

			if rInfo, ok := resourceMap[resourceMapKey]; ok {
				rInfo.DeviceIDs = append(rInfo.DeviceIDs, info.DeviceID)
				logging.Debugf("GetPodResourceMap: appended device ID %s to extended resource map entry %s", info.DeviceID, resourceMapKey)
			} else {
				resourceMap[resourceMapKey] = &types.ResourceInfo{DeviceIDs: []string{info.DeviceID}}
				logging.Debugf("GetPodResourceMap: created new extended resource map entry %s with device ID %s", resourceMapKey, info.DeviceID)
			}
		}
	}

	logging.Debugf("GetPodResourceMap: successfully processed extended resource claim %s", claimName)
	return nil
}

func (d *draClient) getDeviceInfo(ctx context.Context, result resourcev1api.DeviceRequestAllocationResult) (*deviceInfo, error) {
	key := fmt.Sprintf("%s/%s", result.Driver, result.Pool)
	logging.Debugf("getDeviceInfo: looking up device for driver/pool: %s, device: %s", key, result.Device)

	resourceSlices, ok := d.resourceSliceCache[key]
	if !ok {
		logging.Debugf("getDeviceInfo: resource slices for %s not in cache, fetching from API", key)
		// TODO: Use server-side field selector once spec.driver is supported by the API.
		// Currently, ResourceSlice does not support field selection on spec.driver,
		// requiring client-side filtering which may impact performance in very large clusters.
		listOptions := metav1.ListOptions{}
		allResourceSlices, err := d.client.ResourceSlices().List(ctx, listOptions)
		if err != nil {
			logging.Errorf("getDeviceInfo: failed to list resource slices: %v", err)
			return nil, err
		}

		var matchingSlices []*resourcev1api.ResourceSlice
		for i := range allResourceSlices.Items {
			slice := &allResourceSlices.Items[i]
			if slice.Spec.Driver == result.Driver && slice.Spec.Pool.Name == result.Pool {
				matchingSlices = append(matchingSlices, slice)
			}
		}

		if len(matchingSlices) == 0 {
			listErr := fmt.Errorf("no resource slice found for driver/pool %s", key)
			logging.Errorf("getDeviceInfo: %v", listErr)
			return nil, listErr
		}
		resourceSlices = matchingSlices
		d.resourceSliceCache[key] = resourceSlices
		logging.Debugf("getDeviceInfo: cached %d resource slices for %s", len(resourceSlices), key)
	} else {
		logging.Debugf("getDeviceInfo: using cached %d resource slices for %s", len(resourceSlices), key)
	}

	for _, resourceSlice := range resourceSlices {
		logging.Debugf("getDeviceInfo: searching for device %s in slice %s with %d devices", result.Device, resourceSlice.Name, len(resourceSlice.Spec.Devices))
		for _, device := range resourceSlice.Spec.Devices {
			if device.Name != result.Device {
				continue
			}
			logging.Debugf("getDeviceInfo: found device %s, checking attributes", device.Name)

			devIDAttr, exists := device.Attributes[multusDeviceIDAttr]
			if !exists {
				logging.Warningf(
					"getDeviceInfo: allocated device %q (driver %q, pool %q) has no %q attribute; DRA drivers must publish this for Multus; skipping device",
					device.Name, result.Driver, result.Pool, multusDeviceIDAttr)
				continue
			}

			if devIDAttr.StringValue == nil {
				logging.Warningf(
					"getDeviceInfo: allocated device %q (driver %q, pool %q) has %q with nil StringValue; skipping device",
					device.Name, result.Driver, result.Pool, multusDeviceIDAttr)
				continue
			}
			info := &deviceInfo{DeviceID: *devIDAttr.StringValue}

			if resNameAttr, ok := device.Attributes[multusResourceNameAttr]; ok && resNameAttr.StringValue != nil {
				info.ResourceName = *resNameAttr.StringValue
				logging.Debugf("getDeviceInfo: device %s has %s %s", device.Name, multusResourceNameAttr, info.ResourceName)
			}

			logging.Verbosef("getDeviceInfo: successfully retrieved info for device %s (driver/pool: %s): deviceID=%s, resourceName=%s",
				result.Device, key, info.DeviceID, info.ResourceName)
			return info, nil
		}
	}

	notFoundErr := fmt.Errorf("device %s not found for claim resource %s/%s in any matching resource slice", result.Device, result.Driver, result.Pool)
	logging.Errorf("getDeviceInfo: %v", notFoundErr)
	return nil, notFoundErr
}
