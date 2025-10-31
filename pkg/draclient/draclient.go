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

type ClientInterace interface {
	GetPodResourceMap(pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error
}

type draClient struct {
	client             resourcev1.ResourceV1Interface
	resourceSliceCache map[string]*resourcev1api.ResourceSlice
	resourceClaimCache map[string]*resourcev1api.ResourceClaim
}

func NewClient(client resourcev1.ResourceV1Interface) ClientInterace {
	logging.Debugf("NewClient: creating new DRA client")
	return &draClient{
		client:             client,
		resourceSliceCache: make(map[string]*resourcev1api.ResourceSlice),
		resourceClaimCache: make(map[string]*resourcev1api.ResourceClaim),
	}
}

func (d *draClient) GetPodResourceMap(pod *v1.Pod, resourceMap map[string]*types.ResourceInfo) error {
	var err error
	logging.Verbosef("GetPodResourceMap: processing DRA resources for pod %s/%s", pod.Namespace, pod.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, claimResource := range pod.Status.ResourceClaimStatuses { // (resourceClaimName/RequestedDevice)
		claimName := claimResource.Name
		logging.Debugf("GetPodResourceMap: processing resource claim: %s", claimName)

		// get resource claim
		resourceClaim, ok := d.resourceClaimCache[claimName]
		if !ok {
			logging.Debugf("GetPodResourceMap: resource claim %s not in cache, fetching from API", claimName)
			resourceClaim, err = d.client.ResourceClaims(pod.Namespace).Get(ctx, *claimResource.ResourceClaimName, metav1.GetOptions{})
			if err != nil {
				logging.Errorf("GetPodResourceMap: failed to get resource claim %s: %v", claimName, err)
				return err
			}
			d.resourceClaimCache[claimName] = resourceClaim
			logging.Debugf("GetPodResourceMap: cached resource claim %s", claimName)
		} else {
			logging.Debugf("GetPodResourceMap: using cached resource claim %s", claimName)
		}

		for _, result := range resourceClaim.Status.Allocation.Devices.Results {
			logging.Debugf("GetPodResourceMap: processing device allocation - driver: %s, pool: %s, device: %s, request: %s",
				result.Driver, result.Pool, result.Device, result.Request)

			deviceID, err := d.getDeviceID(ctx, result)
			if err != nil {
				logging.Errorf("GetPodResourceMap: failed to get device ID for claim %s: %v", claimName, err)
				return err
			}

			resourceMapKey := fmt.Sprintf("%s/%s", claimName, result.Request)
			if rInfo, ok := resourceMap[resourceMapKey]; ok {
				rInfo.DeviceIDs = append(rInfo.DeviceIDs, deviceID)
				logging.Debugf("GetPodResourceMap: appended device ID %s to existing resource map entry %s", deviceID, resourceMapKey)
			} else {
				resourceMap[resourceMapKey] = &types.ResourceInfo{DeviceIDs: []string{deviceID}}
				logging.Debugf("GetPodResourceMap: created new resource map entry %s with device ID %s", resourceMapKey, deviceID)
			}
		}
		logging.Debugf("GetPodResourceMap: successfully processed resource claim %s", claimName)
	}

	logging.Verbosef("GetPodResourceMap: successfully processed all DRA resources for pod %s/%s, total resources: %d",
		pod.Namespace, pod.Name, len(resourceMap))
	return nil
}

func (d *draClient) getDeviceID(ctx context.Context, result resourcev1api.DeviceRequestAllocationResult) (string, error) {
	key := fmt.Sprintf("%s/%s", result.Driver, result.Pool)
	logging.Debugf("getDeviceID: looking up device ID for driver/pool: %s, device: %s", key, result.Device)

	resourceSlice, ok := d.resourceSliceCache[key]
	if !ok {
		logging.Debugf("getDeviceID: resource slice %s not in cache, fetching from API", key)
		// List all ResourceSlices - field selectors are not supported for spec.driver and spec.pool.name
		listOptions := metav1.ListOptions{}
		allResourceSlices, err := d.client.ResourceSlices().List(ctx, listOptions)
		if err != nil {
			logging.Errorf("getDeviceID: failed to list resource slices: %v", err)
			return "", err
		}

		for _, slice := range allResourceSlices.Items {
			if slice.Spec.Driver == result.Driver && slice.Spec.Pool.Name == result.Pool {
				resourceSlice = slice.DeepCopy()
				break
			}
		}

		if resourceSlice == nil {
			logging.Errorf("getDeviceID: expected 1 resource slice for %s, got 0: no resource slice found", key)
			return "", fmt.Errorf("expected 1 resource slice, got 0: no resource slice found")
		}
		d.resourceSliceCache[key] = resourceSlice
		logging.Debugf("getDeviceID: cached resource slice %s with %d devices", key, len(resourceSlice.Spec.Devices))
	} else {
		logging.Debugf("getDeviceID: using cached resource slice %s", key)
	}

	logging.Debugf("getDeviceID: searching for device %s in %d devices", result.Device, len(resourceSlice.Spec.Devices))
	for _, device := range resourceSlice.Spec.Devices {
		if device.Name != result.Device {
			continue
		}
		logging.Debugf("getDeviceID: found device %s, checking for deviceID attribute", device.Name)
		deviceID, exists := device.Attributes["k8s.cni.cncf.io/deviceID"]
		if !exists {
			logging.Debugf("getDeviceID: device %s does not have k8s.cni.cncf.io/deviceID attribute", device.Name)
			continue
		}
		logging.Verbosef("getDeviceID: successfully retrieved device ID %s for device %s (driver/pool: %s)",
			*deviceID.StringValue, result.Device, key)
		return *deviceID.StringValue, nil
	}

	err := fmt.Errorf("device %s not found for claim resource %s/%s", result.Device, result.Driver, result.Pool)
	logging.Errorf("getDeviceID: %v", err)
	return "", err
}
