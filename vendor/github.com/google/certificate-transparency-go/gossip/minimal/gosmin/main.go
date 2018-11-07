// Copyright 2018 Google Inc. All Rights Reserved.
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

// The gosmin binary runs a minimal gossip implementation.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency-go/gossip/minimal"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// Register PEMKeyFile and PrivateKey ProtoHandlers
	_ "github.com/google/trillian/crypto/keys/der/proto"
	_ "github.com/google/trillian/crypto/keys/pem/proto"
)

var (
	config          = flag.String("config", "", "File holding configuration in text proto format")
	metricsEndpoint = flag.String("metrics_endpoint", "", "Endpoint for serving metrics")
)

func main() {
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	g, err := minimal.NewGossiperFromFile(ctx, *config, nil, prometheus.MetricFactory{})
	if err != nil {
		glog.Exitf("failed to load --config: %v", err)
	}

	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Gossiper Starting ****")

	go awaitSignal(func() {
		glog.Warning("Cancelling master context")
		cancel()
	})

	if *metricsEndpoint != "" {
		// Run a separate handler for metrics.
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			metricsServer := http.Server{Addr: *metricsEndpoint, Handler: mux}
			err := metricsServer.ListenAndServe()
			glog.Warningf("Metrics server exited: %v", err)
		}()
	}

	g.Run(ctx)

	glog.Infof("Stopping server, about to exit")
	glog.Flush()

	// Give things a few seconds to tidy up
	time.Sleep(time.Second * 3)
}

// awaitSignal waits for standard termination signals, then runs the given
// function; it should be run as a separate goroutine.
func awaitSignal(doneFn func()) {
	// Arrange notification for the standard set of signals used to terminate a server
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Now block and wait for a signal
	sig := <-sigs
	glog.Warningf("Signal received: %v", sig)
	glog.Flush()

	doneFn()
}
