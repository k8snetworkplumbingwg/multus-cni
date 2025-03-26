/*
 * Copyright 2023 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

const (
	GroupName = "gpu.resource.multus-cni.io"
	Version   = "v1alpha1"

	GpuConfigKind = "GpuConfig"
)

// Decoder implements a decoder for objects in this API group.
var Decoder runtime.Decoder

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GpuConfig holds the set of parameters for configuring a GPU.
type GpuConfig struct {
	metav1.TypeMeta `json:",inline"`
	Sharing         *GpuSharing `json:"sharing,omitempty"`
}

// DefaultGpuConfig provides the default GPU configuration.
func DefaultGpuConfig() *GpuConfig {
	return &GpuConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupName + "/" + Version,
			Kind:       GpuConfigKind,
		},
		Sharing: &GpuSharing{
			Strategy: TimeSlicingStrategy,
			TimeSlicingConfig: &TimeSlicingConfig{
				Interval: "Default",
			},
		},
	}
}

// Normalize updates a GpuConfig config with implied default values based on other settings.
func (c *GpuConfig) Normalize() error {
	if c == nil {
		return fmt.Errorf("config is 'nil'")
	}
	if c.Sharing == nil {
		c.Sharing = &GpuSharing{
			Strategy: TimeSlicingStrategy,
		}
	}
	if c.Sharing.Strategy == TimeSlicingStrategy && c.Sharing.TimeSlicingConfig == nil {
		c.Sharing.TimeSlicingConfig = &TimeSlicingConfig{
			Interval: "Default",
		}
	}
	if c.Sharing.Strategy == SpacePartitioningStrategy && c.Sharing.SpacePartitioningConfig == nil {
		c.Sharing.SpacePartitioningConfig = &SpacePartitioningConfig{
			PartitionCount: 1,
		}
	}
	return nil
}

func init() {
	// Create a new scheme and add our types to it. If at some point in the
	// future a new version of the configuration API becomes necessary, then
	// conversion functions can be generated and registered to continue
	// supporting older versions.
	scheme := runtime.NewScheme()
	schemeGroupVersion := schema.GroupVersion{
		Group:   GroupName,
		Version: Version,
	}
	scheme.AddKnownTypes(schemeGroupVersion,
		&GpuConfig{},
	)
	metav1.AddToGroupVersion(scheme, schemeGroupVersion)

	// Set up a json serializer to decode our types.
	Decoder = json.NewSerializerWithOptions(
		json.DefaultMetaFactory,
		scheme,
		scheme,
		json.SerializerOptions{
			Pretty: true, Strict: true,
		},
	)
}
