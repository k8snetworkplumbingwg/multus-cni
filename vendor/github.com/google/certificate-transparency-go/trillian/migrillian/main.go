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

// Migrillian tool transfers certs from CT logs to Trillian pre-ordered logs in
// the same order.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	"github.com/google/certificate-transparency-go/client"
	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/trillian/migrillian/configpb"
	"github.com/google/certificate-transparency-go/trillian/migrillian/core"
	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/election2"
	"github.com/google/trillian/util/election2/etcd"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	cfgPath = flag.String("config", "", "Path to migration config file")

	forceMaster = flag.Bool("force_master", false, "If true, assume master for all logs")
	etcdServers = flag.String("etcd_servers", "", "A comma-separated list of etcd servers; no etcd registration if empty")
	lockDir     = flag.String("lock_file_path", "/migrillian/master", "etcd lock file directory path")

	metricsEndpoint = flag.String("metrics_endpoint", "localhost:8099", "Endpoint for serving metrics")

	maxIdleConnsPerHost = flag.Int("max_idle_conns_per_host", 10, "Max idle HTTP connections per host (0 = DefaultMaxIdleConnsPerHost)")
	maxIdleConns        = flag.Int("max_idle_conns", 100, "Max number of idle HTTP connections across all hosts (0 = unlimited)")
	dialTimeout         = flag.Duration("grpc_dial_timeout", 5*time.Second, "Timeout for dialling Trillian")
)

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	defer glog.Flush()

	cfg, err := getConfig()
	if err != nil {
		glog.Exitf("Failed to load MigrillianConfig: %v", err)
	}
	backendMap, err := core.ValidateConfig(cfg)
	if err != nil {
		glog.Exitf("Failed to validate MigrillianConfig: %v", err)
	}

	// Dial all log backends.
	// TODO(pavelkalinnikov): Commonize backend connection code with CTFE.
	connMap := make(map[string]*grpc.ClientConn)
	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	if len(backendMap) == 1 {
		// If there's only one backend we can't serve anything until connected.
		dialOpts = append(dialOpts, grpc.WithBlock())
	}
	for _, be := range backendMap {
		glog.Infof("Dialling Trillian backend: %v", be)
		conn, err := grpc.Dial(be.BackendSpec, dialOpts...)
		if err != nil {
			glog.Exitf("Could not dial Trillian server: %v: %v", be, err)
		}
		defer conn.Close()
		connMap[be.Name] = conn
	}

	httpClient := getHTTPClient()
	mf := prometheus.MetricFactory{}
	ef, closeFn := getElectionFactory()
	defer closeFn()

	ctx := context.Background()
	var ctrls []*core.Controller
	for _, mc := range cfg.MigrationConfigs.Config {
		conn := connMap[mc.LogBackendName] // Not nil (config is verified).
		ctrl, err := getController(ctx, mc, httpClient, mf, ef, conn)
		if err != nil {
			glog.Exitf("Failed to create Controller for %q: %v", mc.SourceUri, err)
		}
		ctrls = append(ctrls, ctrl)
	}

	// Handle metrics on the DefaultServeMux.
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		err := http.ListenAndServe(*metricsEndpoint, nil)
		glog.Fatalf("http.ListenAndServe(): %v", err)
	}()

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go util.AwaitSignal(cctx, cancel)

	core.RunMigration(cctx, ctrls)
}

// getController creates a single log migration Controller.
func getController(
	ctx context.Context,
	cfg *configpb.MigrationConfig,
	httpClient *http.Client,
	mf monitoring.MetricFactory,
	ef election2.Factory,
	conn *grpc.ClientConn,
) (*core.Controller, error) {
	ctOpts := jsonclient.Options{PublicKeyDER: cfg.PublicKey.Der}
	ctClient, err := client.New(cfg.SourceUri, httpClient, ctOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create CT client: %v", err)
	}
	plClient, err := newPreorderedLogClient(ctx, conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create PreorderedLogClient: %v", err)
	}

	opts := core.OptionsFromConfig(cfg)
	return core.NewController(opts, ctClient, plClient, ef, mf), nil
}

// getConfig returns MigrillianConfig loaded from the file specified in flags.
func getConfig() (*configpb.MigrillianConfig, error) {
	if len(*cfgPath) == 0 {
		return nil, errors.New("config file not specified")
	}
	cfg, err := core.LoadConfigFromFile(*cfgPath)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// getHTTPClient returns an HTTP client created from flags.
func getHTTPClient() *http.Client {
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		DisableKeepAlives:     false,
		MaxIdleConns:          *maxIdleConns,
		MaxIdleConnsPerHost:   *maxIdleConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	// TODO(pavelkalinnikov): Make the timeout tunable.
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
}

// newPreorderedLogClient creates a PreorderedLogClient for the specified tree.
func newPreorderedLogClient(
	ctx context.Context,
	conn *grpc.ClientConn,
	cfg *configpb.MigrationConfig,
) (*core.PreorderedLogClient, error) {
	admin := trillian.NewTrillianAdminClient(conn)
	gt := trillian.GetTreeRequest{TreeId: cfg.LogId}
	tree, err := admin.GetTree(ctx, &gt)
	if err != nil {
		return nil, err
	}
	log := trillian.NewTrillianLogClient(conn)
	pref := fmt.Sprintf("%d", cfg.LogId)
	return core.NewPreorderedLogClient(log, tree, cfg.IdentityFunction, pref)
}

// getElectionFactory returns an election factory based on flags, and a
// function which releases the resources associated with the factory.
func getElectionFactory() (election2.Factory, func()) {
	if *forceMaster {
		glog.Warning("Acting as master for all logs")
		return election2.NoopFactory{}, func() {}
	}
	if len(*etcdServers) == 0 {
		glog.Exit("Either --force_master or --etcd_servers must be supplied")
	}

	cli, err := etcd.NewClient(strings.Split(*etcdServers, ","), 5*time.Second)
	if err != nil || cli == nil {
		glog.Exitf("Failed to create etcd client: %v", err)
	}
	closeFn := func() {
		if err := cli.Close(); err != nil {
			glog.Warningf("etcd client Close(): %v", err)
		}
	}

	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s.%d", hostname, os.Getpid())
	factory := etcd.NewFactory(instanceID, cli, *lockDir)

	return factory, closeFn
}
