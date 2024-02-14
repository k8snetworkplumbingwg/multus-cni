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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
)

// MultusDefaultNetworkName holds the default name of the multus network
const (
	multusConfigFileName     = "00-multus.conf"
	MultusDefaultNetworkName = "multus-cni-network"
	UserRWPermission         = 0600
)

// Manager monitors the configuration of the primary CNI plugin, and
// regenerates multus configuration whenever it gets updated.
type Manager struct {
	cniConfigData              map[string]interface{}
	configWatcher              *fsnotify.Watcher
	multusConfig               *MultusConf
	multusConfigDir            string
	multusConfigFilePath       string
	readinessIndicatorFilePath string
	primaryCNIConfigPath       string
}

// NewManager returns a config manager object, configured to read the
// primary CNI configuration in `config.MultusAutoconfigDir`. If
// `config.MultusMasterCni` is empty, this constructor will auto-discover the
// primary CNI for which it will delegate.
func NewManager(config MultusConf) (*Manager, error) {
	var err error
	defaultPluginName := config.MultusMasterCni
	if defaultPluginName == "" {
		defaultPluginName, err = getPrimaryCNIPluginName(config.MultusAutoconfigDir)
		if err != nil {
			_ = logging.Errorf("failed to find the primary CNI plugin: %v", err)
			return nil, err
		}
	}

	return newManager(config, defaultPluginName)
}

// overrideCNIVersion overrides cniVersion in cniConfigFile, it should be used only in kind case
func overrideCNIVersion(cniConfigFile string, multusCNIVersion string) error {
	path, err := filepath.Abs(cniConfigFile)
	if err != nil {
		return fmt.Errorf("illegal path %s in cni config path %s: %w", path, cniConfigFile, err)
	}

	masterCNIConfigData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read cni config %s: %v", path, err)
	}

	var primaryCNIConfigData map[string]interface{}
	if err := json.Unmarshal(masterCNIConfigData, &primaryCNIConfigData); err != nil {
		return fmt.Errorf("failed to unmarshall cni config %s: %w", cniConfigFile, err)
	}

	primaryCNIConfigData["cniVersion"] = multusCNIVersion
	configBytes, err := json.Marshal(primaryCNIConfigData)
	if err != nil {
		return fmt.Errorf("couldn't update cluster network config: %v", err)
	}

	err = os.WriteFile(path, configBytes, 0644)
	if err != nil {
		return fmt.Errorf("couldn't update cluster network config: %v", err)
	}
	return nil
}

func newManager(config MultusConf, defaultCNIPluginName string) (*Manager, error) {
	if config.ForceCNIVersion {
		err := overrideCNIVersion(filepath.Join(config.MultusAutoconfigDir, defaultCNIPluginName), config.CNIVersion)
		if err != nil {
			return nil, err
		}
	}

	readinessIndicatorPath := ""
	if config.ReadinessIndicatorFile != "" {
		readinessIndicatorPath = filepath.Dir(config.ReadinessIndicatorFile)
	}

	watcher, err := newWatcher(config.MultusAutoconfigDir, readinessIndicatorPath)
	if err != nil {
		return nil, err
	}

	if defaultCNIPluginName == fmt.Sprintf("%s/%s", config.MultusAutoconfigDir, multusConfigFileName) {
		return nil, logging.Errorf("cannot specify %s/%s to prevent recursive config load", config.MultusAutoconfigDir, multusConfigFileName)
	}

	configManager := &Manager{
		configWatcher:              watcher,
		multusConfig:               &config,
		multusConfigDir:            config.MultusAutoconfigDir,
		multusConfigFilePath:       filepath.Join(config.CniConfigDir, multusConfigFileName),
		primaryCNIConfigPath:       filepath.Join(config.MultusAutoconfigDir, defaultCNIPluginName),
		readinessIndicatorFilePath: config.ReadinessIndicatorFile,
	}

	if err := configManager.loadPrimaryCNIConfigFromFile(); err != nil {
		return nil, fmt.Errorf("failed to load the primary CNI configuration as a multus delegate with error '%v'", err)
	}

	if config.OverrideNetworkName {
		if err := configManager.overrideNetworkName(); err != nil {
			return nil, logging.Errorf("could not override the network name: %v", err)
		}
	}

	return configManager, nil
}

// Start generates an updated Multus config, writes it, and begins watching
// the config directory and readiness indicator files for changes
func (m *Manager) Start(ctx context.Context, wg *sync.WaitGroup) error {
	generatedMultusConfig, err := m.GenerateConfig()
	if err != nil {
		return logging.Errorf("failed to generated the multus configuration: %v", err)
	}
	logging.Verbosef("Generated MultusCNI config: %s", generatedMultusConfig)

	multusConfigFile, err := m.PersistMultusConfig(generatedMultusConfig)
	if err != nil {
		return logging.Errorf("failed to persist the multus configuration: %v", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := m.monitorPluginConfiguration(ctx); err != nil {
			_ = logging.Errorf("error watching file: %v", err)
		}
		logging.Verbosef("ConfigWatcher done")
		logging.Verbosef("Delete old config @ %v", multusConfigFile)
		os.Remove(multusConfigFile)
	}()

	return nil
}

func (m *Manager) loadPrimaryCNIConfigFromFile() error {
	primaryCNIConfigData, err := primaryCNIData(m.primaryCNIConfigPath)
	if err != nil {
		return logging.Errorf("failed to access the primary CNI configuration from %s: %v", m.primaryCNIConfigPath, err)
	}

	if err = CheckVersionCompatibility(m.multusConfig, primaryCNIConfigData); err != nil {
		return err
	}

	return m.loadPrimaryCNIConfigurationData(primaryCNIConfigData)
}

// overrideNetworkName overrides the name of the multus configuration with the
// name of the delegated primary CNI.
func (m *Manager) overrideNetworkName() error {
	name, ok := m.cniConfigData["name"]
	if !ok {
		return fmt.Errorf("failed to access delegate CNI plugin name")
	}
	networkName := name.(string)

	if networkName == "" {
		return fmt.Errorf("the primary CNI Configuration does not feature the network name: %v", m.cniConfigData)
	}
	m.multusConfig.Name = networkName
	return nil
}

func (m *Manager) loadPrimaryCNIConfigurationData(primaryCNIConfigData interface{}) error {
	cniConfigData := primaryCNIConfigData.(map[string]interface{})

	m.cniConfigData = cniConfigData
	m.multusConfig.ClusterNetwork = m.primaryCNIConfigPath
	return m.multusConfig.setCapabilities(cniConfigData)
}

// GenerateConfig generates a multus configuration from its current state
func (m *Manager) GenerateConfig() (string, error) {
	if err := m.loadPrimaryCNIConfigFromFile(); err != nil {
		_ = logging.Errorf("failed to read the primary CNI plugin config from %s", m.primaryCNIConfigPath)
		return "", nil
	}
	return m.multusConfig.Generate()
}

// monitorPluginConfiguration monitors the configuration file pointed
// to by the primaryCNIPluginName attribute, and re-generates the multus
// configuration whenever the primary CNI config is updated.
func (m *Manager) monitorPluginConfiguration(ctx context.Context) error {
	logging.Verbosef("started to watch file %s", m.primaryCNIConfigPath)

	for {
		select {
		case event := <-m.configWatcher.Events:
			if !m.shouldRegenerateConfig(event) {
				continue
			}
			logging.Debugf("process event: %v", event)

			// if readinessIndicatorFile is removed, then restart multus
			if m.readinessIndicatorFilePath != "" && m.readinessIndicatorFilePath == event.Name {
				logging.Verbosef("readiness indicator file is gone. restart multus-daemon")
				os.Remove(m.multusConfigFilePath)
				os.Exit(2)
			}

			updatedConfig, err := m.GenerateConfig()
			if err != nil {
				_ = logging.Errorf("failed to regenerate the multus configuration: %v", err)
			}

			logging.Debugf("Re-generated MultusCNI config: %s", updatedConfig)
			if _, err := m.PersistMultusConfig(updatedConfig); err != nil {
				_ = logging.Errorf("failed to persist the multus configuration: %v", err)
			}
			if err := m.loadPrimaryCNIConfigFromFile(); err != nil {
				_ = logging.Errorf("failed to reload the updated config: %v", err)
			}

		case err := <-m.configWatcher.Errors:
			if err == nil {
				continue
			}
			logging.Errorf("CNI monitoring error %v", err)

		case <-ctx.Done():
			logging.Verbosef("Stopped monitoring, closing channel ...")
			_ = m.configWatcher.Close()
			return nil
		}
	}
}

// PersistMultusConfig persists the provided configuration to the disc, with
// Read / Write permissions. The output file path is `<multus auto config dir>/00-multus.conf`
func (m *Manager) PersistMultusConfig(config string) (string, error) {
	if _, err := os.Stat(m.multusConfigFilePath); err == nil {
		logging.Debugf("Overwriting Multus CNI configuration @ %s", m.multusConfigFilePath)
	} else {
		logging.Debugf("Writing Multus CNI configuration @ %s", m.multusConfigFilePath)
	}
	return m.multusConfigFilePath, os.WriteFile(m.multusConfigFilePath, []byte(config), UserRWPermission)
}

func (m *Manager) shouldRegenerateConfig(event fsnotify.Event) bool {
	// first, check the readiness indicator file existence
	if event.Name == m.readinessIndicatorFilePath {
		return event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)
	}

	// we're watching the DIR where the config sits, and the event
	// does not concern the primary CNI config. Skip it.
	if event.Name == m.primaryCNIConfigPath {
		return event.Has(fsnotify.Write) || event.Has(fsnotify.Create)
	}
	logging.Debugf("skipping un-related event %v", event)
	return false
}

func getPrimaryCNIPluginName(multusAutoconfigDir string) (string, error) {
	masterCniConfigFileName, err := findMasterPlugin(multusAutoconfigDir, 120)
	if err != nil {
		return "", fmt.Errorf("failed to find the cluster master CNI plugin: %w", err)
	}
	return masterCniConfigFileName, nil
}

func newWatcher(cniConfigDir string, readinessIndicatorDir string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create new watcher for %q: %v", cniConfigDir, err)
	}
	defer func() {
		// Close watcher on error
		if err != nil {
			watcher.Close()
		}
	}()

	if err = watcher.Add(cniConfigDir); err != nil {
		return nil, fmt.Errorf("failed to add watch on %q for cni config: %v", cniConfigDir, err)
	}
	// if readinessIndicatorDir is different from cniConfigDir,
	if readinessIndicatorDir != "" && cniConfigDir != readinessIndicatorDir {
		if err = watcher.Add(readinessIndicatorDir); err != nil {
			return nil, fmt.Errorf("failed to add watch on %q for readinessIndicator: %v", readinessIndicatorDir, err)
		}
	}

	return watcher, nil
}

func primaryCNIData(masterCNIPluginPath string) (interface{}, error) {
	masterCNIConfigData, err := os.ReadFile(masterCNIPluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the cluster primary CNI config %s: %w", masterCNIPluginPath, err)
	}

	var cniData interface{}
	if err := json.Unmarshal(masterCNIConfigData, &cniData); err != nil {
		return nil, fmt.Errorf("failed to unmarshall primary CNI config: %w", err)
	}
	return cniData, nil
}
