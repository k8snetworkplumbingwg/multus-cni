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
	v1 "k8s.io/api/core/v1"
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

type checkpoint struct {
	fileName   string
	podEntires []PodDevicesEntry
}

// GetCheckpoint returns an instance of Checkpoint
func GetCheckpoint() (types.ResourceClient, error) {
	logging.Debugf("GetCheckpoint(): invoked")
	return getCheckpoint(checkPointfile)
}

func getCheckpoint(filePath string) (types.ResourceClient, error) {
	cp := &checkpoint{fileName: filePath}
	err := cp.getPodEntries()
	if err != nil {
		return nil, err
	}
	logging.Debugf("getCheckpoint(): created checkpoint instance with file: %s", filePath)
	return cp, nil
}

// getPodEntries gets all Pod device allocation entries from checkpoint file
func (cp *checkpoint) getPodEntries() error {

	cpd := &Data{}
	rawBytes, err := ioutil.ReadFile(cp.fileName)
	if err != nil {
		return logging.Errorf("getPodEntries(): error reading file %s\n%v\n", checkPointfile, err)
	}

	if err = json.Unmarshal(rawBytes, cpd); err != nil {
		return logging.Errorf("getPodEntries(): error unmarshalling raw bytes %v", err)
	}

	cp.podEntires = cpd.Data.PodDeviceEntries
	logging.Debugf("getPodEntries(): podEntires %+v", cp.podEntires)
	return nil
}

// GetComputeDeviceMap returns an instance of a map of ResourceInfo
func (cp *checkpoint) GetPodResourceMap(pod *v1.Pod) (map[string]*types.ResourceInfo, error) {
	podID := string(pod.UID)
	resourceMap := make(map[string]*types.ResourceInfo)

	if podID == "" {
		return nil, logging.Errorf("GetPodResourceMap(): invalid Pod cannot be empty")
	}

	for _, pod := range cp.podEntires {
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
