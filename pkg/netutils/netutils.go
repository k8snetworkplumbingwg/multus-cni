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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
)

// DeleteDefaultGW removes the default gateway from marked interfaces.
func DeleteDefaultGW(args *skel.CmdArgs, ifName string) error {
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return logging.Errorf("DeleteDefaultGW: Error getting namespace %v", err)
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
	return err
}

// SetDefaultGW adds a default gateway on a specific interface
func SetDefaultGW(args *skel.CmdArgs, ifName string, gateways []net.IP) error {
	// This ensures we're acting within the net namespace for the pod.
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return logging.Errorf("SetDefaultGW: Error getting namespace %v", err)
	}
	defer netns.Close()

	// Do this within the net namespace.
	err = netns.Do(func(_ ns.NetNS) error {
		var err error

		// Pick up the link info as we need the index.
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return logging.Errorf("SetDefaultGW: Error getting link %v", err)
		}

		// Cycle through all the desired gateways.
		for _, gw := range gateways {

			// Create a new route (note: dst is nil by default)
			logging.Debugf("SetDefaultGW: Adding default route on %v (index: %v) to %v", ifName, link.Attrs().Index, gw)
			newDefaultRoute := netlink.Route{
				LinkIndex: link.Attrs().Index,
				Gw:        gw,
			}

			// Perform the creation of the default route....
			err = netlink.RouteAdd(&newDefaultRoute)
			if err != nil {
				logging.Errorf("SetDefaultGW: Error adding route: %v", err)
			}
		}
		return err
	})

	return err
}

// DeleteDefaultGWCache updates libcni cache to remove default gateway routes in result
func DeleteDefaultGWCache(cacheDir string, rt *libcni.RuntimeConf, netName string, ifName string, ipv4, ipv6 bool) error {
	cacheFile := filepath.Join(cacheDir, "results", fmt.Sprintf("%s-%s-%s", netName, rt.ContainerID, rt.IfName))

	cache, err := ioutil.ReadFile(cacheFile)
	if err != nil {
		return err
	}
	logging.Debugf("DeleteDefaultGWCache: update cache to delete GW from: %s", string(cache))
	newCache, err := deleteDefaultGWCacheBytes(cache, ipv4, ipv6)
	if err != nil {
		return err
	}

	logging.Debugf("DeleteDefaultGWCache: update cache to delete GW: %s", string(newCache))
	return ioutil.WriteFile(cacheFile, newCache, 0600)
}

func deleteDefaultGWCacheBytes(cacheFile []byte, ipv4, ipv6 bool) ([]byte, error) {
	var cachedInfo map[string]interface{}
	if err := json.Unmarshal(cacheFile, &cachedInfo); err != nil {
		return nil, err
	}

	// try to get result
	_, ok := cachedInfo["result"]
	if !ok {
		return nil, fmt.Errorf("cannot get result from cache")
	}

	resultJSON, ok := cachedInfo["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("wrong result type: %v", cachedInfo["result"])
	}
	newResult, err := deleteDefaultGWResult(resultJSON, ipv4, ipv6)
	if err != nil {
		return nil, err
	}
	cachedInfo["result"] = newResult

	newCache, err := json.Marshal(cachedInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to encode json: %v", err)
	}
	return newCache, nil
}

func deleteDefaultGWResultRoutes(routes []interface{}, dstGW string) ([]interface{}, error) {
	for i, r := range routes {
		route, ok := r.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("wrong route format: %v", r)
		}
		_, ok = route["dst"]
		if ok {
			dst, ok := route["dst"].(string)
			if !ok {
				return nil, fmt.Errorf("wrong dst format: %v", route["dst"])
			}
			if dst == dstGW {
				routes = append(routes[:i], routes[i+1:]...)
			}
		}
	}
	return routes, nil
}

func deleteDefaultGWResult(result map[string]interface{}, ipv4, ipv6 bool) (map[string]interface{}, error) {
	// try to get cniVersion from result
	_, ok := result["cniVersion"]
	if !ok {
		// fallback to processing result for old cni version(0.1.0/0.2.0)
		return deleteDefaultGWResult020(result, ipv4, ipv6)
	}

	cniVersion, ok := result["cniVersion"].(string)
	if !ok {
		return nil, fmt.Errorf("wrong cniVersion format: %v", result["cniVersion"])
	}

	if cniVersion == "0.1.0" || cniVersion == "0.2.0" {
		// fallback to processing result for old cni version(0.1.0/0.2.0)
		return deleteDefaultGWResult020(result, ipv4, ipv6)
	}

	if cniVersion != "0.3.0" && cniVersion != "0.3.1" && cniVersion != "0.4.0" && cniVersion != "1.0.0" {
		return nil, fmt.Errorf("not supported version: %s", cniVersion)
	}

	_, ok = result["routes"]
	if !ok {
		// No route in result, hence we do nothing
		return result, nil
	}
	routes, ok := result["routes"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("wrong routes format: %v", result["routes"])
	}

	var err error
	// delete IPv4 default routes
	if ipv4 {
		routes, err = deleteDefaultGWResultRoutes(routes, "0.0.0.0/0")
		if err != nil {
			return nil, err
		}
	}

	if ipv6 {
		routes, err = deleteDefaultGWResultRoutes(routes, "::0/0")
		if err != nil {
			return nil, err
		}
	}
	result["routes"] = routes

	return result, nil
}

func deleteDefaultGWResult020(result map[string]interface{}, ipv4, ipv6 bool) (map[string]interface{}, error) {
	var err error
	if ipv4 {
		_, ok := result["ip4"]
		if ok {
			ip4, ok := result["ip4"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("wrong ip4 format: %v", result["ip4"])
			}

			_, ok = ip4["routes"]
			if ok {
				routes, ok := ip4["routes"].([]interface{})
				if !ok {
					return nil, fmt.Errorf("wrong ip4 routes format: %v", ip4["routes"])
				}

				routes, err = deleteDefaultGWResultRoutes(routes, "0.0.0.0/0")
				if err != nil {
					return nil, err
				}
				ip4["routes"] = routes
			}
		}
	}

	if ipv6 {
		_, ok := result["ip6"]
		if ok {
			ip6, ok := result["ip6"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("wrong ip6 format: %v", result["ip6"])
			}

			_, ok = ip6["routes"]
			if ok {
				routes, ok := ip6["routes"].([]interface{})
				if !ok {
					return nil, fmt.Errorf("wrong ip6 routes format: %v", ip6["routes"])
				}

				routes, err = deleteDefaultGWResultRoutes(routes, "::0/0")
				if err != nil {
					return nil, err
				}
				ip6["routes"] = routes
			}
		}
	}

	return result, nil
}

// AddDefaultGWCache updates libcni cache to add default gateway result
func AddDefaultGWCache(cacheDir string, rt *libcni.RuntimeConf, netName string, ifName string, gw []net.IP) error {
	cacheFile := filepath.Join(cacheDir, "results", fmt.Sprintf("%s-%s-%s", netName, rt.ContainerID, rt.IfName))

	cache, err := ioutil.ReadFile(cacheFile)
	if err != nil {
		return err
	}
	logging.Debugf("AddDefaultGWCache: update cache to add GW from: %s", string(cache))
	newCache, err := addDefaultGWCacheBytes(cache, gw)
	if err != nil {
		return err
	}

	logging.Debugf("AddDefaultGWCache: update cache to add GW: %s", string(newCache))
	return ioutil.WriteFile(cacheFile, newCache, 0600)
}

func addDefaultGWCacheBytes(cacheFile []byte, gw []net.IP) ([]byte, error) {
	var cachedInfo map[string]interface{}
	if err := json.Unmarshal(cacheFile, &cachedInfo); err != nil {
		return nil, err
	}

	// try to get result
	_, ok := cachedInfo["result"]
	if !ok {
		return nil, fmt.Errorf("cannot get result from cache")
	}

	resultJSON, ok := cachedInfo["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("wrong result type: %v", cachedInfo["result"])
	}
	newResult, err := addDefaultGWResult(resultJSON, gw)
	if err != nil {
		return nil, err
	}
	cachedInfo["result"] = newResult

	newCache, err := json.Marshal(cachedInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to encode json: %v", err)
	}
	return newCache, nil
}

func addDefaultGWResult(result map[string]interface{}, gw []net.IP) (map[string]interface{}, error) {
	// try to get cniVersion from result
	_, ok := result["cniVersion"]
	if !ok {
		// fallback to processing result for old cni version(0.1.0/0.2.0)
		return addDefaultGWResult020(result, gw)
	}

	cniVersion, ok := result["cniVersion"].(string)
	if !ok {
		return nil, fmt.Errorf("wrong cniVersion format: %v", result["cniVersion"])
	}

	if cniVersion == "0.1.0" || cniVersion == "0.2.0" {
		// fallback to processing result for old cni version(0.1.0/0.2.0)
		return addDefaultGWResult020(result, gw)
	}

	if cniVersion != "0.3.0" && cniVersion != "0.3.1" && cniVersion != "0.4.0" && cniVersion != "1.0.0" {
		return nil, fmt.Errorf("not supported version: %s", cniVersion)
	}

	routes := []interface{}{}
	_, ok = result["routes"]
	if ok {
		routes, ok = result["routes"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("wrong routes format: %v", result["routes"])
		}
	}

	for _, g := range gw {
		dst := "0.0.0.0/0"
		if g.To4() == nil {
			dst = "::0/0"
		}
		routes = append(routes, map[string]string{
			"dst": dst,
			"gw":  g.String(),
		})
	}
	result["routes"] = routes

	return result, nil
}

func addDefaultGWResult020(result map[string]interface{}, gw []net.IP) (map[string]interface{}, error) {
	for _, g := range gw {
		if g.To4() != nil {
			_, ok := result["ip4"]
			if ok {
				ip4, ok := result["ip4"].(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("wrong ip4 format: %v", result["ip4"])
				}
				routes := []interface{}{}
				_, ok = ip4["routes"]
				if ok {
					routes, ok = ip4["routes"].([]interface{})
					if !ok {
						return nil, fmt.Errorf("wrong ip4 routes format: %v", ip4["routes"])
					}
				}
				ip4["routes"] = append(routes, map[string]string{
					"dst": "0.0.0.0/0",
					"gw":  g.String(),
				})
			}
		} else {
			_, ok := result["ip6"]
			if ok {
				ip6, ok := result["ip6"].(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("wrong ip6 format: %v", result["ip4"])
				}
				routes := []interface{}{}
				_, ok = ip6["routes"]
				if ok {
					routes, ok = ip6["routes"].([]interface{})
					if !ok {
						return nil, fmt.Errorf("wrong ip6 routes format: %v", ip6["routes"])
					}
				}
				ip6["routes"] = append(routes, map[string]string{
					"dst": "::/0",
					"gw":  g.String(),
				})
			}
		}
	}
	return result, nil
}
