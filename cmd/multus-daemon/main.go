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
	"net/http/pprof"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/netutil"
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

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	daemonConf, err := cniServerConfig(*configFilePath)
	if err != nil {
		logging.Panicf("startMultusDaemon failed to load the CNI server configuration: %v", err)
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

	var configManager *config.Manager
	var ignoreReadinessIndicator bool
	if multusConf.MultusConfigFile == "auto" {
		if multusConf.CNIVersion == "" {
			_ = logging.Errorf("the CNI version is a mandatory parameter when the '-multus-config-file=auto' option is used")
		}

		// Generate multus CNI config from current CNI config
		configManager, err = config.NewManager(*multusConf)
		if err != nil {
			_ = logging.Errorf("failed to create the configuration manager for the primary CNI plugin: %v", err)
			os.Exit(2)
		}
		// ConfigManager watches the readiness indicator file (if configured)
		// and exits the daemon when that is removed. The CNIServer does
		// not need to re-do that check every CNI operation
		ignoreReadinessIndicator = true
	} else {
		if err := copyUserProvidedConfig(multusConf.MultusConfigFile, multusConf.CniConfigDir); err != nil {
			logging.Errorf("failed to copy the user provided configuration %s: %v", multusConf.MultusConfigFile, err)
		}
	}

	if err := startMultusDaemon(ctx, daemonConf, ignoreReadinessIndicator); err != nil {
		logging.Panicf("failed start the multus thick-plugin listener: %v", err)
		os.Exit(3)
	}

	// Wait until daemon ready
	logging.Verbosef("API readiness check")
	if api.WaitUntilAPIReady(daemonConf.SocketDir) != nil {
		logging.Panicf("failed to ready multus-daemon socket: %v", err)
		os.Exit(1)
	}
	logging.Verbosef("API readiness check done!")

	signalCh := make(chan os.Signal, 16)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range signalCh {
			logging.Verbosef("caught %v, stopping...", sig)
			cancel()
		}
	}()

	var wg sync.WaitGroup
	if configManager != nil {
		if err := configManager.Start(ctx, &wg); err != nil {
			_ = logging.Errorf("failed to start config manager: %v", err)
			os.Exit(3)
		}
	}

	wg.Wait()
	logging.Verbosef("multus daemon is exited")
}

func startMultusDaemon(ctx context.Context, daemonConfig *srv.ControllerNetConf, ignoreReadinessIndicator bool) error {
	if user, err := user.Current(); err != nil || user.Uid != "0" {
		return fmt.Errorf("failed to run multus-daemon with root: %v, now running in uid: %s", err, user.Uid)
	}

	if err := srv.FilesystemPreRequirements(daemonConfig.SocketDir); err != nil {
		return fmt.Errorf("failed to prepare the cni-socket for communicating with the shim: %w", err)
	}

	server, err := srv.NewCNIServer(daemonConfig, daemonConfig.ConfigFileContents, ignoreReadinessIndicator)
	if err != nil {
		return fmt.Errorf("failed to create the server: %v", err)
	}

	if daemonConfig.MetricsPort != nil {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		if daemonConfig.EnablePprof != nil && *daemonConfig.EnablePprof {
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			logging.Verbosef("pprof endpoints enabled on metrics port %d", *daemonConfig.MetricsPort)
		}
		metricsSrv := &http.Server{
			Addr:              fmt.Sprintf(":%d", *daemonConfig.MetricsPort),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		logging.Debugf("metrics port: %d", *daemonConfig.MetricsPort)
		go utilwait.UntilWithContext(ctx, func(_ context.Context) {
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Debugf("metrics server error: %v", err)
			}
		}, 0)
		go func() {
			<-ctx.Done()
			metricsSrv.Shutdown(context.Background())
		}()
	}

	l, err := srv.GetListener(api.SocketPath(daemonConfig.SocketDir))
	if err != nil {
		return fmt.Errorf("failed to start the CNI server using socket %s. Reason: %+v", api.SocketPath(daemonConfig.SocketDir), err)
	}

	if limit := daemonConfig.ConnectionLimit; limit != nil {
		if *limit <= 0 {
			return fmt.Errorf("connection limit must be greater than 0, got %d", *limit)
		}
		logging.Debugf("connection limit: %d", *limit)
		l = netutil.LimitListener(l, *limit)
	}

	server.Start(ctx, l)

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return nil
}

func cniServerConfig(configFilePath string) (*srv.ControllerNetConf, error) {
	path, err := filepath.Abs(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("illegal path %s in server config path %s: %w", path, configFilePath, err)
	}

	configFileContents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return srv.LoadDaemonNetConf(configFileContents)
}

func copyUserProvidedConfig(multusConfigPath string, cniConfigDir string) error {
	path, err := filepath.Abs(multusConfigPath)
	if err != nil {
		return fmt.Errorf("illegal path %s in multusConfigPath %s: %w", path, multusConfigPath, err)
	}

	srcFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open (READ only) file %s: %w", path, err)
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
