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

// This binary works as a server that receives requests from multus-shim
// CNI plugin and creates network interface for kubernets pods.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilwait "k8s.io/apimachinery/pkg/util/wait"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/multus"
	srv "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/server"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/server/api"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/server/config"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	multusPluginName = "multus-shim"
)

const (
	defaultCniConfigDir                 = "/etc/cni/net.d"
	defaultMultusGlobalNamespaces       = ""
	defaultMultusLogFile                = ""
	defaultMultusLogMaxSize             = 100 // megabytes
	defaultMultusLogMaxAge              = 5   // days
	defaultMultusLogMaxBackups          = 5
	defaultMultusLogCompress            = true
	defaultMultusLogLevel               = ""
	defaultMultusLogToStdErr            = false
	defaultMultusMasterCNIFile          = ""
	defaultMultusNamespaceIsolation     = false
	defaultMultusReadinessIndicatorFile = ""
	defaultSocketDir                    = "/run/multus/"
)

func main() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	cniConfigDir := flag.String("cni-config-dir", defaultCniConfigDir, "CNI config dir")
	multusConfigFile := flag.String("multus-conf-file", "auto", "The multus configuration file to use. By default, a new configuration is generated.")
	multusMasterCni := flag.String("multus-master-cni-file", "", "The relative name of the configuration file of the cluster primary CNI.")
	multusAutoconfigDir := flag.String("multus-autoconfig-dir", *cniConfigDir, "The directory path for the generated multus configuration.")
	namespaceIsolation := flag.Bool("namespace-isolation", false, "If the network resources are only available within their defined namespaces.")
	globalNamespaces := flag.String("global-namespaces", "", "Comma-separated list of namespaces which can be referred to globally when namespace isolation is enabled.")
	logToStdErr := flag.Bool("multus-log-to-stderr", false, "If the multus logs are also to be echoed to stderr.")
	logLevel := flag.String("multus-log-level", "", "One of: debug/verbose/error/panic. Used only with --multus-conf-file=auto.")
	logFile := flag.String("multus-log-file", "", "Path where to multus will log. Used only with --multus-conf-file=auto.")
	logMaxSize := flag.Int("multus-log-max-size", defaultMultusLogMaxSize, "The maximum size in megabytes of the log file before it gets rotated")
	logMaxAge := flag.Int("multus-log-max-age", defaultMultusLogMaxAge, "The maximum number of days to retain old log files in their filename")
	logMaxBackups := flag.Int("multus-log-max-backups", defaultMultusLogMaxBackups, "The maximum number of old log files to retain")
	logCompress := flag.Bool("multus-log-compress", defaultMultusLogCompress, "Compress determines if the rotated log files should be compressed using gzip")
	cniVersion := flag.String("cni-version", "", "Allows you to specify CNI spec version. Used only with --multus-conf-file=auto.")
	socketDir := flag.String("socket-dir", defaultSocketDir, "Specifies the directory where the socket file resides.")
	forceCNIVersion := flag.Bool("force-cni-version", false, "Force to use given CNI version. only for kind-e2e testing") // this is only for kind-e2e
	readinessIndicator := flag.String("readiness-indicator-file", "", "Which file should be used as the readiness indicator. Used only with --multus-conf-file=auto.")
	overrideNetworkName := flag.Bool("override-network-name", false, "Used when we need overrides the name of the multus configuration with the name of the delegated primary CNI")
	version := flag.Bool("version", false, "Show version")

	configFilePath := flag.String("config", types.DefaultMultusDaemonConfigFile, "Specify the path to the multus-daemon configuration")

	flag.Parse()

	if *version {
		fmt.Printf("multus-daemon: %s\n", multus.PrintVersionString())
		os.Exit(4)
	}

	configWatcherStopChannel := make(chan struct{})
	configWatcherDoneChannel := make(chan struct{})
	serverStopChannel := make(chan struct{})
	serverDoneChannel := make(chan struct{})
	if err := startMultusDaemon(*configFilePath, serverStopChannel, serverDoneChannel); err != nil {
		logging.Panicf("failed start the multus thick-plugin listener: %v", err)
		os.Exit(3)
	}

	// Generate multus CNI config from current CNI config
	if *multusConfigFile == "auto" {
		if *cniVersion == "" {
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

		if *readinessIndicator != defaultMultusReadinessIndicatorFile {
			configurationOptions = append(
				configurationOptions, config.WithReadinessFileIndicator(*readinessIndicator))
		}

		configurationOptions = append(
			configurationOptions, config.WithCniConfigDir(*cniConfigDir))

		configurationOptions = append(
			configurationOptions, config.WithSocketDir(*socketDir))

		// logOptions

		var logOptionFuncs []config.LogOptionFunc
		if *logMaxAge != defaultMultusLogMaxAge {
			logOptionFuncs = append(logOptionFuncs, config.WithLogMaxAge(logMaxAge))
		}
		if *logMaxSize != defaultMultusLogMaxSize {
			logOptionFuncs = append(logOptionFuncs, config.WithLogMaxSize(logMaxSize))
		}
		if *logMaxBackups != defaultMultusLogMaxBackups {
			logOptionFuncs = append(logOptionFuncs, config.WithLogMaxBackups(logMaxBackups))
		}
		if *logCompress != defaultMultusLogCompress {
			logOptionFuncs = append(logOptionFuncs, config.WithLogCompress(logCompress))
		}

		if len(logOptionFuncs) > 0 {
			logOptions := &config.LogOptions{}
			config.MutateLogOptions(logOptions, logOptionFuncs...)
			configurationOptions = append(configurationOptions, config.WithLogOptions(logOptions))
		}

		multusConfig, err := config.NewMultusConfig(multusPluginName, *cniVersion, configurationOptions...)
		if err != nil {
			_ = logging.Errorf("Failed to create multus config: %v", err)
			os.Exit(3)
		}

		var configManager *config.Manager
		if *multusMasterCni == "" {
			configManager, err = config.NewManager(*multusConfig, *multusAutoconfigDir, *forceCNIVersion)
		} else {
			configManager, err = config.NewManagerWithExplicitPrimaryCNIPlugin(
				*multusConfig, *multusAutoconfigDir, *multusMasterCni, *forceCNIVersion)
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

		go func(stopChannel chan<- struct{}, doneChannel chan<- struct{}) {
			if err := configManager.MonitorPluginConfiguration(configWatcherStopChannel, doneChannel); err != nil {
				_ = logging.Errorf("error watching file: %v", err)
			}
		}(configWatcherStopChannel, configWatcherDoneChannel)

		<-configWatcherDoneChannel
	} else {
		if err := copyUserProvidedConfig(*multusConfigFile, *cniConfigDir); err != nil {
			logging.Errorf("failed to copy the user provided configuration %s: %v", *multusConfigFile, err)
		}
	}

	serverDone := false
	configWatcherDone := false
	for {
		select {
		case <-configWatcherDoneChannel:
			logging.Verbosef("ConfigWatcher done")
			configWatcherDone = true
		case <-serverDoneChannel:
			logging.Verbosef("multus-server done.")
			serverDone = true
		}

		if serverDone && configWatcherDone {
			return
		}
	}
	// never reached
}

func startMultusDaemon(configFilePath string, stopCh chan struct{}, done chan struct{}) error {
	daemonConfig, config, err := types.LoadDaemonNetConf(configFilePath)
	if err != nil {
		logging.Panicf("failed to load the multus-daemon configuration: %v", err)
		os.Exit(1)
	}

	if user, err := user.Current(); err != nil || user.Uid != "0" {
		return fmt.Errorf("failed to run multus-daemon with root: %v, now running in uid: %s", err, user.Uid)
	}

	if err := srv.FilesystemPreRequirements(daemonConfig.MultusSocketDir); err != nil {
		return fmt.Errorf("failed to prepare the cni-socket for communicating with the shim: %w", err)
	}

	server, err := srv.NewCNIServer(daemonConfig, config)
	if err != nil {
		return fmt.Errorf("failed to create the server: %v", err)
	}

	if daemonConfig.MetricsPort != nil {
		go utilwait.Until(func() {
			http.Handle("/metrics", promhttp.Handler())
			logging.Debugf("metrics port: %d", *daemonConfig.MetricsPort)
			logging.Debugf("metrics: %s", http.ListenAndServe(fmt.Sprintf(":%d", *daemonConfig.MetricsPort), nil))
		}, 0, stopCh)
	}

	l, err := srv.GetListener(api.SocketPath(daemonConfig.MultusSocketDir))
	if err != nil {
		return fmt.Errorf("failed to start the CNI server using socket %s. Reason: %+v", api.SocketPath(daemonConfig.MultusSocketDir), err)
	}

	server.SetKeepAlivesEnabled(false)
	go func() {
		utilwait.Until(func() {
			logging.Debugf("open for business")
			if err := server.Serve(l); err != nil {
				utilruntime.HandleError(fmt.Errorf("CNI server Serve() failed: %v", err))
			}
		}, 0, stopCh)
		server.Shutdown(context.TODO())
		close(done)
	}()

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
	nBytes, err := io.Copy(dstFile, srcFile)
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
