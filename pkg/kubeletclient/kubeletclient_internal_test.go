// Copyright (c) 2026 The Multus Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package kubeletclient

import (
	"testing"

	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

// TestGetDevicePluginResourcesDoesNotAliasDeviceIds is a regression
// test for kubeletclient aliasing dev.DeviceIds into the cached
// kubelet response, which let SortDeviceIDs's in-place sort scramble
// rc.resources under concurrent GetPodResourceMap callers (#1495).
func TestGetDevicePluginResourcesDoesNotAliasDeviceIds(t *testing.T) {
	rc := &kubeletClient{}
	// The kubelet returns DeviceIds in an arbitrary order. Pre-sort to
	// a known shape so we can detect any subsequent mutation of the
	// backing array.
	original := []string{"dev-b", "dev-a", "dev-c"}
	deviceIds := append([]string(nil), original...)

	devices := []*podresourcesapi.ContainerDevices{
		{ResourceName: "vendor.example.com/gpu", DeviceIds: deviceIds},
	}
	resourceMap := map[string]*types.ResourceInfo{}

	rc.getDevicePluginResources(devices, resourceMap)

	// Sorting the map's stored slice must not bleed back into the caller's
	// kubelet response slice, which is a proxy for the cached
	// rc.resources backing array.
	types.SortDeviceIDs(resourceMap)

	for i := range original {
		if deviceIds[i] != original[i] {
			t.Fatalf("kubelet response was mutated at index %d: got %q want %q (full: %v)",
				i, deviceIds[i], original[i], deviceIds)
		}
	}

	stored := resourceMap["vendor.example.com/gpu"].DeviceIDs
	if len(stored) != len(original) {
		t.Fatalf("resourceMap slice length changed: got %d want %d", len(stored), len(original))
	}
	// SortDeviceIDs should have sorted the map-owned copy.
	expectedSorted := []string{"dev-a", "dev-b", "dev-c"}
	for i := range expectedSorted {
		if stored[i] != expectedSorted[i] {
			t.Fatalf("resourceMap slice not sorted at index %d: got %q want %q (full: %v)",
				i, stored[i], expectedSorted[i], stored)
		}
	}
}
