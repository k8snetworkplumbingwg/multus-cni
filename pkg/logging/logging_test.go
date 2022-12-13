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

package logging

import (
	"fmt"
	"os"
	"testing"

	testutils "gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/testing"
	"gopkg.in/natefinch/lumberjack.v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging")
}

var _ = Describe("logging operations", func() {

	BeforeEach(func() {
		loggingStderr = false
		loggingW = nil
		loggingLevel = PanicLevel
	})

	It("Check file setter with empty", func() {
		SetLogFile("")
		Expect(loggingW).To(BeNil())
	})

	It("Check file setter with empty", func() {
		SetLogFile("/tmp/foobar.logging")
		Expect(loggingW).NotTo(Equal(nil))
		// check file existence
	})

	It("Check file setter with bad filepath", func() {
		SetLogFile("/invalid/filepath")
		Expect(loggingW).NotTo(Equal(nil))
		// check file existence
	})

	It("Check loglevel setter", func() {
		SetLogLevel("debug")
		Expect(loggingLevel).To(Equal(DebugLevel))
		Expect(loggingLevel.String()).To(Equal("debug"))
		SetLogLevel("Error")
		Expect(loggingLevel).To(Equal(ErrorLevel))
		Expect(loggingLevel.String()).To(Equal("error"))
		SetLogLevel("VERbose")
		Expect(loggingLevel).To(Equal(VerboseLevel))
		Expect(loggingLevel.String()).To(Equal("verbose"))
		SetLogLevel("PANIC")
		Expect(loggingLevel).To(Equal(PanicLevel))
		Expect(loggingLevel.String()).To(Equal("panic"))
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

	It("Check log function is worked", func() {
		Debugf("foobar")
		Verbosef("foobar")
		Expect(Errorf("foobar")).NotTo(BeNil())
		Panicf("foobar")
	})

	It("Check log function is worked with stderr", func() {
		SetLogStderr(true)
		Debugf("foobar")
		Verbosef("foobar")
		Expect(Errorf("foobar")).NotTo(BeNil())
		Panicf("foobar")
	})

	It("Check log function is worked with stderr", func() {
		tmpDir, err := os.MkdirTemp("", "multus_tmp")
		SetLogFile(fmt.Sprintf("%s/log.txt", tmpDir))
		Debugf("foobar")
		Verbosef("foobar")
		Expect(Errorf("foobar")).NotTo(BeNil())
		Panicf("foobar")
		logger.Filename = ""
		loggingW = nil
		err = os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		// Revert the log variable to init
		loggingW = nil
		logger = &lumberjack.Logger{}
	})

	// Tests public getter
	It("Check getter for logging level with current level", func() {
		currentLevel := loggingLevel
		Expect(currentLevel).To(Equal(GetLoggingLevel()))
	})

	It("Check user settings logOptions for logging", func() {
		SetLogFile("/var/log/multus.log")
		expectLogger := &lumberjack.Logger{
			Filename:   "/var/log/multus.log",
			MaxAge:     1,
			MaxSize:    10,
			MaxBackups: 1,
			Compress:   true,
		}
		logOptions := &LogOptions{
			MaxAge:     testutils.Int(1),
			MaxSize:    testutils.Int(10),
			MaxBackups: testutils.Int(1),
			Compress:   testutils.Bool(true),
		}
		SetLogOptions(logOptions)
		Expect(expectLogger).To(Equal(logger))
	})

	It("Check user settings logOptions and missing some options", func() {
		SetLogFile("/var/log/multus.log")
		expectLogger := &lumberjack.Logger{
			Filename:   "/var/log/multus.log",
			MaxAge:     5,
			MaxSize:    100,
			MaxBackups: 1,
			Compress:   true,
		}
		logOptions := &LogOptions{
			MaxBackups: testutils.Int(1),
			Compress:   testutils.Bool(true),
		}
		SetLogOptions(logOptions)
		Expect(expectLogger).To(Equal(logger))
	})

	It("Check user don't settings logOptions for logging", func() {
		SetLogFile("/var/log/multus.log")
		logger1 := &lumberjack.Logger{
			Filename:   "/var/log/multus.log",
			MaxAge:     5,
			MaxSize:    100,
			MaxBackups: 5,
			Compress:   true,
		}
		SetLogOptions(nil)
		Expect(logger1).To(Equal(logger))
	})

})
