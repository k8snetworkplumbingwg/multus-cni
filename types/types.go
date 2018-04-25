// Copyright (c) 2017 Intel Corporation
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
//

package types

import (
	"net"

	"github.com/containernetworking/cni/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetConf for cni config file written in json
type NetConf struct {
	types.NetConf
	CNIDir     string                   `json:"cniDir"`
	Delegates  []map[string]interface{} `json:"delegates"`
	Kubeconfig string                   `json:"kubeconfig"`
	UseDefault bool                     `json:"always_use_default"`
}

type Network struct {
	metav1.TypeMeta `json:",inline"`
	// Note that ObjectMeta is mandatory, as an object
	// name is required
	Metadata metav1.ObjectMeta `json:"metadata,omitempty" description:"standard object metadata"`

	// Specification describing how to invoke a CNI plugin to
	// add or remove network attachments for a Pod.
	// In the absence of valid keys in a Spec, the runtime (or
	// meta-plugin) should load and execute a CNI .configlist
	// or .config (in that order) file on-disk whose JSON
	// “name” key matches this Network object’s name.
	// +optional
	Spec NetworkSpec `json:"spec"`
}

type NetworkSpec struct {
	// Config contains a standard JSON-encoded CNI configuration
	// or configuration list which defines the plugin chain to
	// execute.  If present, this key takes precedence over
	// ‘Plugin’.
	// +optional
	Config string `json:"config"`

	// Plugin contains the name of a CNI plugin on-disk in a
	// runtime-defined path (eg /opt/cni/bin and/or other paths.
	// This plugin should be executed with a basic CNI JSON
	// configuration on stdin containing the Network object
	// name and the plugin:
	//   { “cniVersion”: “0.3.1”, “type”: <Plugin>, “name”: <Network.Name> }
	// and any additional “runtimeConfig” field per the
	// CNI specification and conventions.
	// +optional
	Plugin string `json:"plugin"`
}

// K8sArgs is the valid CNI_ARGS used for Kubernetes
type K8sArgs struct {
	types.CommonArgs
	IP                         net.IP
	K8S_POD_NAME               types.UnmarshallableString
	K8S_POD_NAMESPACE          types.UnmarshallableString
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}
