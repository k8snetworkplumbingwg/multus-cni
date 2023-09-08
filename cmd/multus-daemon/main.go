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
	"os/signal"
	"os/user"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	utilwait "k8s.io/apimachinery/pkg/util/wait"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/multus"
	srv "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/api"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/server/config"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"

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

	configWatcherDoneChannel := make(chan struct{})
	multusConfigFile := ""
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	daemonConf, err := cniServerConfig(*configFilePath)
	if err != nil {
		os.Exit(1)
	}

	multusConf, err := config.ParseMultusConfig(*configFilePath)
	if err != nil {
		logging.Panicf("startMultusDaemon failed to load the multus configuration: %v", err)
		os.Exit(1)
	}

	logging.Verbosef("multus-daemon started")

	if multusConf.ReadinessIndicatorFile != "" {
		// Check readinessindicator file before daemon launch
		logging.Verbosef("Readiness Indicator file check")
		if err := types.GetReadinessIndicatorFile(multusConf.ReadinessIndicatorFile); err != nil {
			_ = logging.Errorf("have you checked that your default network is ready? still waiting for readinessindicatorfile @ %v. pollimmediate error: %v", multusConf.ReadinessIndicatorFile, err)
			os.Exit(1)
		}
		logging.Verbosef("Readiness Indicator file check done!")
	}

	if err := startMultusDaemon(ctx, daemonConf); err != nil {
		logging.Panicf("failed start the multus thick-plugin listener: %v", err)
		os.Exit(3)
	}

	// Wait until daemon ready
	logging.Verbosef("API readiness check")
	if waitUntilAPIReady(daemonConf.SocketDir) != nil {
		logging.Panicf("failed to ready multus-daemon socket: %v", err)
		os.Exit(1)
	}
	logging.Verbosef("API readiness check done!")

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

		multusConfigFile, err = configManager.PersistMultusConfig(generatedMultusConfig)
		if err != nil {
			_ = logging.Errorf("failed to persist the multus configuration: %v", err)
		}

		go func(ctx context.Context, doneChannel chan<- struct{}) {
			if err := configManager.MonitorPluginConfiguration(ctx, doneChannel); err != nil {
				_ = logging.Errorf("error watching file: %v", err)
			}
		}(ctx, configWatcherDoneChannel)
	} else {
		if err := copyUserProvidedConfig(multusConf.MultusConfigFile, multusConf.CniConfigDir); err != nil {
			logging.Errorf("failed to copy the user provided configuration %s: %v", multusConf.MultusConfigFile, err)
		}
	}

	signalCh := make(chan os.Signal, 16)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range signalCh {
			logging.Verbosef("caught %v, stopping...", sig)
			cancel()
		}
	}()

	var wg sync.WaitGroup
	if multusConf.MultusConfigFile == "auto" {
		wg.Add(1)
		go func() {
			<-configWatcherDoneChannel
			logging.Verbosef("ConfigWatcher done")
			logging.Verbosef("Delete old config @ %v", multusConfigFile)
			os.Remove(multusConfigFile)
			wg.Done()
		}()
	}

	wg.Wait()
	logging.Verbosef("multus daemon is exited")
}

func waitUntilAPIReady(socketPath string) error {
	apiReadyPollDuration := 100 * time.Millisecond
	apiReadyPollTimeout := 1000 * time.Millisecond

	return utilwait.PollImmediate(apiReadyPollDuration, apiReadyPollTimeout, func() (bool, error) {
		_, err := api.DoCNI(api.GetAPIEndpoint(api.MultusHealthAPIEndpoint), nil, api.SocketPath(socketPath))
		return err == nil, nil
	})
}

func startMultusDaemon(ctx context.Context, daemonConfig *srv.ControllerNetConf) error {
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
		go utilwait.UntilWithContext(ctx, func(ctx context.Context) {
			http.Handle("/metrics", promhttp.Handler())
			logging.Debugf("metrics port: %d", *daemonConfig.MetricsPort)
			logging.Debugf("metrics: %s", http.ListenAndServe(fmt.Sprintf(":%d", *daemonConfig.MetricsPort), nil))
		}, 0)
	}

	l, err := srv.GetListener(api.SocketPath(daemonConfig.SocketDir))
	if err != nil {
		return fmt.Errorf("failed to start the CNI server using socket %s. Reason: %+v", api.SocketPath(daemonConfig.SocketDir), err)
	}

	server.Start(ctx, l)

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
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
