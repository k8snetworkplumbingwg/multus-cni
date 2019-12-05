// Copyright (c) 2019 Kubernetes Network Plumbing Working Group
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

package utils

import (
	"encoding/json"
	"fmt"
	"github.com/containernetworking/cni/libcni"
	"strings"

	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

// GetCNIConfig (from annotation string to CNI JSON bytes)
func GetCNIConfig(net *v1.NetworkAttachmentDefinition, confDir string) (config []byte, err error) {
	emptySpec := v1.NetworkAttachmentDefinitionSpec{}
	if net.Spec == emptySpec {
		// Network Spec empty; generate delegate from CNI JSON config
		// from the configuration directory that has the same network
		// name as the custom resource
		config, err = GetCNIConfigFromFile(net.Name, confDir)
		if err != nil {
			return nil, fmt.Errorf("GetCNIConfig: err in GetCNIConfigFromFile: %v", err)
		}
	} else {
		// Config contains a standard JSON-encoded CNI configuration
		// or configuration list which defines the plugin chain to
		// execute.
		config, err = GetCNIConfigFromSpec(net.Spec.Config, net.Name)
		if err != nil {
			return nil, fmt.Errorf("GetCNIConfig: err in getCNIConfigFromSpec: %v", err)
		}
	}
	return config, nil
}

// GetCNIConfigFromSpec reads a CNI JSON configuration from given directory (confDir)
func GetCNIConfigFromFile(name, confDir string) ([]byte, error) {
	// In the absence of valid keys in a Spec, the runtime (or
	// meta-plugin) should load and execute a CNI .configlist
	// or .config (in that order) file on-disk whose JSON
	// "name" key matches this Network object’s name.

	// In part, adapted from K8s pkg/kubelet/dockershim/network/cni/cni.go#getDefaultCNINetwork
	files, err := libcni.ConfFiles(confDir, []string{".conf", ".json", ".conflist"})
	switch {
	case err != nil:
		return nil, fmt.Errorf("No networks found in %s", confDir)
	case len(files) == 0:
		return nil, fmt.Errorf("No networks found in %s", confDir)
	}

	for _, confFile := range files {
		var confList *libcni.NetworkConfigList
		if strings.HasSuffix(confFile, ".conflist") {
			confList, err = libcni.ConfListFromFile(confFile)
			if err != nil {
				return nil, fmt.Errorf("Error loading CNI conflist file %s: %v", confFile, err)
			}

			if confList.Name == name || name == "" {
				return confList.Bytes, nil
			}

		} else {
			conf, err := libcni.ConfFromFile(confFile)
			if err != nil {
				return nil, fmt.Errorf("Error loading CNI config file %s: %v", confFile, err)
			}

			if conf.Network.Name == name || name == "" {
				// Ensure the config has a "type" so we know what plugin to run.
				// Also catches the case where somebody put a conflist into a conf file.
				if conf.Network.Type == "" {
					return nil, fmt.Errorf("Error loading CNI config file %s: no 'type'; perhaps this is a .conflist?", confFile)
				}
				return conf.Bytes, nil
			}
		}
	}

	return nil, fmt.Errorf("no network available in the name %s in cni dir %s", name, confDir)
}

// GetCNIConfigFromSpec reads a CNI JSON configuration from the NetworkAttachmentDefinition
// object's Spec.Config field and fills in any missing details like the network name
func GetCNIConfigFromSpec(configData, netName string) ([]byte, error) {
	var rawConfig map[string]interface{}
	var err error

	configBytes := []byte(configData)
	err = json.Unmarshal(configBytes, &rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal Spec.Config: %v", err)
	}

	// Inject network name if missing from Config for the thick plugin case
	if n, ok := rawConfig["name"]; !ok || n == "" {
		rawConfig["name"] = netName
		configBytes, err = json.Marshal(rawConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to re-marshal Spec.Config: %v", err)
		}
	}

	return configBytes, nil
}
