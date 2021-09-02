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
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	multusPluginName     = "multus"
	multusConfigFileName = "00-multus.conf"
	userRWPermission     = 0600
)

const (
	defaultCniConfigDir                 = "/etc/cni/net.d"
	defaultMultusAdditionalBinDir       = ""
	defaultMultusCNIVersion             = ""
	defaultMultusConfigFile             = "auto"
	defaultMultusGlobalNamespaces       = ""
	defaultMultusKubeconfigPath         = "/etc/cni/net.d/multus.d/multus.kubeconfig"
	defaultMultusLogFile                = ""
	defaultMultusLogLevel               = ""
	defaultMultusLogToStdErr            = false
	defaultMultusMasterCNIFile          = ""
	defaultMultusNamespaceIsolation     = false
	defaultMultusReadinessIndicatorFile = ""
)

const (
	cniConfigDirVarName           = "cni-config-dir"
	multusAdditionalBinDirVarName = "additional-bin-dir"
	multusAutoconfigDirVarName    = "multus-autoconfig-dir"
	multusCNIVersion              = "cni-version"
	multusConfigFileVarName       = "multus-config-file"
	multusGlobalNamespaces        = "global-namespaces"
	multusLogFile                 = "multus-log-file"
	multusLogLevel                = "multus-log-level"
	multusLogToStdErr             = "multus-log-to-stderr"
	multusKubeconfigPath          = "multus-kubeconfig-file-host"
	multusMasterCNIFileVarName    = "multus-master-cni-file"
	multusNamespaceIsolation      = "namespace-isolation"
	multusReadinessIndicatorFile  = "readiness-indicator-file"
)

type primaryCNIConfigData struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	CNIVersion   string                 `json:"cniVersion,omitempty"`
	Capabilities map[string]bool        `json:"capabilities,omitempty"`
	Plugins      []*delegatedPluginConf `json:"plugins,omitempty"`
}

type delegatedPluginConf struct {
	CNIVersion string `json:"cniVersion,omitempty"`

	Name         string          `json:"name,omitempty"`
	Type         string          `json:"type,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
}

func main() {
	cniConfigDir := flag.String(cniConfigDirVarName, defaultCniConfigDir, "CNI config dir")
	multusConfigFile := flag.String(multusConfigFileVarName, defaultMultusConfigFile, "The multus configuration file to use. By default, a new configuration is generated.")
	multusMasterCni := flag.String(multusMasterCNIFileVarName, defaultMultusMasterCNIFile, "The relative name of the configuration file of the cluster primary CNI.")
	multusAutoconfigDir := flag.String(multusAutoconfigDirVarName, *cniConfigDir, "The directory path for the generated multus configuration.")
	namespaceIsolation := flag.Bool(multusNamespaceIsolation, defaultMultusNamespaceIsolation, "If the network resources are only available within their defined namespaces.")
	globalNamespaces := flag.String(multusGlobalNamespaces, defaultMultusGlobalNamespaces, "Comma-separated list of namespaces which can be referred to globally when namespace isolation is enabled.")
	logToStdErr := flag.Bool(multusLogToStdErr, defaultMultusLogToStdErr, "If the multus logs are also to be echoed to stderr.")
	logLevel := flag.String(multusLogLevel, defaultMultusLogLevel, "One of: debug/verbose/error/panic. Used only with --multus-conf-file=auto.")
	logFile := flag.String(multusLogFile, defaultMultusLogFile, "Path where to multus will log. Used only with --multus-conf-file=auto.")
	cniVersion := flag.String(multusCNIVersion, defaultMultusCNIVersion, "Allows you to specify CNI spec version. Used only with --multus-conf-file=auto.")
	additionalBinDir := flag.String(multusAdditionalBinDirVarName, defaultMultusAdditionalBinDir, "Additional binary directory to specify in the configurations. Used only with --multus-conf-file=auto.")
	readinessIndicator := flag.String(multusReadinessIndicatorFile, defaultMultusReadinessIndicatorFile, "Which file should be used as the readiness indicator. Used only with --multus-conf-file=auto.")
	multusKubeconfig := flag.String(multusKubeconfigPath, defaultMultusKubeconfigPath, "The path to the kubeconfig")
	overrideNetworkName := flag.Bool("override-network-name", false, "Used when ")
	flag.Parse()

	if *multusConfigFile == defaultMultusConfigFile {
		if *cniVersion == defaultMultusCNIVersion {
			logInvalidArg("the CNI version is a mandatory parameter when the '-multus-config-file=auto' option is used")
		}

		var masterCniConfigPath string
		if *multusMasterCni == "" {
			masterCniConfigFileName, err := findMasterPlugin(*multusAutoconfigDir, 120)
			if err != nil {
				logError("failed to find the cluster master CNI plugin: %w", err)
			}
			masterCniConfigPath = cniPluginConfigFilePath(*multusAutoconfigDir, masterCniConfigFileName)
		}

		masterCNIPluginForMultusConfigGeneration, err := primaryCNIData(masterCniConfigPath)
		if err != nil {
			logError("failed to access the primary CNI configuration from %s: %w", masterCniConfigPath, err)
		}
		networkName := "multus-cni-network"
		if *overrideNetworkName {
			structuredMasterCNIPluginData, err := relevantPrimaryCNIData(masterCniConfigPath)
			if err != nil {
				logError("failed to extract the relevant CNI data from %s: %w", masterCniConfigPath, err)
			}
			networkName = structuredMasterCNIPluginData.Name
		}

		multusConfig := newMultusConfig(networkName, multusPluginName, *cniVersion, *multusKubeconfig, *masterCNIPluginForMultusConfigGeneration)

		if *namespaceIsolation {
			multusConfig.withNamespaceIsolation()
		}

		if *globalNamespaces != defaultMultusGlobalNamespaces {
			multusConfig.withGlobalNamespaces(*globalNamespaces)
		}

		if *logToStdErr != defaultMultusLogToStdErr {
			multusConfig.withLogToStdErr()
		}

		if *logLevel != defaultMultusLogLevel {
			multusConfig.withLogLevel(*logLevel)
		}

		if *logFile != defaultMultusLogFile {
			multusConfig.withLogFile(*logFile)
		}

		if *additionalBinDir != defaultMultusAdditionalBinDir {
			multusConfig.withAdditionalBinaryFileDir(*additionalBinDir)
		}

		if *readinessIndicator != defaultMultusReadinessIndicatorFile {
			multusConfig.withReadinessFileIndicator(*readinessIndicator)
		}

		structuredMasterCNIPluginData, err := relevantPrimaryCNIData(masterCniConfigPath)
		if err != nil {
			logError("failed to access the primary CNI plugin configuration: %w", err)
		}

		multusConfig.withCapabilities(*structuredMasterCNIPluginData)

		generatedMultusConfig, err := multusConfig.generate()
		if err != nil {
			logError("failed to generated the multus configuration: %w", err)
		}
		logInfo("\n\nGenerated MultusCNI config: %s\n", generatedMultusConfig)

		if err := persistMultusConfig(generatedMultusConfig, cniPluginConfigFilePath(*multusAutoconfigDir, multusConfigFileName)); err != nil {
			logError("failed to persist the multus configuration: %w", err)
		}
	} else {
		if err := copyUserProvidedConfig(*multusConfigFile, *cniConfigDir); err != nil {
			logError("failed to copy the user provided configuration %s: %w", *multusConfigFile, err)
		}
	}
}

func cniPluginConfigFilePath(cniConfigDir string, cniConfigFileName string) string {
	return cniConfigDir + fmt.Sprintf("/%s", cniConfigFileName)
}

func relevantPrimaryCNIData(masterCNIPluginPath string) (*primaryCNIConfigData, error) {
	masterCNIConfigData, err := ioutil.ReadFile(masterCNIPluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the cluster primary CNI config %s: %w", masterCNIPluginPath, err)
	}

	cniData := &primaryCNIConfigData{}
	if err := json.Unmarshal(masterCNIConfigData, cniData); err != nil {
		return nil, fmt.Errorf("failed to unmarshall primary CNI config: %w", err)
	}
	return cniData, nil
}

func primaryCNIData(masterCNIPluginPath string) (*map[string]interface{}, error) {
	masterCNIConfigData, err := ioutil.ReadFile(masterCNIPluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the cluster primary CNI config %s: %w", masterCNIPluginPath, err)
	}

	cniData := &map[string]interface{}{}
	if err := json.Unmarshal(masterCNIConfigData, cniData); err != nil {
		return nil, fmt.Errorf("failed to unmarshall primary CNI config: %w", err)
	}
	return cniData, nil
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

func copyUserProvidedConfig(multusConfigPath string, cniConfigDir string) error {
	srcFile, err := os.Open(multusConfigPath)
	if err != nil {
		return fmt.Errorf("failed to open (READ only) file %s: %w", multusConfigPath, err)
	}

	dstFileName := cniConfigDir + "/" + filepath.Base(multusConfigPath)
	dstFile, err := os.Create(dstFileName)
	if err != nil {
		return fmt.Errorf("creating copying file %s: %w", dstFileName, err)
	}
	nBytes, err := io.Copy(srcFile, dstFile)
	if err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}
	srcFileInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat the file: %w", err)
	} else if nBytes != srcFileInfo.Size() {
		return fmt.Errorf("error copying file - copied only %d bytes out of %d", nBytes, srcFileInfo.Size())
	}
	return nil
}

func persistMultusConfig(config string, multusConfigFilePath string) error {
	return ioutil.WriteFile(multusConfigFilePath, []byte(config), userRWPermission)
}

func logInvalidArg(format string, values ...interface{}) {
	log.Printf("ERROR: %s", fmt.Errorf(format, values...).Error())
	flag.PrintDefaults()
	os.Exit(1)
}

func logError(format string, values ...interface{}) {
	log.Printf("ERROR: %s", fmt.Errorf(format, values...).Error())
	os.Exit(1)
}

func logInfo(format string, values ...interface{}) {
	log.Printf("INFO: %s", fmt.Sprintf(format, values...))
}
