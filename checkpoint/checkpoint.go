// Copyright (c) 2018 Intel Corporation
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

package checkpoint

import (
	"encoding/json"
	"io/ioutil"

	"github.com/intel/multus-cni/logging"
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
		return podEntries, logging.Errorf("getPodEntries(): error reading file %s\n%v\n", checkPointfile, err)

	}

	if err = json.Unmarshal(rawBytes, cpd); err != nil {
		return podEntries, logging.Errorf("getPodEntries(): error unmarshalling raw bytes %v", err)
	}

	return cpd.Data.PodDeviceEntries, nil
}

var instance map[string]*types.ResourceInfo

// GetComputeDeviceMap returns an instance of a map of ResourceInfo
func GetComputeDeviceMap(podID string) (map[string]*types.ResourceInfo, error) {

	if instance == nil {
		if resourceMap, err := getResourceMapFromFile(podID); err == nil {
			logging.Debugf("GetComputeDeviceMap(): created new instance of resourceMap for Pod: %s", podID)
			instance = resourceMap
		} else {
			logging.Errorf("GetComputeDeviceMap(): error creating resourceMap instance %v", err)
			return nil, err
		}
	}
	logging.Debugf("GetComputeDeviceMap(): resourceMap instance: %+v", instance)
	return instance, nil
}

func getResourceMapFromFile(podID string) (map[string]*types.ResourceInfo, error) {
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
