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

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilwait "k8s.io/apimachinery/pkg/util/wait"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/config"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	srv "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/server"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
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
	defaultMultusRunDir                 = "/host/var/run/multus-cni/"
	defaultMultusBinDir                 = "/host/opt/cni/bin"
	defaultMultusCNIDir                 = "/host/var/lib/cni/multus"
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
	multusRunDir                  = "multus-rundir"
	multusCNIDirVarName           = "cniDir"
	multusBinDirVarName           = "binDir"
)

func main() {
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
	overrideNetworkName := flag.Bool("override-network-name", false, "Used when we need overrides the name of the multus configuration with the name of the delegated primary CNI")
	multusBinDir := flag.String(multusBinDirVarName, defaultMultusBinDir, "The directory where the CNI plugin binaries are available")
	multusCniDir := flag.String(multusCNIDirVarName, defaultMultusCNIDir, "The directory where the multus CNI cache is located")

	configFilePath := flag.String("config", types.DefaultMultusDaemonConfigFile, "Specify the path to the multus-daemon configuration")

	flag.Parse()

	daemonConfig, err := types.LoadDaemonNetConf(*configFilePath)
	if err != nil {
		logging.Panicf("failed to load the multus-daemon configuration: %v", err)
		os.Exit(1)
	}

	if err := startMultusDaemon(daemonConfig); err != nil {
		logging.Panicf("failed start the multus thick-plugin listener: %v", err)
		os.Exit(3)
	}

	if *multusConfigFile == defaultMultusConfigFile {
		if *cniVersion == defaultMultusCNIVersion {
			_ = logging.Errorf("the CNI version is a mandatory parameter when the '-multus-config-file=auto' option is used")
		}

		var configurationOptions []config.Option
		configurationOptions = append(configurationOptions, config.WithAdditionalBinaryFileDir(*multusBinDir))
		configurationOptions = append(configurationOptions, config.WithCniDir(*multusCniDir))

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

		multusConfig, err := config.NewMultusConfig(multusPluginName, *cniVersion, *multusKubeconfig, configurationOptions...)
		if err != nil {
			_ = logging.Errorf("Failed to create multus config: %v", err)
			os.Exit(3)
		}

		var configManager *config.Manager
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

func startMultusDaemon(daemonConfig *types.ControllerNetConf) error {
	if err := srv.FilesystemPreRequirements(daemonConfig.MultusSocketDir); err != nil {
		return fmt.Errorf("failed to prepare the cni-socket for communicating with the shim: %w", err)
	}

	server, err := srv.NewCNIServer(daemonConfig.MultusSocketDir)
	if err != nil {
		return fmt.Errorf("failed to create the server: %v", err)
	}

	l, err := srv.GetListener(srv.SocketPath(daemonConfig.MultusSocketDir))
	if err != nil {
		return fmt.Errorf("failed to start the CNI server using socket %s. Reason: %+v", srv.SocketPath(daemonConfig.MultusSocketDir), err)
	}

	server.SetKeepAlivesEnabled(false)
	go utilwait.Forever(func() {
		logging.Debugf("open for business")
		if err := server.Serve(l); err != nil {
			utilruntime.HandleError(fmt.Errorf("CNI server Serve() failed: %v", err))
		}
	}, 0)

	return nil
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
