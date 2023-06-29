// Copyright (c) 2019 Intel Corporation
// Copyright (c) 2021 Multus Authors
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

package netutils

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types/020"
	cni040 "github.com/containernetworking/cni/pkg/types/040"
	cni100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"

	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNetutils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "netutils")
}

// helper function
func testAddRoute(link netlink.Link, ip net.IP, mask net.IPMask, gw net.IP) error {
	dst := &net.IPNet{
		IP:   ip,
		Mask: mask,
	}
	route := netlink.Route{LinkIndex: link.Attrs().Index, Dst: dst, Gw: gw}
	return netlink.RouteAdd(&route)
}

func testAddAddr(link netlink.Link, ip net.IP, mask net.IPMask) error {
	return netlink.AddrAdd(link, &netlink.Addr{IPNet: &net.IPNet{IP: ip, Mask: mask}})
}

func testGetResultFromCache(data []byte) []byte {
	var cachedInfo map[string]interface{}
	ExpectWithOffset(1, json.Unmarshal(data, &cachedInfo)).NotTo(HaveOccurred())

	// try to get result
	_, ok := cachedInfo["result"]
	ExpectWithOffset(1, ok).To(BeTrue())

	resultJSON, ok := cachedInfo["result"].(map[string]interface{})
	ExpectWithOffset(1, ok).To(BeTrue())

	resultByte, err := json.Marshal(resultJSON)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return resultByte
}

func test020ResultHasIPv4DefaultRoute(data []byte) bool {
	resultRaw, err := types020.NewResult(data)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	result, err := types020.GetResult(resultRaw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, r := range result.IP4.Routes {
		if r.Dst.String() == "0.0.0.0/0" {
			return true
		}
	}
	return false
}

func test020ResultHasIPv6DefaultRoute(data []byte) bool {
	resultRaw, err := types020.NewResult(data)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	result, err := types020.GetResult(resultRaw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, r := range result.IP6.Routes {
		if r.Dst.String() == "::/0" {
			return true
		}
	}
	return false
}

func test040ResultHasIPv4DefaultRoute(data []byte) bool {
	resultRaw, err := cni040.NewResult(data)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	result, err := cni040.GetResult(resultRaw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, r := range result.Routes {
		if r.Dst.String() == "0.0.0.0/0" {
			return true
		}
	}
	return false
}

func test040ResultHasIPv6DefaultRoute(data []byte) bool {
	resultRaw, err := cni040.NewResult(data)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	result, err := cni040.GetResult(resultRaw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, r := range result.Routes {
		if r.Dst.String() == "::/0" {
			return true
		}
	}
	return false
}

func test100ResultHasIPv4DefaultRoute(data []byte) bool {
	resultRaw, err := cni100.NewResult(data)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	result, err := cni100.GetResult(resultRaw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, r := range result.Routes {
		if r.Dst.String() == "0.0.0.0/0" {
			return true
		}
	}
	return false
}

func test100ResultHasIPv6DefaultRoute(data []byte) bool {
	resultRaw, err := cni100.NewResult(data)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	result, err := cni100.GetResult(resultRaw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	for _, r := range result.Routes {
		if r.Dst.String() == "::/0" {
			return true
		}
	}
	return false
}

var _ = Describe("netutil netlink function testing", func() {
	const IFNAME string = "dummy0"
	var IFMAC net.HardwareAddr = net.HardwareAddr([]byte{0x02, 0x66, 0x7d, 0xe3, 0x14, 0x1c})
	var originalNS ns.NetNS
	var targetNS ns.NetNS

	BeforeEach(func() {
		// Create a new NetNS so we don't modify the host
		var err error
		originalNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())

		targetNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())

		Expect(targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			Expect(netlink.LinkAdd(&netlink.Dummy{
				LinkAttrs: netlink.LinkAttrs{
					Name:         IFNAME,
					HardwareAddr: IFMAC,
					Index:        10,
				},
			})).Should(Succeed())

			_, err = netlink.LinkByName(IFNAME)
			Expect(err).NotTo(HaveOccurred())

			return nil
		})).Should(Succeed())
	})

	AfterEach(func() {
		Expect(originalNS.Close()).To(Succeed())
	})

	Context("test DeleteDefaultGW", func() {
		It("verify default gateway is removed", func() {
			Expect(targetNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				link, err := netlink.LinkByName(IFNAME)
				Expect(err).NotTo(HaveOccurred())
				Expect(netlink.LinkSetUp(link)).NotTo(HaveOccurred())

				// addr 10.0.0.2/24
				Expect(testAddAddr(link, net.IPv4(10, 0, 0, 2), net.CIDRMask(24, 32))).Should(Succeed())

				// add default gateway into IFNAME
				Expect(testAddRoute(link,
					net.IPv4(0, 0, 0, 0), net.CIDRMask(0, 0),
					net.IPv4(10, 0, 0, 1))).Should(Succeed())

				//"dst": "10.0.0.0/16"
				Expect(testAddRoute(link,
					net.IPv4(10, 0, 0, 0), net.CIDRMask(16, 32),
					net.IPv4(10, 0, 0, 1))).Should(Succeed())

				return nil
			})).Should(Succeed())

			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       targetNS.Path(),
				IfName:      IFNAME,
			}

			Expect(originalNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				Expect(DeleteDefaultGW(args.Netns, IFNAME)).Should(Succeed())
				return nil
			})).Should(Succeed())
		})
	})

	Context("test SetDefaultGW", func() {
		It("verify default gateway is removed", func() {
			Expect(targetNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()
				link, err := netlink.LinkByName(IFNAME)
				Expect(err).NotTo(HaveOccurred())
				Expect(netlink.LinkSetUp(link)).Should(Succeed())

				// addr 10.0.0.2/24
				Expect(testAddAddr(link, net.IPv4(10, 0, 0, 2), net.CIDRMask(24, 32))).Should(Succeed())

				//"dst": "10.0.0.0/16"
				Expect(testAddRoute(link,
					net.IPv4(10, 0, 0, 0), net.CIDRMask(16, 32),
					net.IPv4(10, 0, 0, 1))).Should(Succeed())

				return nil
			})).Should(Succeed())

			args := &skel.CmdArgs{
				ContainerID: "dummy",
				Netns:       targetNS.Path(),
				IfName:      IFNAME,
			}

			Expect(originalNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				Expect(SetDefaultGW(args.Netns, IFNAME, []net.IP{net.ParseIP("10.0.0.1")})).Should(Succeed())
				return nil
			})).Should(Succeed())
		})
	})

})

var _ = Describe("netutil cnicache function testing", func() {
	Context("test DeleteDefaultGWCache", func() {
		It("verify ipv4 default gateway is removed from CNI 0.1.0/0.2.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "dns": {},
    "ip4": {
      "ip": "10.1.1.103/24",
      "routes": [
        {
          "dst": "20.0.0.0/24",
          "gw": "10.1.1.1"
        },
        {
          "dst": "0.0.0.0/0",
          "gw": "10.1.1.1"
        },
        {
          "dst": "30.0.0.0/24",
          "gw": "10.1.1.1"
        }
      ]
    },
    "ip6": {
      "ip": "10::1:1:103/64",
      "routes": [
        {
          "dst": "20::0:0:0/56",
          "gw": "10::1:1:1"
        },
        {
          "dst": "::0/0",
          "gw": "10::1:1:1"
        },
        {
          "dst": "30::0:0:0/64",
          "gw": "10::1:1:1"
        }
      ]
    }
  }
}`)
			newResult, err := deleteDefaultGWCacheBytes(origResult, true, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(test020ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())
			Expect(test020ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 010/020 Result
			type CNICacheResult020 struct {
				Kind   string `json:"kind"`
				Result struct {
					IP4 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip4"`
					IP6 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip6"`
				} `json:"result"`
			}
			result := CNICacheResult020{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.IP4.Routes)).To(Equal(2))
			Expect(len(result.Result.IP6.Routes)).To(Equal(3))
		})

		It("verify ipv6 default gateway is removed from CNI 0.1.0/0.2.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "dns": {},
    "ip4": {
      "ip": "10.1.1.103/24",
      "routes": [
        {
          "dst": "20.0.0.0/24",
          "gw": "10.1.1.1"
        },
        {
          "dst": "0.0.0.0/0",
          "gw": "10.1.1.1"
        },
        {
          "dst": "30.0.0.0/24",
          "gw": "10.1.1.1"
        }
      ]
    },
    "ip6": {
      "ip": "10::1:1:103/64",
      "routes": [
        {
          "dst": "20::0:0:0/56",
          "gw": "10::1:1:1"
        },
        {
          "dst": "::0/0",
          "gw": "10::1:1:1"
        },
        {
          "dst": "30::0:0:0/64",
          "gw": "10::1:1:1"
        }
      ]
    }
  }
}`)
			newResult, err := deleteDefaultGWCacheBytes(origResult, false, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(test020ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test020ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 010/020 Result
			type CNICacheResult020 struct {
				Kind   string `json:"kind"`
				Result struct {
					IP4 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip4"`
					IP6 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip6"`
				} `json:"result"`
			}
			result := CNICacheResult020{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.IP4.Routes)).To(Equal(3))
			Expect(len(result.Result.IP6.Routes)).To(Equal(2))
		})

		It("verify ipv4 default gateway is removed from CNI 0.3.0/0.3.1/0.4.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "0.3.1",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0,
        "version": "4"
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0,
        "version": "6"
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "0.0.0.0/0",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "::0/0",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := deleteDefaultGWCacheBytes(origResult, true, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(test040ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())
			Expect(test040ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 0.3.0/0.3.1/0.4.0 Result
			type CNICacheResult030_040 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult030_040{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(5))
		})

		It("verify ipv6 default gateway is removed from CNI 0.3.0/0.3.1/0.4.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "0.3.1",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0,
        "version": "4"
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0,
        "version": "6"
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "0.0.0.0/0",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "::0/0",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := deleteDefaultGWCacheBytes(origResult, false, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(test040ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test040ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 0.3.0/0.3.1/0.4.0 Result
			type CNICacheResult030_040 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult030_040{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(5))
		})

		It("verify ipv4 default gateway is removed from CNI 1.0.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "1.0.0",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "0.0.0.0/0",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "::0/0",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := deleteDefaultGWCacheBytes(origResult, true, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(test100ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())
			Expect(test100ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 1.0.0 Result
			type CNICacheResult100 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult100{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(5))
		})

		It("verify ipv6 default gateway is removed from CNI 1.0.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "1.0.0",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "0.0.0.0/0",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "::0/0",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := deleteDefaultGWCacheBytes(origResult, false, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(test100ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test100ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 1.0.0 Result
			type CNICacheResult100 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult100{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(5))
		})

		It("verify ipv4 default gateway is added to CNI 0.1.0/0.2.0 results without routes", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "dns": {},
    "ip4": {
      "ip": "10.1.1.103/24"
    },
    "ip6": {
      "ip": "10::1:1:103/64",
      "routes": [
        {
          "dst": "20::0:0:0/56",
          "gw": "10::1:1:1"
        },
        {
          "dst": "::0/0",
          "gw": "10::1:1:1"
        },
        {
          "dst": "30::0:0:0/64",
          "gw": "10::1:1:1"
        }
      ]
    }
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10.1.1.1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test020ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test020ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 010/020 Result
			type CNICacheResult020 struct {
				Kind   string `json:"kind"`
				Result struct {
					IP4 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip4"`
					IP6 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip6"`
				} `json:"result"`
			}
			result := CNICacheResult020{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.IP4.Routes)).To(Equal(1))
		})

		It("verify ipv4 default gateway is added to CNI 0.1.0/0.2.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "dns": {},
    "ip4": {
      "ip": "10.1.1.103/24",
      "routes": [
        {
          "dst": "20.0.0.0/24",
          "gw": "10.1.1.1"
        },
        {
          "dst": "30.0.0.0/24",
          "gw": "10.1.1.1"
        }
      ]
    },
    "ip6": {
      "ip": "10::1:1:103/64",
      "routes": [
        {
          "dst": "20::0:0:0/56",
          "gw": "10::1:1:1"
        },
        {
          "dst": "::0/0",
          "gw": "10::1:1:1"
        },
        {
          "dst": "30::0:0:0/64",
          "gw": "10::1:1:1"
        }
      ]
    }
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10.1.1.1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test020ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test020ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 010/020 Result
			type CNICacheResult020 struct {
				Kind   string `json:"kind"`
				Result struct {
					IP4 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip4"`
					IP6 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip6"`
				} `json:"result"`
			}
			result := CNICacheResult020{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.IP4.Routes)).To(Equal(3))
		})

		It("verify ipv6 default gateway is added to CNI 0.1.0/0.2.0 results without routes", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "dns": {},
    "ip4": {
      "ip": "10.1.1.103/24",
      "routes": [
        {
          "dst": "20.0.0.0/24",
          "gw": "10.1.1.1"
        },
        {
          "dst": "0.0.0.0/0",
          "gw": "10.1.1.1"
        },
        {
          "dst": "30.0.0.0/24",
          "gw": "10.1.1.1"
        }
      ]
    },
    "ip6": {
      "ip": "10::1:1:103/64"
    }
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10::1:1:1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test020ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test020ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 010/020 Result
			type CNICacheResult020 struct {
				Kind   string `json:"kind"`
				Result struct {
					IP4 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip4"`
					IP6 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip6"`
				} `json:"result"`
			}
			result := CNICacheResult020{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.IP6.Routes)).To(Equal(1))
		})

		It("verify ipv6 default gateway is added to CNI 0.1.0/0.2.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "dns": {},
    "ip4": {
      "ip": "10.1.1.103/24",
      "routes": [
        {
          "dst": "20.0.0.0/24",
          "gw": "10.1.1.1"
        },
        {
          "dst": "0.0.0.0/0",
          "gw": "10.1.1.1"
        }, 
        {
          "dst": "30.0.0.0/24",
          "gw": "10.1.1.1"
        }
      ]
    },
    "ip6": {
      "ip": "10::1:1:103/64",
      "routes": [
        {
          "dst": "20::0:0:0/56",
          "gw": "10::1:1:1"
        },
        {
          "dst": "30::0:0:0/64",
          "gw": "10::1:1:1"
        }
      ]
    }
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10::1:1:1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test020ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test020ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 010/020 Result
			type CNICacheResult020 struct {
				Kind   string `json:"kind"`
				Result struct {
					IP4 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip4"`
					IP6 struct {
						IP     string `json:"ip"`
						Routes []struct {
							Dst string `json:"dst"`
							Gw  string `json:"gw"`
						} `json:"routes"`
					} `json:"ip6"`
				} `json:"result"`
			}
			result := CNICacheResult020{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.IP6.Routes)).To(Equal(3))
		})

		It("verify ipv4 default gateway is added to CNI 0.3.0/0.3.1/0.4.0 results without routes", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "0.3.1",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0,
        "version": "4"
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0,
        "version": "6"
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10.1.1.1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test040ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test040ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 0.3.0/0.3.1/0.4.0 Result
			type CNICacheResult030_040 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult030_040{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(1))
		})

		It("verify ipv4 default gateway is added to CNI 0.3.0/0.3.1/0.4.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "0.3.1",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0,
        "version": "4"
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0,
        "version": "6"
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10.1.1.1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test040ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test040ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 0.3.0/0.3.1/0.4.0 Result
			type CNICacheResult030_040 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult030_040{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(5))
		})

		It("verify ipv6 default gateway is added to CNI 0.3.0/0.3.1/0.4.0 results without routes", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "0.3.1",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0,
        "version": "4"
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0,
        "version": "6"
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10::1:1:1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test040ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())
			Expect(test040ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 0.3.0/0.3.1/0.4.0 Result
			type CNICacheResult030_040 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult030_040{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(1))
		})

		It("verify ipv6 default gateway is added to CNI 0.3.0/0.3.1/0.4.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "0.3.1",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0,
        "version": "4"
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0,
        "version": "6"
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "0.0.0.0/0",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10::1:1:1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test040ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test040ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 0.3.0/0.3.1/0.4.0 Result
			type CNICacheResult030_040 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult030_040{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(6))
		})

		It("verify ipv4 default gateway is added to CNI 1.0.0 results without routes", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "1.0.0",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10.1.1.1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test100ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test100ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 1.0.0 Result
			type CNICacheResult100 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult100{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(1))
		})

		It("verify ipv4 default gateway is added to CNI 1.0.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "1.0.0",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10.1.1.1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test100ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test100ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())

			// Simplified CNI Cache with 1.0.0 Result
			type CNICacheResult100 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult100{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(5))
		})

		It("verify ipv6 default gateway is added to CNI 1.0.0 results without routes", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "1.0.0",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10::1:1:1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test100ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeFalse())
			Expect(test100ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 1.0.0 Result
			type CNICacheResult100 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult100{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(1))
		})

		It("verify ipv6 default gateway is added to CNI 1.0.0 results", func() {
			origResult := []byte(`{
  "kind": "cniCacheV1",
  "result": {
    "cniVersion": "1.0.0",
    "dns": {},
    "interfaces": [
      {
        "mac": "0a:c2:e6:3d:45:17",
        "name": "net1",
        "sandbox": "/run/netns/bb74fcb9-989a-4589-b2df-ddd0384a8ee5"
      }
    ],
    "ips": [
      {
        "address": "10.1.1.103/24",
        "interface": 0
      },
      {
        "address": "10::1:1:103/64",
        "interface": 0
      }
    ],
    "routes": [
      {
        "dst": "20.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "30.0.0.0/24",
        "gw": "10.1.1.1"
      },
      {
        "dst": "0.0.0.0/0",
        "gw": "10.1.1.1"
      },
      {
        "dst": "20::0:0:0/56",
        "gw": "10::1:1:1"
      },
      {
        "dst": "30::0:0:0/64",
        "gw": "10::1:1:1"
      }
    ]
  }
}`)
			newResult, err := addDefaultGWCacheBytes(origResult, []net.IP{net.ParseIP("10::1:1:1")})
			Expect(err).NotTo(HaveOccurred())

			Expect(test100ResultHasIPv4DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())
			Expect(test100ResultHasIPv6DefaultRoute(testGetResultFromCache(newResult))).To(BeTrue())

			// Simplified CNI Cache with 1.0.0 Result
			type CNICacheResult100 struct {
				Kind   string `json:"kind"`
				Result struct {
					Routes []struct {
						Dst string `json:"dst"`
						Gw  string `json:"gw"`
					} `json:"routes"`
				} `json:"result"`
			}
			result := CNICacheResult100{}
			Expect(json.Unmarshal(newResult, &result)).NotTo(HaveOccurred())
			Expect(len(result.Result.Routes)).To(Equal(6))
		})

	})
})

var _ = Describe("other function unit testing", func() {
	It("deleteDefaultGWResultRoutes with invalid config", func() {
		cniRouteConfig := []byte(`[
		{ "dst": "0.0.0.0/0", "gw": "10.1.1.1" },
		{ "dst": "10.1.1.0/24" },
		{ "dst": "0.0.0.0/0", "gw": "10.1.1.1" }
		]`)

		var routes []interface{}
		err := json.Unmarshal(cniRouteConfig, &routes)
		Expect(err).NotTo(HaveOccurred())

		newRoute, err := deleteDefaultGWResultRoutes(routes, "0.0.0.0/0")
		Expect(err).NotTo(HaveOccurred())
		routeJSON, err := json.Marshal(newRoute)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeJSON).Should(MatchJSON(`[{"dst":"10.1.1.0/24"}]`))
	})
})
