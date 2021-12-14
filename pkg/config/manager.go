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

	"github.com/fsnotify/fsnotify"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
)

// MultusDefaultNetworkName holds the default name of the multus network
const (
	multusConfigFileName     = "00-multus.conf"
	MultusDefaultNetworkName = "multus-cni-network"
	userRWPermission         = 0600
)

// Manager monitors the configuration of the primary CNI plugin, and
// regenerates multus configuration whenever it gets updated.
type Manager struct {
	cniConfigData        map[string]interface{}
	configWatcher        *fsnotify.Watcher
	multusConfig         *MultusConf
	multusConfigDir      string
	multusConfigFilePath string
	primaryCNIConfigPath string
}

// NewManager returns a config manager object, configured to persist the
// configuration to `multusAutoconfigDir`. This constructor will auto-discover
// the primary CNI for which it will delegate.
func NewManager(config MultusConf, multusAutoconfigDir string) (*Manager, error) {
	defaultCNIPluginName, err := primaryCNIPluginName(multusAutoconfigDir)
	if err != nil {
		_ = logging.Errorf("failed to find the primary CNI plugin: %v", err)
		return nil, err
	}
	return newManager(config, multusAutoconfigDir, defaultCNIPluginName)
}

// NewManagerWithExplicitPrimaryCNIPlugin returns a config manager object,
// configured to persist the configuration to `multusAutoconfigDir`. This
// constructor will use the primary CNI plugin indicated by the user, via the
// primaryCNIPluginName variable.
func NewManagerWithExplicitPrimaryCNIPlugin(config MultusConf, multusAutoconfigDir string, primaryCNIPluginName string) (*Manager, error) {
	return newManager(config, multusAutoconfigDir, primaryCNIPluginName)
}

func newManager(config MultusConf, multusConfigDir string, defaultCNIPluginName string) (*Manager, error) {
	watcher, err := newWatcher(multusConfigDir)
	if err != nil {
		return nil, err
	}

	configManager := &Manager{
		configWatcher:        watcher,
		multusConfig:         &config,
		multusConfigDir:      multusConfigDir,
		multusConfigFilePath: cniPluginConfigFilePath(multusConfigDir, multusConfigFileName),
		primaryCNIConfigPath: cniPluginConfigFilePath(multusConfigDir, defaultCNIPluginName),
	}

	if err := configManager.loadPrimaryCNIConfigFromFile(); err != nil {
		return nil, fmt.Errorf("failed to load the primary CNI configuration as a multus delegate with error '%v'", err)
	}

	return configManager, nil
}

func (m *Manager) loadPrimaryCNIConfigFromFile() error {
	primaryCNIConfigData, err := primaryCNIData(m.primaryCNIConfigPath)
	if err != nil {
		return logging.Errorf("failed to access the primary CNI configuration from %s: %v", m.primaryCNIConfigPath, err)
	}
	return m.loadPrimaryCNIConfigurationData(primaryCNIConfigData)
}

// OverrideNetworkName overrides the name of the multus configuration with the
// name of the delegated primary CNI.
func (m *Manager) OverrideNetworkName() error {
	name, ok := m.cniConfigData["name"]
	if !ok {
		return fmt.Errorf("failed to access delegate CNI plugin name")
	}
	networkName := name.(string)

	if networkName == "" {
		return fmt.Errorf("the primary CNI Configuration does not feature the network name: %v", m.cniConfigData)
	}
	return m.multusConfig.Mutate(WithOverriddenName(networkName))
}

func (m *Manager) loadPrimaryCNIConfigurationData(primaryCNIConfigData interface{}) error {
	cniConfigData := primaryCNIConfigData.(map[string]interface{})

	m.cniConfigData = cniConfigData
	return m.multusConfig.Mutate(
		withDelegates(cniConfigData),
		withCapabilities(cniConfigData))
}

// GenerateConfig generates a multus configuration from its current state
func (m Manager) GenerateConfig() (string, error) {
	if err := m.loadPrimaryCNIConfigFromFile(); err != nil {
		_ = logging.Errorf("failed to read the primary CNI plugin config from %s", m.primaryCNIConfigPath)
		return "", nil
	}
	return m.multusConfig.Generate()
}

// MonitorDelegatedPluginConfiguration monitors the configuration file pointed
// to by the primaryCNIPluginName attribute, and re-generates the multus
// configuration whenever the primary CNI config is updated.
func (m Manager) MonitorDelegatedPluginConfiguration(shutDown chan struct{}, done chan struct{}) error {
	logging.Verbosef("started to watch file %s", m.primaryCNIConfigPath)

	for {
		select {
		case event := <-m.configWatcher.Events:
			// we're watching the DIR where the config sits, and the event
			// does not concern the primary CNI config. Skip it.
			if event.Name != m.primaryCNIConfigPath {
				logging.Debugf("skipping un-related event %v", event)
				continue
			}

			if !shouldRegenerateConfig(event) {
				continue
			}

			updatedConfig, err := m.GenerateConfig()
			if err != nil {
				_ = logging.Errorf("failed to regenerate the multus configuration: %v", err)
			}

			logging.Debugf("Re-generated MultusCNI config: %s", updatedConfig)
			if err := m.PersistMultusConfig(updatedConfig); err != nil {
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

		case <-shutDown:
			logging.Verbosef("Stopped monitoring, closing channel ...")
			_ = m.configWatcher.Close()
			done <- struct{}{}
			return nil
		}
	}
}

// PersistMultusConfig persists the provided configuration to the disc, with
// Read / Write permissions. The output file path is `<multus auto config dir>/00-multus.conf`
func (m Manager) PersistMultusConfig(config string) error {
	return ioutil.WriteFile(m.multusConfigFilePath, []byte(config), userRWPermission)
}

func primaryCNIPluginName(multusAutoconfigDir string) (string, error) {
	masterCniConfigFileName, err := findMasterPlugin(multusAutoconfigDir, 120)
	if err != nil {
		return "", fmt.Errorf("failed to find the cluster master CNI plugin: %w", err)
	}
	return masterCniConfigFileName, nil
}

func cniPluginConfigFilePath(cniConfigDir string, cniConfigFileName string) string {
	return cniConfigDir + fmt.Sprintf("/%s", cniConfigFileName)
}

func newWatcher(cniConfigDir string) (*fsnotify.Watcher, error) {
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
		return nil, fmt.Errorf("failed to add watch on %q: %v", cniConfigDir, err)
	}

	return watcher, nil
}

func shouldRegenerateConfig(event fsnotify.Event) bool {
	return event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Create == fsnotify.Create
}

func primaryCNIData(masterCNIPluginPath string) (interface{}, error) {
	masterCNIConfigData, err := ioutil.ReadFile(masterCNIPluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the cluster primary CNI config %s: %w", masterCNIPluginPath, err)
	}

	var cniData interface{}
	if err := json.Unmarshal(masterCNIConfigData, &cniData); err != nil {
		return nil, fmt.Errorf("failed to unmarshall primary CNI config: %w", err)
	}
	return cniData, nil
}
