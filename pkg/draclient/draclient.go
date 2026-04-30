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
	"errors"
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

// errDeviceNotInAnySlice is returned when allocation names a device that does not appear
// in any ResourceSlice for that driver/pool (wrapped in getDeviceInfo). Callers may skip
// individual results so multi-device claims (e.g. SR-IOV + GPU) still succeed for CNI.
var errDeviceNotInAnySlice = errors.New("device not in any matching resource slice")

// namespacedClaimCacheKey avoids cache collisions: ResourceClaim is namespaced.
func namespacedClaimCacheKey(namespace, claimName string) string {
	return namespace + "/" + claimName
}

type deviceInfo struct {
	DeviceID     string
	ResourceName string
}

// deviceInfoCacheKey uniquely identifies a device within a driver/pool combination.
type deviceInfoCacheKey struct {
	driverPool string // "driver/pool"
	deviceName string
}

type ClientInterface interface {
	GetPodResourceMap(ctx context.Context, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error
}

type draClient struct {
	client resourcev1.ResourceV1Interface
	// deviceInfoCache stores lightweight device attributes extracted from ResourceSlices.
	// Keys are (driver/pool, deviceName); only the two attributes Multus reads are kept,
	// so full ResourceSlice objects (~400KB each) are GC'd immediately after listing.
	deviceInfoCache map[deviceInfoCacheKey]*deviceInfo
	// populatedDrivers tracks which "nodeName/driverName" combinations have already been
	// fetched from the API, preventing redundant List calls within a client's lifetime.
	populatedDrivers map[string]bool
	// Keys are namespace/claimName (ResourceClaim is namespaced).
	resourceClaimCache map[string]*resourcev1api.ResourceClaim
}

func NewClient(client resourcev1.ResourceV1Interface) ClientInterface {
	logging.Debugf("NewClient: creating new DRA client")
	return &draClient{
		client:             client,
		deviceInfoCache:    make(map[deviceInfoCacheKey]*deviceInfo),
		populatedDrivers:   make(map[string]bool),
		resourceClaimCache: make(map[string]*resourcev1api.ResourceClaim),
	}
}

func (d *draClient) GetPodResourceMap(ctx context.Context, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error {
	logging.Verbosef("GetPodResourceMap: processing DRA resources for pod %s/%s", pod.Namespace, pod.Name)

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	nodeName := pod.Spec.NodeName

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

		results := resourceClaim.Status.Allocation.Devices.Results
		resolvedCount := 0
		for _, result := range results {
			logging.Debugf("GetPodResourceMap: processing device allocation - driver: %s, pool: %s, device: %s, request: %s",
				result.Driver, result.Pool, result.Device, result.Request)

			info, err := d.getDeviceInfo(ctx, nodeName, result)
			if err != nil {
				if errors.Is(err, errDeviceNotInAnySlice) {
					logging.Warningf(
						"GetPodResourceMap: skipping allocation result for claim %s (driver=%s pool=%s device=%s): %v",
						claimName, result.Driver, result.Pool, result.Device, err)
					continue
				}
				logging.Errorf("GetPodResourceMap: failed to get device info for claim %s: %v", claimName, err)
				return err
			}

			if info.ResourceName == "" {
				logging.Warningf(
					"GetPodResourceMap: skipping allocation result for claim %s (driver=%s pool=%s device=%s): no %q (only devices published for CNI are mapped)",
					claimName, result.Driver, result.Pool, result.Device, multusResourceNameAttr)
				continue
			}

			resourceMapKey := info.ResourceName
			if rInfo, ok := resourceMap[resourceMapKey]; ok {
				rInfo.DeviceIDs = append(rInfo.DeviceIDs, info.DeviceID)
				logging.Debugf("GetPodResourceMap: appended device ID %s to existing resource map entry %s", info.DeviceID, resourceMapKey)
			} else {
				resourceMap[resourceMapKey] = &types.ResourceInfo{DeviceIDs: []string{info.DeviceID}}
				logging.Debugf("GetPodResourceMap: created new resource map entry %s with device ID %s", resourceMapKey, info.DeviceID)
			}
			resolvedCount++
		}
		if resolvedCount == 0 && len(results) > 0 {
			logging.Warningf(
				"GetPodResourceMap: claim %s had no allocation results mapped for Multus (skipping this claim; existing kubelet/device-plugin map entries are kept). "+
					"Fix DRA ResourceSlices or Multus attributes if this claim should contribute to CNI.",
				claimName)
			continue
		}
		logging.Debugf("GetPodResourceMap: successfully processed resource claim %s", claimName)
	}

	if pod.Status.ExtendedResourceClaimStatus != nil {
		if err := d.processExtendedResourceClaimStatus(ctx, nodeName, pod, resourceMap); err != nil {
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
func (d *draClient) processExtendedResourceClaimStatus(ctx context.Context, nodeName string, pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error {
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
			info, err := d.getDeviceInfo(ctx, nodeName, result)
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

// ensureDriverCachePopulated lists ResourceSlices for the given node and driver (using server-side
// field selectors) and extracts only the two attributes Multus needs into deviceInfoCache.
// Full ResourceSlice objects are discarded after extraction, keeping memory usage minimal.
// Subsequent calls for the same node/driver combination are no-ops.
func (d *draClient) ensureDriverCachePopulated(ctx context.Context, nodeName, driverName string) error {
	populatedKey := nodeName + "/" + driverName
	if d.populatedDrivers[populatedKey] {
		return nil
	}

	listOptions := metav1.ListOptions{}
	if nodeName != "" && driverName != "" {
		listOptions.FieldSelector = fmt.Sprintf("spec.nodeName=%s,spec.driver=%s", nodeName, driverName)
	} else if nodeName != "" {
		listOptions.FieldSelector = fmt.Sprintf("spec.nodeName=%s", nodeName)
	}

	logging.Debugf("ensureDriverCachePopulated: listing ResourceSlices (fieldSelector=%q)", listOptions.FieldSelector)
	slices, err := d.client.ResourceSlices().List(ctx, listOptions)
	if err != nil {
		logging.Errorf("ensureDriverCachePopulated: failed to list resource slices: %v", err)
		return err
	}
	logging.Debugf("ensureDriverCachePopulated: listed %d ResourceSlice(s) for node=%q driver=%q", len(slices.Items), nodeName, driverName)

	for i := range slices.Items {
		slice := &slices.Items[i]
		driverPool := fmt.Sprintf("%s/%s", slice.Spec.Driver, slice.Spec.Pool.Name)
		for _, device := range slice.Spec.Devices {
			key := deviceInfoCacheKey{driverPool: driverPool, deviceName: device.Name}
			info := &deviceInfo{}
			if attr, ok := device.Attributes[multusDeviceIDAttr]; ok && attr.StringValue != nil {
				info.DeviceID = *attr.StringValue
			}
			if attr, ok := device.Attributes[multusResourceNameAttr]; ok && attr.StringValue != nil {
				info.ResourceName = *attr.StringValue
			}
			if info.DeviceID != "" {
				d.deviceInfoCache[key] = info
			}
		}
	}

	d.populatedDrivers[populatedKey] = true
	return nil
}

func (d *draClient) getDeviceInfo(ctx context.Context, nodeName string, result resourcev1api.DeviceRequestAllocationResult) (*deviceInfo, error) {
	driverPool := fmt.Sprintf("%s/%s", result.Driver, result.Pool)
	logging.Debugf("getDeviceInfo: looking up device for driver/pool: %s, device: %s", driverPool, result.Device)

	if err := d.ensureDriverCachePopulated(ctx, nodeName, result.Driver); err != nil {
		return nil, err
	}

	key := deviceInfoCacheKey{driverPool: driverPool, deviceName: result.Device}
	info, ok := d.deviceInfoCache[key]
	if !ok {
		notFoundErr := fmt.Errorf("%w: device %s not found for claim resource %s/%s in any matching resource slice",
			errDeviceNotInAnySlice, result.Device, result.Driver, result.Pool)
		logging.Errorf("getDeviceInfo: %v", notFoundErr)
		return nil, notFoundErr
	}

	if info.DeviceID == "" {
		logging.Warningf(
			"getDeviceInfo: device %q (driver %q, pool %q) has no %q in ResourceSlice; skipping allocation result",
			result.Device, result.Driver, result.Pool, multusDeviceIDAttr)
		return nil, fmt.Errorf("%w: device %q present in slice but missing %q",
			errDeviceNotInAnySlice, result.Device, multusDeviceIDAttr)
	}

	logging.Verbosef("getDeviceInfo: successfully retrieved info for device %s (driver/pool: %s): deviceID=%s, resourceName=%s",
		result.Device, driverPool, info.DeviceID, info.ResourceName)
	return info, nil
}
