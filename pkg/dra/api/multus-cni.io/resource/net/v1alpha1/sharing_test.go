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

func TestGpuSharingGetTimeSlicingConfig(t *testing.T) {
	tests := map[string]struct {
		gpuSharing  *GpuSharing
		expected    *TimeSlicingConfig
		expectedErr error
	}{
		"nil GpuSharing": {
			gpuSharing:  nil,
			expectedErr: errors.New("no sharing set to get config from"),
		},
		"strategy is not TimeSlicing": {
			gpuSharing: &GpuSharing{
				Strategy: SpacePartitioningStrategy,
			},
			expectedErr: errors.New("strategy is not set to 'TimeSlicing'"),
		},
		"non-nil SpacePartitioningConfig": {
			gpuSharing: &GpuSharing{
				Strategy:                TimeSlicingStrategy,
				SpacePartitioningConfig: &SpacePartitioningConfig{},
			},
			expectedErr: errors.New("cannot use SpacePartitioningConfig with the 'TimeSlicing' strategy"),
		},
		"valid TimeSlicingConfig": {
			gpuSharing: &GpuSharing{
				Strategy: TimeSlicingStrategy,
				TimeSlicingConfig: &TimeSlicingConfig{
					Interval: LongTimeSlice,
				},
			},
			expected: &TimeSlicingConfig{
				Interval: LongTimeSlice,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			timeSlicing, err := test.gpuSharing.GetTimeSlicingConfig()
			assert.Equal(t, test.expected, timeSlicing)
			assert.Equal(t, test.expectedErr, err)
		})
	}

}
func TestGpuSharingGetSpacePartitioningConfig(t *testing.T) {
	tests := map[string]struct {
		gpuSharing  *GpuSharing
		expected    *SpacePartitioningConfig
		expectedErr error
	}{
		"nil GpuSharing": {
			gpuSharing:  nil,
			expectedErr: errors.New("no sharing set to get config from"),
		},
		"strategy is not SpacePartitioning": {
			gpuSharing: &GpuSharing{
				Strategy: TimeSlicingStrategy,
			},
			expectedErr: errors.New("strategy is not set to 'SpacePartitioning'"),
		},
		"non-nil TimeSlicingConfig": {
			gpuSharing: &GpuSharing{
				Strategy:          SpacePartitioningStrategy,
				TimeSlicingConfig: &TimeSlicingConfig{},
			},
			expectedErr: errors.New("cannot use TimeSlicingConfig with the 'SpacePartitioning' strategy"),
		},
		"valid SpacePartitioningConfig": {
			gpuSharing: &GpuSharing{
				Strategy: SpacePartitioningStrategy,
				SpacePartitioningConfig: &SpacePartitioningConfig{
					PartitionCount: 5,
				},
			},
			expected: &SpacePartitioningConfig{
				PartitionCount: 5,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			spacePartitioning, err := test.gpuSharing.GetSpacePartitioningConfig()
			assert.Equal(t, test.expected, spacePartitioning)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}
