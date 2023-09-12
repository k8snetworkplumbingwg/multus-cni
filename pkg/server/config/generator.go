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

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
)

const (
	configListCapabilityKey   = "plugins"
	multusPluginName          = "multus-shim"
	singleConfigCapabilityKey = "capabilities"
)

// Option mutates the `conf` object
type Option func(conf *MultusConf) error

// MultusConf holds the multus configuration
type MultusConf struct {
	BinDir                   string              `json:"binDir,omitempty"`
	Capabilities             map[string]bool     `json:"capabilities,omitempty"`
	CNIVersion               string              `json:"cniVersion"`
	LogFile                  string              `json:"logFile,omitempty"`
	LogLevel                 string              `json:"logLevel,omitempty"`
	LogToStderr              bool                `json:"logToStderr,omitempty"`
	LogOptions               *logging.LogOptions `json:"logOptions,omitempty"`
	Name                     string              `json:"name"`
	ClusterNetwork           string              `json:"clusterNetwork,omitempty"`
	NamespaceIsolation       bool                `json:"namespaceIsolation,omitempty"`
	RawNonIsolatedNamespaces string              `json:"globalNamespaces,omitempty"`
	ReadinessIndicatorFile   string              `json:"readinessindicatorfile,omitempty"`
	Type                     string              `json:"type"`
	CniDir                   string              `json:"cniDir,omitempty"`
	CniConfigDir             string              `json:"cniConfigDir,omitempty"`
	DaemonSocketDir          string              `json:"daemonSocketDir,omitempty"`
	MultusConfigFile         string              `json:"multusConfigFile,omitempty"`
	MultusMasterCni          string              `json:"multusMasterCNI,omitempty"`
	MultusAutoconfigDir      string              `json:"multusAutoconfigDir,omitempty"`
	ForceCNIVersion          bool                `json:"forceCNIVersion,omitempty"`
	OverrideNetworkName      bool                `json:"overrideNetworkName,omitempty"`
}

// ParseMultusConfig parses multus config from configPath and create MultusConf.
func ParseMultusConfig(configPath string) (*MultusConf, error) {
	config, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ParseMultusConfig failed to read the config file's contents: %w", err)
	}

	multusconf := MultusConf{
		MultusConfigFile: "auto",
		Type:             multusPluginName,
		Capabilities:     map[string]bool{},
		CniConfigDir:     "/etc/cni/net.d",
	}

	if err := json.Unmarshal(config, &multusconf); err != nil {
		return nil, fmt.Errorf("failed to unmarshall the daemon configuration: %w", err)
	}
	multusconf.Name = MultusDefaultNetworkName // change name

	return &multusconf, nil
}

// CheckVersionCompatibility checks compatibilty of the
// top level cni version with the delegate cni version.
// Since version 0.4.0, CHECK was introduced, which
// causes incompatibility.
func CheckVersionCompatibility(mc *MultusConf, delegate interface{}) error {
	const versionFmt = "delegate cni version is %s while top level cni version is %s"
	v040, _ := semver.Make("0.4.0")
	multusCNIVersion, err := semver.Make(mc.CNIVersion)

	if err != nil {
		return errors.New("couldn't get top level cni version")
	}

	if multusCNIVersion.GTE(v040) {
		delegatesMap, ok := delegate.(map[string]interface{})
		if !ok {
			return errors.New("couldn't get cni version of delegate")
		}
		delegateVersion, ok := delegatesMap["cniVersion"].(string)
		if !ok {
			return errors.New("couldn't get cni version of delegate")
		}
		v, err := semver.Make(delegateVersion)
		if err != nil {
			return err
		}
		if v.LT(v040) {
			return fmt.Errorf(versionFmt, delegateVersion, mc.CNIVersion)
		}
	}

	return nil
}

// Generate generates the multus configuration from whatever state is currently
// held
func (mc *MultusConf) Generate() (string, error) {
	// before marshal, flush variables which is not required for multus-shim config
	mc.CniConfigDir = ""
	mc.MultusConfigFile = ""
	mc.MultusAutoconfigDir = ""
	mc.MultusMasterCni = ""
	mc.ForceCNIVersion = false

	data, err := json.Marshal(mc)
	return string(data), err
}

func (mc *MultusConf) setCapabilities(cniData interface{}) error {
	var enabledCapabilities []string
	var pluginsList []interface{}
	cniDataMap, ok := cniData.(map[string]interface{})
	if ok {
		if pluginsListEntry, ok := cniDataMap[configListCapabilityKey]; ok {
			pluginsList = pluginsListEntry.([]interface{})
		}
	} else {
		return errors.New("couldn't get cni config from delegate")
	}

	if len(pluginsList) > 0 {
		for _, pluginData := range pluginsList {
			enabledCapabilities = append(
				enabledCapabilities,
				extractCapabilities(pluginData)...)
		}
	} else {
		enabledCapabilities = extractCapabilities(cniData)
	}

	for _, capability := range enabledCapabilities {
		mc.Capabilities[capability] = true
	}
	return nil
}

func extractCapabilities(capabilitiesInterface interface{}) []string {
	capabilitiesMap, ok := capabilitiesInterface.(map[string]interface{})
	if !ok {
		return nil
	}
	capabilitiesMapEntry, ok := capabilitiesMap[singleConfigCapabilityKey]
	if !ok {
		return nil
	}
	capabilities, ok := capabilitiesMapEntry.(map[string]interface{})
	if !ok {
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
	files, err := os.ReadDir(cniConfigDirPath)
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
