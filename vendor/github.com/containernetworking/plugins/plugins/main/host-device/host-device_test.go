// Copyright 2017 CNI authors
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

package main

import (
	"fmt"
	"math/rand"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

var _ = Describe("base functionality", func() {
	var originalNS ns.NetNS
	var ifname string

	BeforeEach(func() {
		var err error
		originalNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())

		ifname = fmt.Sprintf("dummy-%x", rand.Int31())
	})

	AfterEach(func() {
		originalNS.Close()
	})

	It("Works with a valid config", func() {
		var origLink netlink.Link

		// prepare ifname in original namespace
		err := originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			err := netlink.LinkAdd(&netlink.Dummy{
				LinkAttrs: netlink.LinkAttrs{
					Name: ifname,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			origLink, err = netlink.LinkByName(ifname)
			Expect(err).NotTo(HaveOccurred())
			err = netlink.LinkSetUp(origLink)
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		// call CmdAdd
		targetNS, err := testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())

		CNI_IFNAME := "eth0"
		conf := fmt.Sprintf(`{
			"cniVersion": "0.3.0",
			"name": "cni-plugin-host-device-test",
			"type": "host-device",
			"device": %q
		}`, ifname)
		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       targetNS.Path(),
			IfName:      CNI_IFNAME,
			StdinData:   []byte(conf),
		}
		var resI types.Result
		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			var err error
			resI, _, err = testutils.CmdAddWithArgs(args, func() error { return cmdAdd(args) })
			return err
		})
		Expect(err).NotTo(HaveOccurred())

		// check that the result was sane
		res, err := current.NewResultFromResult(resI)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Interfaces).To(Equal([]*current.Interface{
			{
				Name:    CNI_IFNAME,
				Mac:     origLink.Attrs().HardwareAddr.String(),
				Sandbox: targetNS.Path(),
			},
		}))

		// assert that dummy0 is now in the target namespace
		err = targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			link, err := netlink.LinkByName(CNI_IFNAME)
			Expect(err).NotTo(HaveOccurred())
			Expect(link.Attrs().HardwareAddr).To(Equal(origLink.Attrs().HardwareAddr))
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		// assert that dummy0 is now NOT in the original namespace anymore
		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			_, err := netlink.LinkByName(ifname)
			Expect(err).To(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		// Check that deleting the device moves it back and restores the name
		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			err = testutils.CmdDelWithArgs(args, func() error {
				return cmdDel(args)
			})
			Expect(err).NotTo(HaveOccurred())

			_, err := netlink.LinkByName(ifname)
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

	})

	It("fails an invalid config", func() {
		conf := `{
			"cniVersion": "0.3.0",
			"name": "cni-plugin-host-device-test",
			"type": "host-device"
		}`

		args := &skel.CmdArgs{
			ContainerID: "dummy",
			Netns:       originalNS.Path(),
			IfName:      ifname,
			StdinData:   []byte(conf),
		}
		_, _, err := testutils.CmdAddWithArgs(args, func() error { return cmdAdd(args) })
		Expect(err).To(MatchError(`specify either "device", "hwaddr" or "kernelpath"`))

	})

})
