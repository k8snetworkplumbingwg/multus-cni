// Copyright (c) 2018 Intel Corporation
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

package types

//revive:disable:dot-imports
import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests cover parsing of net-attach-def config shapes that a namespaced
// tenant can author. Malformed shapes must produce an error, never a panic.
var _ = Describe("LoadDelegateNetConf robustness against malformed net-attach-def config", func() {
	It("returns an error (does not panic) for a conflist with an empty plugins array", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"empty","plugins":[]}`)
		Expect(func() {
			_, err := LoadDelegateNetConf(conf, nil, "", "")
			Expect(err).To(HaveOccurred())
		}).NotTo(Panic())
	})

	It("returns an error (does not panic) when 'args' is a scalar instead of an object", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"scalarargs","type":"mynet","args":"not-an-object"}`)
		cniArgs := &map[string]interface{}{"K8S_POD_NAME": "p"}
		netElement := &NetworkSelectionElement{CNIArgs: cniArgs}
		Expect(func() {
			_, err := LoadDelegateNetConf(conf, netElement, "", "")
			Expect(err).To(HaveOccurred())
		}).NotTo(Panic())
	})

	It("returns an error (does not panic) when 'args.cni' is a scalar instead of an object", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"scalarcni","type":"mynet","args":{"cni":"not-an-object"}}`)
		cniArgs := &map[string]interface{}{"K8S_POD_NAME": "p"}
		netElement := &NetworkSelectionElement{CNIArgs: cniArgs}
		Expect(func() {
			_, err := LoadDelegateNetConf(conf, netElement, "", "")
			Expect(err).To(HaveOccurred())
		}).NotTo(Panic())
	})

	It("returns an error (does not panic) for a conflist whose plugin entry is a scalar", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"badplugin","plugins":["not-an-object"]}`)
		cniArgs := &map[string]interface{}{"K8S_POD_NAME": "p"}
		netElement := &NetworkSelectionElement{CNIArgs: cniArgs}
		Expect(func() {
			_, err := LoadDelegateNetConf(conf, netElement, "", "")
			Expect(err).To(HaveOccurred())
		}).NotTo(Panic())
	})

	It("returns an error (does not panic) for a conflist whose first plugin is null", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"nullplugin","plugins":[null]}`)
		Expect(func() {
			_, err := LoadDelegateNetConf(conf, nil, "", "")
			Expect(err).To(HaveOccurred())
		}).NotTo(Panic())
	})

	It("still accepts a well-formed 'args.cni' object", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"okargs","type":"mynet","args":{"cni":{"existing":"v"}}}`)
		cniArgs := &map[string]interface{}{"added": "v2"}
		netElement := &NetworkSelectionElement{CNIArgs: cniArgs}
		delegate, err := LoadDelegateNetConf(conf, netElement, "", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(delegate).NotTo(BeNil())
	})

	It("wraps injectCNIArgs error with %w in addCNIArgsInConfig and addCNIArgsInConfList", func() {
		conf := []byte(`{"cniVersion":"0.3.1","name":"badargs","type":"mynet","args":"not-an-object"}`)
		cniArgs := &map[string]interface{}{"K8S_POD_NAME": "p"}
		_, err := addCNIArgsInConfig(conf, cniArgs)
		Expect(err).To(HaveOccurred())
		Expect(errors.Unwrap(err)).NotTo(BeNil())

		conflist := []byte(`{"cniVersion":"0.3.1","name":"badplugin","plugins":[{"type":"mynet","args":"not-an-object"}]}`)
		_, err = addCNIArgsInConfList(conflist, cniArgs)
		Expect(err).To(HaveOccurred())
		Expect(errors.Unwrap(err)).NotTo(BeNil())
	})
})
