package checkpoint

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/intel/multus-cni/types"
)

const (
	checkPointfile = "/var/lib/kubelet/device-plugins/kubelet_internal_checkpoint"
)

type PodDevicesEntry struct {
	PodUID        string
	ContainerName string
	ResourceName  string
	DeviceIDs     []string
	AllocResp     []byte
}

type checkpointData struct {
	PodDeviceEntries  []PodDevicesEntry
	RegisteredDevices map[string][]string
}

type Data struct {
	Data     checkpointData
	Checksum uint64
}

// getPodEntries gets all Pod device allocation entries from checkpoint file
func getPodEntries() ([]PodDevicesEntry, error) {

	podEntries := []PodDevicesEntry{}

	cpd := &Data{}
	rawBytes, err := ioutil.ReadFile(checkPointfile)
	if err != nil {
		return podEntries, fmt.Errorf("getPodEntries(): error reading file %s\n%v\n", checkPointfile, err)

	}

	if err = json.Unmarshal(rawBytes, cpd); err != nil {
		return podEntries, fmt.Errorf("getPodEntries(): error unmarshalling raw bytes %v", err)
	}

	return cpd.Data.PodDeviceEntries, nil
}

// GetComputeDeviceMap returns a map of resourceName to list of device IDs
func GetComputeDeviceMap(podID string) (map[string]*types.ResourceInfo, error) {

	resourceMap := make(map[string]*types.ResourceInfo)
	podEntires, err := getPodEntries()

	if err != nil {
		return nil, err
	}

	for _, pod := range podEntires {
		if pod.PodUID == podID {
			entry, ok := resourceMap[pod.ResourceName]
			if ok {
				// already exists; append to it
				entry.DeviceIDs = append(entry.DeviceIDs, pod.DeviceIDs...)
			} else {
				// new entry
				resourceMap[pod.ResourceName] = &types.ResourceInfo{DeviceIDs: pod.DeviceIDs}
			}
		}
	}

	return resourceMap, nil
}
