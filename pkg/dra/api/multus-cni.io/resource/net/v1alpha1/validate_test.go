/*
 * Copyright 2025 The Kubernetes Authors.
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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGpuConfigValidate(t *testing.T) {
	tests := map[string]struct {
		gpuConfig *GpuConfig
		expected  error
	}{
		"empty GpuConfig": {
			gpuConfig: &GpuConfig{},
			expected:  errors.New("no sharing strategy set"),
		},
		"empty GpuConfig.Sharing": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{},
			},
			expected: errors.New("unknown GPU sharing strategy: "),
		},
		"unknown GPU sharing strategy": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: "unknown",
				},
			},
			expected: errors.New("unknown GPU sharing strategy: unknown"),
		},
		"empty GpuConfig.Sharing.TimeSlicingConfig": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy:          TimeSlicingStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{},
				},
			},
			expected: errors.New("unknown time-slice interval: "),
		},
		"valid GpuConfig with TimeSlicing": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: TimeSlicingStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{
						Interval: MediumTimeSlice,
					},
				},
			},
			expected: nil,
		},
		"negative GpuConfig.Sharing.SpacePartitioningConfig.PartitionCount": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: SpacePartitioningStrategy,
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: -1,
					},
				},
			},
			expected: errors.New("invalid partition count: -1"),
		},
		"valid GpuConfig with SpacePartitioning": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: SpacePartitioningStrategy,
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: 1000,
					},
				},
			},
			expected: nil,
		},
		"default GpuConfig": {
			gpuConfig: DefaultGpuConfig(),
			expected:  nil,
		},
		"invalid TimeSlicingConfig ignored with strategy is SpacePartitioning": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy:          SpacePartitioningStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{},
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: 1,
					},
				},
			},
			expected: nil,
		},
		"invalid SpacePartitioningConfig ignored with strategy is TimeSlicing": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: TimeSlicingStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{
						Interval: MediumTimeSlice,
					},
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: -1,
					},
				},
			},
			expected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.gpuConfig.Validate()
			assert.Equal(t, test.expected, err)
		})
	}
}
