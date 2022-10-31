// Copyright (c) 2022 Multus Authors
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

package server

import (
	"net/http"

	"github.com/containernetworking/cni/pkg/invoke"

	"github.com/prometheus/client_golang/prometheus"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/k8sclient"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

// Metrics represents server's metrics.
type Metrics struct {
	requestCounter *prometheus.CounterVec
}

// Server represents an HTTP server listening to a unix socket. It will handle
// the CNI shim requests issued when a pod is added / removed.
type Server struct {
	http.Server
	rundir       string
	kubeclient   *k8sclient.ClientInfo
	exec         invoke.Exec
	serverConfig []byte
	metrics      *Metrics
}

// ShimNetConf for the shim cni config file written in json
type ShimNetConf struct {
	types.NetConf

	MultusSocketDir string `json:"socketDir"`
	LogFile         string `json:"logFile,omitempty"`
	LogLevel        string `json:"logLevel,omitempty"`
	LogToStderr     bool   `json:"logToStderr,omitempty"`
}

// ControllerNetConf for the controller cni configuration
type ControllerNetConf struct {
	ConfDir     string `json:"confDir"`
	CNIDir      string `json:"cniDir"`
	BinDir      string `json:"binDir"`
	LogFile     string `json:"logFile"`
	LogLevel    string `json:"logLevel"`
	LogToStderr bool   `json:"logToStderr,omitempty"`

	// Option to point to the path of the unix domain socket through which the
	// multus client / server communicate.
	MultusSocketDir string `json:"socketDir"`
}
