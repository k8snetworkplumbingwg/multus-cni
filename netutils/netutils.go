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

	"github.com/intel/multus-cni/logging"

	"github.com/vishvananda/netlink"
)

// DeleteDefaultGW removes the default gateway from marked interfaces.
func DeleteDefaultGW(args *skel.CmdArgs, ifName string, res *cnitypes.Result) (*current.Result, error) {
	logging.Debugf("XXX: DeleteDefaultGW: %s", args.Netns)
	result, err := current.NewResultFromResult(*res)
	if err != nil {
		return nil, logging.Errorf("XXX: %v", err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return nil, logging.Errorf("XXX: %v", err)
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
