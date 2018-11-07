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

package minimal

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/certificate-transparency-go/gossip/minimal/configpb"
	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/certificate-transparency-go/x509util"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/monitoring"

	logclient "github.com/google/certificate-transparency-go/client"
	hubclient "github.com/google/trillian-examples/gossip/client"
)

// NewGossiperFromFile creates a gossiper from the given filename, which should
// contain text-protobuf encoded configuration data, together with an optional
// http Client.
func NewGossiperFromFile(ctx context.Context, filename string, hc *http.Client, mf monitoring.MetricFactory) (*Gossiper, error) {
	cfgText, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfgProto configpb.GossipConfig
	if err := proto.UnmarshalText(string(cfgText), &cfgProto); err != nil {
		return nil, fmt.Errorf("%s: failed to parse gossip config: %v", filename, err)
	}
	cfg, err := NewGossiper(ctx, &cfgProto, hc, mf)
	if err != nil {
		return nil, fmt.Errorf("%s: config error: %v", filename, err)
	}
	return cfg, nil
}

// NewGossiper creates a gossiper from the given configuration protobuf and optional
// http client.
func NewGossiper(ctx context.Context, cfg *configpb.GossipConfig, hc *http.Client, mf monitoring.MetricFactory) (*Gossiper, error) {
	once.Do(func() { setupMetrics(mf) })
	if len(cfg.DestHub) == 0 {
		return nil, errors.New("no dest hub config found")
	}
	if len(cfg.SourceLog) == 0 {
		return nil, errors.New("no source log config found")
	}

	needPrivKey := false
	for _, destHub := range cfg.DestHub {
		if !destHub.IsHub {
			// Destinations include at least one CT Log, so need a private key
			// for cert generation for all such destinations.
			needPrivKey = true
			break
		}
	}

	var signer crypto.Signer
	var root *x509.Certificate
	if needPrivKey {
		if cfg.PrivateKey == nil {
			return nil, errors.New("no private key found")
		}
		var keyProto ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(cfg.PrivateKey, &keyProto); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cfg.PrivateKey: %v", err)
		}
		var err error
		signer, err = keys.NewSigner(ctx, keyProto.Message)
		if err != nil {
			return nil, fmt.Errorf("failed to load private key: %v", err)
		}

		root, err = x509util.CertificateFromPEM([]byte(cfg.RootCert))
		if err != nil {
			return nil, fmt.Errorf("failed to parse root cert: %v", err)
		}
	}

	dests := make(map[string]*destHub)
	for _, lc := range cfg.DestHub {
		hub, err := hubFromProto(lc, hc)
		if err != nil {
			return nil, fmt.Errorf("failed to parse dest hub config: %v", err)
		}
		if _, ok := dests[hub.Name]; ok {
			return nil, fmt.Errorf("duplicate dest hubs for name %s", hub.Name)
		}
		dests[hub.Name] = hub
		isHub := 0.0
		if lc.IsHub {
			isHub = 1.0
		}
		destPureHub.Set(isHub, hub.Name)
	}
	srcs := make(map[string]*sourceLog)
	for _, lc := range cfg.SourceLog {
		base, err := logConfigFromProto(lc, hc)
		if err != nil {
			return nil, fmt.Errorf("failed to parse source log config: %v", err)
		}
		if _, ok := srcs[base.Name]; ok {
			return nil, fmt.Errorf("duplicate source logs for name %s", base.Name)
		}
		srcs[base.Name] = &sourceLog{logConfig: *base}
		knownSourceLogs.Set(1.0, base.Name)
	}

	return &Gossiper{
		signer:     signer,
		root:       root,
		dests:      dests,
		srcs:       srcs,
		bufferSize: int(cfg.BufferSize),
	}, nil
}

func logConfigFromProto(cfg *configpb.LogConfig, hc *http.Client) (*logConfig, error) {
	if cfg.Name == "" {
		return nil, errors.New("no log name provided")
	}
	interval, err := ptypes.Duration(cfg.MinReqInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MinReqInterval: %v", err)
	}
	opts := jsonclient.Options{PublicKeyDER: cfg.PublicKey.GetDer()}
	client, err := logclient.New(cfg.Url, hc, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create log client for %q: %v", cfg.Name, err)
	}
	if client.Verifier == nil {
		glog.Warningf("No public key provided for log %s, signature checks will be skipped", cfg.Name)
	}
	return &logConfig{
		Name:        cfg.Name,
		URL:         cfg.Url,
		Log:         client,
		MinInterval: interval,
	}, nil
}

func hubFromProto(cfg *configpb.HubConfig, hc *http.Client) (*destHub, error) {
	if cfg.Name == "" {
		return nil, errors.New("no source log name provided")
	}
	interval, err := ptypes.Duration(cfg.MinReqInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MinReqInterval: %v", err)
	}
	var submitter hubSubmitter
	opts := jsonclient.Options{PublicKeyDER: cfg.PublicKey.GetDer()}
	if cfg.IsHub {
		cl, err := hubclient.New(cfg.Url, hc, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create hub client for %q: %v", cfg.Name, err)
		}
		if cl.Verifier == nil {
			glog.Warningf("No public key provided for hub %s, signature checks will be skipped", cfg.Name)
		}
		submitter = &pureHubSubmitter{cl}
	} else {
		cl, err := logclient.New(cfg.Url, hc, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create log client for %q: %v", cfg.Name, err)
		}
		if cl.Verifier == nil {
			glog.Warningf("No public key provided for CT log %s, signature checks will be skipped", cfg.Name)
		}
		submitter = &ctLogSubmitter{cl}
	}
	return &destHub{
		Name:              cfg.Name,
		URL:               cfg.Url,
		Submitter:         submitter,
		MinInterval:       interval,
		lastHubSubmission: make(map[string]time.Time),
	}, nil
}

func hubScannerFromProto(cfg *configpb.HubConfig, hc *http.Client) (*hubScanner, error) {
	if cfg.Name == "" {
		return nil, errors.New("no hub name provided")
	}
	interval, err := ptypes.Duration(cfg.MinReqInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MinReqInterval: %v", err)
	}
	opts := jsonclient.Options{PublicKeyDER: cfg.PublicKey.GetDer()}
	if cfg.IsHub {
		return nil, errors.New("Pure Gossip Hubs not yet supported")
	}
	cl, err := logclient.New(cfg.Url, hc, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create log client for %q: %v", cfg.Name, err)
	}
	if cl.Verifier == nil {
		glog.Warningf("No public key provided for CT log %s, signature checks will be skipped", cfg.Name)
	}
	return &hubScanner{Name: cfg.Name, URL: cfg.Url, MinInterval: interval, Log: cl}, nil
}
