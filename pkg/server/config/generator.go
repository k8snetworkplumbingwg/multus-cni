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
)

const (
	configListCapabilityKey   = "plugins"
	singleConfigCapabilityKey = "capabilities"
)

// LogOptionFunc mutates the `LoggingOptions` object
type LogOptionFunc func(logOptions *LogOptions)

// Option mutates the `conf` object
type Option func(conf *MultusConf) error

// MultusConf holds the multus configuration, and persists it to disk
type MultusConf struct {
	BinDir                   string          `json:"binDir,omitempty"`
	Capabilities             map[string]bool `json:"capabilities,omitempty"`
	CNIVersion               string          `json:"cniVersion"`
	LogFile                  string          `json:"logFile,omitempty"`
	LogLevel                 string          `json:"logLevel,omitempty"`
	LogToStderr              bool            `json:"logToStderr,omitempty"`
	LogOptions               *LogOptions     `json:"logOptions,omitempty"`
	Name                     string          `json:"name"`
	ClusterNetwork           string          `json:"clusterNetwork,omitempty"`
	NamespaceIsolation       bool            `json:"namespaceIsolation,omitempty"`
	RawNonIsolatedNamespaces string          `json:"globalNamespaces,omitempty"`
	ReadinessIndicatorFile   string          `json:"readinessindicatorfile,omitempty"`
	Type                     string          `json:"type"`
	CniDir                   string          `json:"cniDir,omitempty"`
	CniConfigDir             string          `json:"cniConfigDir,omitempty"`
	SocketDir                string          `json:"socketDir,omitempty"`
}

// LogOptions specifies the configuration of the log
type LogOptions struct {
	MaxAge     *int  `json:"maxAge,omitempty"`
	MaxSize    *int  `json:"maxSize,omitempty"`
	MaxBackups *int  `json:"maxBackups,omitempty"`
	Compress   *bool `json:"compress,omitempty"`
}

// NewMultusConfig creates a basic configuration generator. It can be mutated
// via the `With...` methods.
func NewMultusConfig(pluginName string, cniVersion string, configurationOptions ...Option) (*MultusConf, error) {
	multusConfig := &MultusConf{
		Name:         MultusDefaultNetworkName,
		CNIVersion:   cniVersion,
		Type:         pluginName,
		Capabilities: map[string]bool{},
	}

	err := multusConfig.Mutate(configurationOptions...)
	return multusConfig, err
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
	data, err := json.Marshal(mc)
	return string(data), err
}

// Mutate updates the MultusConf attributes according to the provided
// configuration `Option`s
func (mc *MultusConf) Mutate(configurationOptions ...Option) error {
	for _, configOption := range configurationOptions {
		err := configOption(mc)
		if err != nil {
			return err
		}
	}

	return nil
}

// WithNamespaceIsolation mutates the inner state to enable the
// NamespaceIsolation attribute
func WithNamespaceIsolation() Option {
	return func(conf *MultusConf) error {
		conf.NamespaceIsolation = true
		return nil
	}
}

// WithGlobalNamespaces mutates the inner state to set the
// RawNonIsolatedNamespaces attribute
func WithGlobalNamespaces(globalNamespaces string) Option {
	return func(conf *MultusConf) error {
		conf.RawNonIsolatedNamespaces = globalNamespaces
		return nil
	}
}

// WithLogToStdErr mutates the inner state to enable the
// WithLogToStdErr attribute
func WithLogToStdErr() Option {
	return func(conf *MultusConf) error {
		conf.LogToStderr = true
		return nil
	}
}

// WithLogLevel mutates the inner state to set the
// LogLevel attribute
func WithLogLevel(logLevel string) Option {
	return func(conf *MultusConf) error {
		conf.LogLevel = logLevel
		return nil
	}
}

// WithLogFile mutates the inner state to set the
// logFile attribute
func WithLogFile(logFile string) Option {
	return func(conf *MultusConf) error {
		conf.LogFile = logFile
		return nil
	}
}

// WithLogOptions mutates the inner state to set the
// LogOptions attribute
func WithLogOptions(logOptions *LogOptions) Option {
	return func(conf *MultusConf) error {
		conf.LogOptions = logOptions
		return nil
	}
}

// WithReadinessFileIndicator mutates the inner state to set the
// ReadinessIndicatorFile attribute
func WithReadinessFileIndicator(path string) Option {
	return func(conf *MultusConf) error {
		conf.ReadinessIndicatorFile = path
		return nil
	}
}

// WithAdditionalBinaryFileDir mutates the inner state to set the
// BinDir attribute
func WithAdditionalBinaryFileDir(directoryPath string) Option {
	return func(conf *MultusConf) error {
		conf.BinDir = directoryPath
		return nil
	}
}

// WithOverriddenName mutates the inner state to set the
// Name attribute
func WithOverriddenName(networkName string) Option {
	return func(conf *MultusConf) error {
		conf.Name = networkName
		return nil
	}
}

// WithCniDir mutates the inner state to set the
// multus CNI cache directory
func WithCniDir(cniDir string) Option {
	return func(conf *MultusConf) error {
		conf.CniDir = cniDir
		return nil
	}
}

// WithCniConfigDir mutates the inner state to set the
// multus CNI configuration directory
func WithCniConfigDir(confDir string) Option {
	return func(conf *MultusConf) error {
		conf.CniConfigDir = confDir
		return nil
	}
}

// WithSocketDir mutates the socket directory
func WithSocketDir(sockDir string) Option {
	return func(conf *MultusConf) error {
		conf.SocketDir = sockDir
		return nil
	}
}

func withClusterNetwork(clusterNetwork string) Option {
	return func(conf *MultusConf) error {
		conf.ClusterNetwork = clusterNetwork
		return nil
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
	} else {
		return func(conf *MultusConf) error {
			return errors.New("couldn't get cni config from delegate")
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

	return func(conf *MultusConf) error {
		for _, capability := range enabledCapabilities {
			conf.Capabilities[capability] = true
		}
		return nil
	}
}

// MutateLogOptions update the LoggingOptions of the MultusConf according
// to the provided configuration `loggingOptions`
func MutateLogOptions(logOption *LogOptions, logOptionFunc ...LogOptionFunc) {
	for _, loggingOption := range logOptionFunc {
		loggingOption(logOption)
	}
}

// WithLogMaxSize mutates the inner state to set the
// logMaxSize attribute
func WithLogMaxSize(maxSize *int) LogOptionFunc {
	return func(logOptions *LogOptions) {
		logOptions.MaxSize = maxSize
	}
}

// WithLogMaxAge mutates the inner state to set the
// logMaxAge attribute
func WithLogMaxAge(maxAge *int) LogOptionFunc {
	return func(logOptions *LogOptions) {
		logOptions.MaxAge = maxAge
	}
}

// WithLogMaxBackups mutates the inner state to set the
// logMaxBackups attribute
func WithLogMaxBackups(maxBackups *int) LogOptionFunc {
	return func(logOptions *LogOptions) {
		logOptions.MaxBackups = maxBackups
	}
}

// WithLogCompress mutates the inner state to set the
// logCompress attribute
func WithLogCompress(compress *bool) LogOptionFunc {
	return func(logOptions *LogOptions) {
		logOptions.Compress = compress
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
