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
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/config"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/multus"
)

const (
	multusPluginName     = "multus"
	multusConfigFileName = "00-multus.conf"
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
	multusConfigFileVarName       = "multus-conf-file"
	multusGlobalNamespaces        = "global-namespaces"
	multusLogFile                 = "multus-log-file"
	multusLogLevel                = "multus-log-level"
	multusLogToStdErr             = "multus-log-to-stderr"
	multusKubeconfigPath          = "multus-kubeconfig-file-host"
	multusMasterCNIFileVarName    = "multus-master-cni-file"
	multusNamespaceIsolation      = "namespace-isolation"
	multusReadinessIndicatorFile  = "readiness-indicator-file"
)

func main() {
	versionOpt := false
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

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
	flag.BoolVar(&versionOpt, "version", false, "Show application version")
	flag.BoolVar(&versionOpt, "v", false, "Show application version")
	flag.Parse()
	if versionOpt == true {
		fmt.Printf("%s\n", multus.PrintVersionString())
		return
	}

	if *logToStdErr {
		logging.SetLogStderr(*logToStdErr)
	}
	if *logFile != defaultMultusLogFile {
		logging.SetLogFile(*logFile)
	}
	if *logLevel != defaultMultusLogLevel {
		logging.SetLogLevel(*logLevel)
	}

	if *multusConfigFile == defaultMultusConfigFile {
		if *cniVersion == defaultMultusCNIVersion {
			_ = logging.Errorf("the CNI version is a mandatory parameter when the '-multus-config-file=auto' option is used")
		}

		var configurationOptions []config.Option
		if *namespaceIsolation {
			configurationOptions = append(
				configurationOptions, config.WithNamespaceIsolation())
		}

		if *globalNamespaces != defaultMultusGlobalNamespaces {
			configurationOptions = append(
				configurationOptions, config.WithGlobalNamespaces(*globalNamespaces))
		}

		if *logToStdErr != defaultMultusLogToStdErr {
			configurationOptions = append(
				configurationOptions, config.WithLogToStdErr())
		}

		if *logLevel != defaultMultusLogLevel {
			configurationOptions = append(
				configurationOptions, config.WithLogLevel(*logLevel))
		}

		if *logFile != defaultMultusLogFile {
			configurationOptions = append(
				configurationOptions, config.WithLogFile(*logFile))
		}

		if *additionalBinDir != defaultMultusAdditionalBinDir {
			configurationOptions = append(
				configurationOptions, config.WithAdditionalBinaryFileDir(*additionalBinDir))
		}

		if *readinessIndicator != defaultMultusReadinessIndicatorFile {
			configurationOptions = append(
				configurationOptions, config.WithReadinessFileIndicator(*readinessIndicator))
		}
		multusConfig := config.NewMultusConfig(multusPluginName, *cniVersion, *multusKubeconfig, configurationOptions...)

		var configManager *config.Manager
		var err error
		if *multusMasterCni == "" {
			configManager, err = config.NewManager(*multusConfig, *multusAutoconfigDir)
		} else {
			configManager, err = config.NewManagerWithExplicitPrimaryCNIPlugin(
				*multusConfig, *multusAutoconfigDir, *multusMasterCni)
		}
		if err != nil {
			_ = logging.Errorf("failed to create the configuration manager for the primary CNI plugin: %v", err)
			os.Exit(2)
		}

		if *overrideNetworkName {
			if err := configManager.OverrideNetworkName(); err != nil {
				_ = logging.Errorf("could not override the network name: %v", err)
			}
		}

		generatedMultusConfig, err := configManager.GenerateConfig()
		if err != nil {
			_ = logging.Errorf("failed to generated the multus configuration: %v", err)
		}
		logging.Verbosef("Generated MultusCNI config: %s", generatedMultusConfig)

		if err := configManager.PersistMultusConfig(generatedMultusConfig); err != nil {
			_ = logging.Errorf("failed to persist the multus configuration: %v", err)
		}

		configWatcherDoneChannel := make(chan struct{})
		go func(stopChannel chan struct{}, doneChannel chan struct{}) {
			defer func() {
				stopChannel <- struct{}{}
			}()
			if err := configManager.MonitorDelegatedPluginConfiguration(stopChannel, configWatcherDoneChannel); err != nil {
				_ = logging.Errorf("error watching file: %v", err)
			}
		}(make(chan struct{}), configWatcherDoneChannel)

		<-configWatcherDoneChannel
	} else {
		if err := copyUserProvidedConfig(*multusConfigFile, *cniConfigDir); err != nil {
			logging.Errorf("failed to copy the user provided configuration %s: %v", *multusConfigFile, err)
		}
	}
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
