// Copyright (c) 2018 Intel Corporation
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

package logging

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging")
}

var _ = Describe("logging operations", func() {

	BeforeEach(func() {
		loggingStderr = false
		loggingFp = nil
		loggingLevel = PanicLevel
	})

	It("Check file setter with empty", func() {
		SetLogFile("")
		Expect(loggingFp).To(BeNil())
	})

	It("Check file setter with empty", func() {
		SetLogFile("/tmp/foobar.logging")
		Expect(loggingFp).NotTo(Equal(nil))
		// check file existance
	})

	It("Check file setter with bad filepath", func() {
		SetLogFile("/invalid/filepath")
		Expect(loggingFp).NotTo(Equal(nil))
		// check file existance
	})

	It("Check loglevel setter", func() {
		SetLogLevel("debug")
		Expect(loggingLevel).To(Equal(DebugLevel))
		SetLogLevel("Error")
		Expect(loggingLevel).To(Equal(ErrorLevel))
		SetLogLevel("VERbose")
		Expect(loggingLevel).To(Equal(VerboseLevel))
		SetLogLevel("PANIC")
		Expect(loggingLevel).To(Equal(PanicLevel))
	})

	It("Check loglevel setter with invalid level", func() {
		currentLevel := loggingLevel
		SetLogLevel("XXXX")
		Expect(loggingLevel).To(Equal(currentLevel))
	})

	It("Check log to stderr setter with invalid level", func() {
		currentVal := loggingStderr
		SetLogStderr(!currentVal)
		Expect(loggingStderr).NotTo(Equal(currentVal))
	})

	// Tests public getter
	It("Check getter for logging level with current level", func() {
		currentLevel := loggingLevel
		Expect(currentLevel).To(Equal(GetLoggingLevel()))
	})

	It("Detects a known error", func() {
		newerror := Errorf("Testing 123", fmt.Errorf("error dialing DHCP daemon: dial unix /run/cni/dhcp.sock: connect: no such file or directory"))
		Expect(newerror.Error()).To(ContainSubstring("please check that the dhcp cni daemon is running and is properly configured."))
	})

	It("Properly errors when an error message is not set for a pattern", func() {
		_, err := getKnownErrorMessage("intentionally unset error pattern")
		Expect(err).To(HaveOccurred())
	})

	It("Has a message set for each error pattern that is set", func() {
		// If this fails, it probably means you added the error pattern, but, not the error message.
		for _, errorpattern := range knownErrorPatterns {
			message, err := getKnownErrorMessage(errorpattern)
			Expect(message).NotTo(HaveLen(0))
			Expect(err).NotTo(HaveOccurred())
		}
	})

})
