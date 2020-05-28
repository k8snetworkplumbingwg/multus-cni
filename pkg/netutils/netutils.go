// Copyright (c) 2019 Multus Authors
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

package netutils

import (
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	"gopkg.in/intel/multus-cni.v3/pkg/logging"
	"github.com/vishvananda/netlink"
	"net"
	"strings"
)

// DeleteDefaultGW removes the default gateway from marked interfaces.
func DeleteDefaultGW(args *skel.CmdArgs, ifName string, res *cnitypes.Result) (*current.Result, error) {
	result, err := current.NewResultFromResult(*res)
	if err != nil {
		return nil, logging.Errorf("DeleteDefaultGW: Error creating new from current CNI result: %v", err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return nil, logging.Errorf("DeleteDefaultGW: Error getting namespace %v", err)
	}
	defer netns.Close()

	err = netns.Do(func(_ ns.NetNS) error {
		var err error
		link, _ := netlink.LinkByName(ifName)
		routes, _ := netlink.RouteList(link, netlink.FAMILY_ALL)
		for _, nlroute := range routes {
			if nlroute.Dst == nil {
				err = netlink.RouteDel(&nlroute)
			}
		}
		return err
	})
	var newRoutes []*cnitypes.Route
	for _, route := range result.Routes {
		if mask, _ := route.Dst.Mask.Size(); mask != 0 {
			newRoutes = append(newRoutes, route)
		}
	}
	result.Routes = newRoutes
	return result, err
}

// SetDefaultGW adds a default gateway on a specific interface
func SetDefaultGW(args *skel.CmdArgs, ifName string, gateways []net.IP, res *cnitypes.Result) (*current.Result, error) {

	// Use the current CNI result...
	result, err := current.NewResultFromResult(*res)
	if err != nil {
		return nil, logging.Errorf("SetDefaultGW: Error creating new CNI result from current: %v", err)
	}

	// This ensures we're acting within the net namespace for the pod.
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return nil, logging.Errorf("SetDefaultGW: Error getting namespace %v", err)
	}
	defer netns.Close()

	var newResultDefaultRoutes []*cnitypes.Route

	// Do this within the net namespace.
	err = netns.Do(func(_ ns.NetNS) error {
		var err error

		// Pick up the link info as we need the index.
		link, _ := netlink.LinkByName(ifName)

		// Cycle through all the desired gateways.
		for _, gw := range gateways {

			// Create a new route (note: dst is nil by default)
			logging.Debugf("SetDefaultGW: Adding default route on %v (index: %v) to %v", ifName, link.Attrs().Index, gw)
			newDefaultRoute := netlink.Route{
				LinkIndex: link.Attrs().Index,
				Gw:        gw,
			}

			// Build a new element for the results route

			// Set a correct CIDR depending on IP type
			_, dstipnet, _ := net.ParseCIDR("::0/0")
			if strings.Count(gw.String(), ":") < 2 {
				_, dstipnet, _ = net.ParseCIDR("0.0.0.0/0")
			}
			newResultDefaultRoutes = append(newResultDefaultRoutes, &cnitypes.Route{Dst: *dstipnet, GW: gw})

			// Perform the creation of the default route....
			err = netlink.RouteAdd(&newDefaultRoute)
			if err != nil {
				logging.Errorf("SetDefaultGW: Error adding route: %v", err)
			}
		}
		return err
	})

	result.Routes = newResultDefaultRoutes
	return result, err

}
