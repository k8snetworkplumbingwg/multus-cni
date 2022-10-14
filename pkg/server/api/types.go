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

package api

import (
	cni100 "github.com/containernetworking/cni/pkg/types/100"
)

// Request sent to the Server by the multus-shim
type Request struct {
	// CNI environment variables, like CNI_COMMAND and CNI_NETNS
	Env map[string]string `json:"env,omitempty"`
	// CNI configuration passed via stdin to the CNI plugin
	Config []byte `json:"config,omitempty"`
	// Annotation for Delegate request
	InterfaceAttributes *DelegateInterfaceAttributes `json:"interfaceAttributes,omitempty"`
}

// DelegateInterfaceAttributes annotates delegate request for additional config
type DelegateInterfaceAttributes struct {
	// IPRequest contains an optional requested IP address for this network
	// attachment
	IPRequest []string `json:"ips,omitempty"`
	// MacRequest contains an optional requested MAC address for this
	// network attachment
	MacRequest string `json:"mac,omitempty"`
	// CNIArgs contains additional CNI arguments for the network interface
	CNIArgs *map[string]interface{} `json:"cni-args"`
}

// Response represents the response (computed in the CNI server) for
// ADD / DEL / CHECK for a Pod.
type Response struct {
	Result *cni100.Result
}
