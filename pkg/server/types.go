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
