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

func TestGpuConfigNormalize(t *testing.T) {
	tests := map[string]struct {
		gpuConfig   *GpuConfig
		expected    *GpuConfig
		expectedErr error
	}{
		"nil GpuConfig": {
			gpuConfig:   nil,
			expectedErr: errors.New("config is 'nil'"),
		},
		"empty GpuConfig": {
			gpuConfig: &GpuConfig{},
			expected: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: TimeSlicingStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{
						Interval: DefaultTimeSlice,
					},
				},
			},
		},
		"empty GpuConfig with SpacePartitioning": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: SpacePartitioningStrategy,
				},
			},
			expected: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: SpacePartitioningStrategy,
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: 1,
					},
				},
			},
		},
		"full GpuConfig": {
			gpuConfig: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: SpacePartitioningStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{
						Interval: ShortTimeSlice,
					},
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: 5,
					},
				},
			},
			expected: &GpuConfig{
				Sharing: &GpuSharing{
					Strategy: SpacePartitioningStrategy,
					TimeSlicingConfig: &TimeSlicingConfig{
						Interval: ShortTimeSlice,
					},
					SpacePartitioningConfig: &SpacePartitioningConfig{
						PartitionCount: 5,
					},
				},
			},
		},
		"default GpuConfig is already normalized": {
			gpuConfig: DefaultGpuConfig(),
			expected:  DefaultGpuConfig(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.gpuConfig.Normalize()
			assert.Equal(t, test.expected, test.gpuConfig)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}
