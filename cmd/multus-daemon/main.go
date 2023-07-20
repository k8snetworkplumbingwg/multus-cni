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

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus"
	srv "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/api"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/config"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// keep in command line option
	version := flag.Bool("version", false, "Show version")

	configFilePath := flag.String("config", srv.DefaultMultusDaemonConfigFile, "Specify the path to the multus-daemon configuration")

	flag.Parse()

	if *version {
		fmt.Printf("multus-daemon: %s\n", multus.PrintVersionString())
		os.Exit(4)
	}

	configWatcherStopChannel := make(chan struct{})
	configWatcherDoneChannel := make(chan struct{})
	serverStopChannel := make(chan struct{})
	serverDoneChannel := make(chan struct{})

	daemonConf, err := cniServerConfig(*configFilePath)
	if err != nil {
		os.Exit(1)
	}

	if err := startMultusDaemon(daemonConf, serverStopChannel, serverDoneChannel); err != nil {
		logging.Panicf("failed start the multus thick-plugin listener: %v", err)
		os.Exit(3)
	}

	multusConf, err := config.ParseMultusConfig(*configFilePath)
	if err != nil {
		logging.Panicf("startMultusDaemon failed to load the multus configuration: %v", err)
		os.Exit(1)
	}

	// Generate multus CNI config from current CNI config
	if multusConf.MultusConfigFile == "auto" {
		if multusConf.CNIVersion == "" {
			_ = logging.Errorf("the CNI version is a mandatory parameter when the '-multus-config-file=auto' option is used")
		}

		var configManager *config.Manager
		if multusConf.MultusMasterCni == "" {
			configManager, err = config.NewManager(*multusConf, multusConf.MultusAutoconfigDir, multusConf.ForceCNIVersion)
		} else {
			configManager, err = config.NewManagerWithExplicitPrimaryCNIPlugin(
				*multusConf, multusConf.MultusAutoconfigDir, multusConf.MultusMasterCni, multusConf.ForceCNIVersion)
		}
		if err != nil {
			_ = logging.Errorf("failed to create the configuration manager for the primary CNI plugin: %v", err)
			os.Exit(2)
		}

		if multusConf.OverrideNetworkName {
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
		if err := copyUserProvidedConfig(multusConf.MultusConfigFile, multusConf.CniConfigDir); err != nil {
			logging.Errorf("failed to copy the user provided configuration %s: %v", multusConf.MultusConfigFile, err)
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

func startMultusDaemon(daemonConfig *srv.ControllerNetConf, stopCh chan struct{}, done chan struct{}) error {
	if user, err := user.Current(); err != nil || user.Uid != "0" {
		return fmt.Errorf("failed to run multus-daemon with root: %v, now running in uid: %s", err, user.Uid)
	}

	if err := srv.FilesystemPreRequirements(daemonConfig.SocketDir); err != nil {
		return fmt.Errorf("failed to prepare the cni-socket for communicating with the shim: %w", err)
	}

	server, err := srv.NewCNIServer(daemonConfig, daemonConfig.ConfigFileContents)
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

	l, err := srv.GetListener(api.SocketPath(daemonConfig.SocketDir))
	if err != nil {
		return fmt.Errorf("failed to start the CNI server using socket %s. Reason: %+v", api.SocketPath(daemonConfig.SocketDir), err)
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

func cniServerConfig(configFilePath string) (*srv.ControllerNetConf, error) {
	configFileContents, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}
	return srv.LoadDaemonNetConf(configFileContents)
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
