// Copyright (c) 2021 Multus Authors
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

package main

import (
	"encoding/json"
)

// MultusConf is struct to hold the configuration of the delegating CNI plugin
type MultusConf struct {
	Name            string                   `json:"name"`
	Type            string                   `json:"type"`
	CniVersion      string                   `json:"cniVersion"`
	Capabilities    map[string]bool          `json:"capabilities,omitempty"`
	ConfDir         string                   `json:"confDir,omitempty"`
	CNIDir          string                   `json:"cniDir,omitempty"`
	BinDir          string                   `json:"binDir,omitempty"`
	Kubeconfig      string                   `json:"kubeconfig"`
	ClusterNetwork  string                   `json:"clusterNetwork,omitempty"`
	DefaultNetworks []string                 `json:"defaultNetworks,omitempty"`
	Delegates       []map[string]interface{} `json:"delegates"`
	LogFile         string                   `json:"logFile,omitempty"`
	LogLevel        string                   `json:"logLevel,omitempty"`
	LogToStderr     bool                     `json:"logToStderr,omitempty"`

	// Default network readiness options
	ReadinessIndicatorFile string `json:"readinessindicatorfile,omitempty"`
	// Option to isolate the usage of CR's to the namespace in which a pod resides.
	NamespaceIsolation       bool   `json:"namespaceIsolation,omitempty"`
	RawNonIsolatedNamespaces string `json:"globalNamespaces,omitempty"`
}

func newMultusConfig(name string, pluginName string, cniVersion string, kubeconfig string, primaryCNIPluginConfig map[string]interface{}) *MultusConf {
	multusConfig := &MultusConf{
		Name:         name,
		Type:         pluginName,
		CniVersion:   cniVersion,
		Kubeconfig:   kubeconfig,
		Delegates:    []map[string]interface{}{primaryCNIPluginConfig},
		Capabilities: map[string]bool{},
	}

	return multusConfig
}

func (mc *MultusConf) generate() (string, error) {
	data, err := json.Marshal(mc)
	return string(data), err
}

func (mc *MultusConf) withNamespaceIsolation() {
	mc.NamespaceIsolation = true
}

func (mc *MultusConf) withGlobalNamespaces(globalNamespaces string) {
	mc.RawNonIsolatedNamespaces = globalNamespaces
}

func (mc *MultusConf) withLogToStdErr() {
	mc.LogToStderr = true
}

func (mc *MultusConf) withLogLevel(logLevel string) {
	mc.LogLevel = logLevel
}

func (mc *MultusConf) withLogFile(logFile string) {
	mc.LogFile = logFile
}

func (mc *MultusConf) withReadinessFileIndicator(path string) {
	mc.ReadinessIndicatorFile = path
}

func (mc *MultusConf) withAdditionalBinaryFileDir(directoryPath string) {
	mc.BinDir = directoryPath
}

func (mc *MultusConf) withCapabilities(cniData primaryCNIConfigData) {
	var enabledCapabilities []string
	if len(cniData.Plugins) > 0 {
		for _, pluginData := range cniData.Plugins {
			for capName, isCapabilityEnabled := range pluginData.Capabilities {
				if isCapabilityEnabled {
					enabledCapabilities = append(enabledCapabilities, capName)
				}
			}
		}
	} else if len(cniData.Capabilities) > 0 {
		for capName, isCapabilityEnabled := range cniData.Capabilities {
			if isCapabilityEnabled {
				enabledCapabilities = append(enabledCapabilities, capName)
			}
		}
	}

	for _, capability := range enabledCapabilities {
		mc.Capabilities[capability] = true
	}
}
