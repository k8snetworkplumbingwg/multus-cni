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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
})
