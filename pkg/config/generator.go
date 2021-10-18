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

package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/koron/go-dproxy"
)

const (
	configListCapabilityKey   = "plugins"
	singleConfigCapabilityKey = "capabilities"
)

// MultusConf holds the multus configuration, and persists it to disk
type MultusConf struct {
	BinDir                   string          `json:"binDir,omitempty"`
	Capabilities             map[string]bool `json:"capabilities,omitempty"`
	CNIVersion               string          `json:"cniVersion"`
	Delegates                []interface{}   `json:"delegates"`
	LogFile                  string          `json:"logFile,omitempty"`
	LogLevel                 string          `json:"logLevel,omitempty"`
	LogToStderr              bool            `json:"logToStderr,omitempty"`
	Kubeconfig               string          `json:"kubeconfig"`
	Name                     string          `json:"name"`
	NamespaceIsolation       bool            `json:"namespaceIsolation,omitempty"`
	RawNonIsolatedNamespaces string          `json:"globalNamespaces,omitempty"`
	ReadinessIndicatorFile   string          `json:"readinessindicatorfile,omitempty"`
	Type                     string          `json:"type"`
}

// NewMultusConfig creates a basic configuration generator. It can be mutated
// via the `With...` methods.
func NewMultusConfig(pluginName string, cniVersion string, kubeconfig string) *MultusConf {
	return &MultusConf{
		Name:         MultusDefaultNetworkName,
		CNIVersion:   cniVersion,
		Type:         pluginName,
		Capabilities: map[string]bool{},
		Kubeconfig:   kubeconfig,
		Delegates:    []interface{}{},
	}
}

// Generate generates the multus configuration from whatever state is currently
// held
func (mc *MultusConf) Generate() (string, error) {
	data, err := json.Marshal(mc)
	return string(data), err
}

// WithNamespaceIsolation mutates the inner state to enable the
// NamespaceIsolation attribute
func (mc *MultusConf) WithNamespaceIsolation() {
	mc.NamespaceIsolation = true
}

// WithGlobalNamespaces mutates the inner state to set the
// RawNonIsolatedNamespaces attribute
func (mc *MultusConf) WithGlobalNamespaces(globalNamespaces string) {
	mc.RawNonIsolatedNamespaces = globalNamespaces
}

// WithLogToStdErr mutates the inner state to enable the
// WithLogToStdErr attribute
func (mc *MultusConf) WithLogToStdErr() {
	mc.LogToStderr = true
}

// WithLogLevel mutates the inner state to set the
// LogLevel attribute
func (mc *MultusConf) WithLogLevel(logLevel string) {
	mc.LogLevel = logLevel
}

// WithLogFile mutates the inner state to set the
// logFile attribute
func (mc *MultusConf) WithLogFile(logFile string) {
	mc.LogFile = logFile
}

// WithReadinessFileIndicator mutates the inner state to set the
// ReadinessIndicatorFile attribute
func (mc *MultusConf) WithReadinessFileIndicator(path string) {
	mc.ReadinessIndicatorFile = path
}

// WithAdditionalBinaryFileDir mutates the inner state to set the
// BinDir attribute
func (mc *MultusConf) WithAdditionalBinaryFileDir(directoryPath string) {
	mc.BinDir = directoryPath
}

// WithOverriddenName mutates the inner state to set the
// Name attribute
func (mc *MultusConf) WithOverriddenName(networkName string) {
	mc.Name = networkName
}

func (mc *MultusConf) withCapabilities(cniData dproxy.Proxy) {
	var enabledCapabilities []string
	pluginsList, err := cniData.M(configListCapabilityKey).Array()
	if err != nil {
		pluginsList = nil
	}
	if len(pluginsList) > 0 {
		for _, pluginData := range pluginsList {
			enabledCapabilities = append(
				enabledCapabilities,
				extractCapabilities(dproxy.New(pluginData))...)
		}
	} else {
		enabledCapabilities = extractCapabilities(cniData)
	}

	for _, capability := range enabledCapabilities {
		mc.Capabilities[capability] = true
	}
}

func (mc *MultusConf) withDelegates(primaryCNIConfigData interface{}) {
	mc.Delegates = []interface{}{primaryCNIConfigData}
}

func extractCapabilities(capabilitiesProxy dproxy.Proxy) []string {
	capabilities, err := capabilitiesProxy.M(singleConfigCapabilityKey).Map()
	if err != nil {
		return nil
	}

	var enabledCapabilities []string
	if len(capabilities) > 0 {
		for capName, isCapabilityEnabled := range capabilities {
			if isCapabilityEnabled.(bool) {
				enabledCapabilities = append(enabledCapabilities, capName)
			}
		}
	}
	return enabledCapabilities
}

func findMasterPlugin(cniConfigDirPath string, remainingTries int) (string, error) {
	if remainingTries == 0 {
		return "", fmt.Errorf("could not find a plugin configuration in %s", cniConfigDirPath)
	}
	var cniPluginConfigs []string
	files, err := ioutil.ReadDir(cniConfigDirPath)
	if err != nil {
		return "", fmt.Errorf("error when listing the CNI plugin configurations: %w", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "00-multus") {
			continue
		}
		fileExtension := filepath.Ext(file.Name())
		if fileExtension == ".conf" || fileExtension == ".conflist" {
			cniPluginConfigs = append(cniPluginConfigs, file.Name())
		}
	}

	if len(cniPluginConfigs) == 0 {
		time.Sleep(time.Second)
		return findMasterPlugin(cniConfigDirPath, remainingTries-1)
	}
	sort.Strings(cniPluginConfigs)
	return cniPluginConfigs[0], nil
}
