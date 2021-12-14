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
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver"
)

const (
	configListCapabilityKey   = "plugins"
	singleConfigCapabilityKey = "capabilities"
)

// Option mutates the `conf` object
type Option func(conf *MultusConf)

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
func NewMultusConfig(pluginName string, cniVersion string, kubeconfig string, configurationOptions ...Option) (*MultusConf, error) {
	multusConfig := &MultusConf{
		Name:         MultusDefaultNetworkName,
		CNIVersion:   cniVersion,
		Type:         pluginName,
		Capabilities: map[string]bool{},
		Kubeconfig:   kubeconfig,
		Delegates:    []interface{}{},
	}

	err := multusConfig.Mutate(configurationOptions...)
	return multusConfig, err
}

// CheckVersionCompatibility checks compatibilty of the
// top level cni version with the delegate cni version.
// Since version 0.4.0, CHECK was introduced, which
// causes incompatibility.
func CheckVersionCompatibility(mc *MultusConf) error {
	const versionFmt = "delegate cni version is %s while top level cni version is %s"
	v040, _ := semver.Make("0.4.0")
	multusCNIVersion, err := semver.Make(mc.CNIVersion)

	if err != nil {
		return errors.New("couldn't get top level cni version")
	}

	if multusCNIVersion.GTE(v040) {
		for _, delegate := range mc.Delegates {
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
	}

	return nil
}

// Generate generates the multus configuration from whatever state is currently
// held
func (mc *MultusConf) Generate() (string, error) {
	data, err := json.Marshal(mc)
	return string(data), err
}

// Mutate updates the MultusConf attributes according to the provided
// configuration `Option`s
func (mc *MultusConf) Mutate(configurationOptions ...Option) error {
	for _, configOption := range configurationOptions {
		configOption(mc)
	}

	return CheckVersionCompatibility(mc)
}

// WithNamespaceIsolation mutates the inner state to enable the
// NamespaceIsolation attribute
func WithNamespaceIsolation() Option {
	return func(conf *MultusConf) {
		conf.NamespaceIsolation = true
	}
}

// WithGlobalNamespaces mutates the inner state to set the
// RawNonIsolatedNamespaces attribute
func WithGlobalNamespaces(globalNamespaces string) Option {
	return func(conf *MultusConf) {
		conf.RawNonIsolatedNamespaces = globalNamespaces
	}
}

// WithLogToStdErr mutates the inner state to enable the
// WithLogToStdErr attribute
func WithLogToStdErr() Option {
	return func(conf *MultusConf) {
		conf.LogToStderr = true
	}
}

// WithLogLevel mutates the inner state to set the
// LogLevel attribute
func WithLogLevel(logLevel string) Option {
	return func(conf *MultusConf) {
		conf.LogLevel = logLevel
	}
}

// WithLogFile mutates the inner state to set the
// logFile attribute
func WithLogFile(logFile string) Option {
	return func(conf *MultusConf) {
		conf.LogFile = logFile
	}
}

// WithReadinessFileIndicator mutates the inner state to set the
// ReadinessIndicatorFile attribute
func WithReadinessFileIndicator(path string) Option {
	return func(conf *MultusConf) {
		conf.ReadinessIndicatorFile = path
	}
}

// WithAdditionalBinaryFileDir mutates the inner state to set the
// BinDir attribute
func WithAdditionalBinaryFileDir(directoryPath string) Option {
	return func(conf *MultusConf) {
		conf.BinDir = directoryPath
	}
}

// WithOverriddenName mutates the inner state to set the
// Name attribute
func WithOverriddenName(networkName string) Option {
	return func(conf *MultusConf) {
		conf.Name = networkName
	}
}

func withCapabilities(cniData interface{}) Option {
	var enabledCapabilities []string
	var pluginsList []interface{}
	cniDataMap, ok := cniData.(map[string]interface{})
	if ok {
		if pluginsListEntry, ok := cniDataMap[configListCapabilityKey]; ok {
			pluginsList = pluginsListEntry.([]interface{})
		}
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

	return func(conf *MultusConf) {
		for _, capability := range enabledCapabilities {
			conf.Capabilities[capability] = true
		}
	}
}

func withDelegates(primaryCNIConfigData map[string]interface{}) Option {
	return func(conf *MultusConf) {
		conf.Delegates = []interface{}{primaryCNIConfigData}
	}
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
