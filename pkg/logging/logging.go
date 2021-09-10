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
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Level type
type Level uint32

// PanicLevel...MaxLevel indicates the logging level
const (
	PanicLevel Level = iota
	ErrorLevel
	VerboseLevel
	DebugLevel
	MaxLevel
	UnknownLevel
)

var loggingStderr bool
var loggingW io.Writer
var loggingLevel Level

const defaultTimestampFormat = time.RFC3339

func (l Level) String() string {
	switch l {
	case PanicLevel:
		return "panic"
	case VerboseLevel:
		return "verbose"
	case ErrorLevel:
		return "error"
	case DebugLevel:
		return "debug"
	}
	return "unknown"
}

func printf(level Level, format string, a ...interface{}) {
	header := "%s [%s] "
	t := time.Now()
	if level > loggingLevel {
		return
	}

	if loggingStderr {
		fmt.Fprintf(os.Stderr, header, t.Format(defaultTimestampFormat), level)
		fmt.Fprintf(os.Stderr, format, a...)
		fmt.Fprintf(os.Stderr, "\n")
	}

	if loggingW != nil {
		fmt.Fprintf(loggingW, header, t.Format(defaultTimestampFormat), level)
		fmt.Fprintf(loggingW, format, a...)
		fmt.Fprintf(loggingW, "\n")
	}
}

// Debugf prints logging if logging level >= debug
func Debugf(format string, a ...interface{}) {
	printf(DebugLevel, format, a...)
}

// Verbosef prints logging if logging level >= verbose
func Verbosef(format string, a ...interface{}) {
	printf(VerboseLevel, format, a...)
}

// Errorf prints logging if logging level >= error
func Errorf(format string, a ...interface{}) error {
	printf(ErrorLevel, format, a...)
	return fmt.Errorf(format, a...)
}

// Panicf prints logging plus stack trace. This should be used only for unrecoverable error
func Panicf(format string, a ...interface{}) {
	printf(PanicLevel, format, a...)
	printf(PanicLevel, "========= Stack trace output ========")
	printf(PanicLevel, "%+v", errors.New("Multus Panic"))
	printf(PanicLevel, "========= Stack trace output end ========")
}

// GetLoggingLevel gets current logging level
func GetLoggingLevel() Level {
	return loggingLevel
}

func getLoggingLevel(levelStr string) Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return DebugLevel
	case "verbose":
		return VerboseLevel
	case "error":
		return ErrorLevel
	case "panic":
		return PanicLevel
	}
	fmt.Fprintf(os.Stderr, "multus logging: cannot set logging level to %s\n", levelStr)
	return UnknownLevel
}

// SetLogLevel sets logging level
func SetLogLevel(levelStr string) {
	level := getLoggingLevel(levelStr)
	if level < MaxLevel {
		loggingLevel = level
	}
}

// SetLogStderr sets flag for logging stderr output
func SetLogStderr(enable bool) {
	loggingStderr = enable
}

// SetLogFile sets logging file
func SetLogFile(filename string) {
	if filename == "" {
		return
	}

	loggingW = &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    100, // megabytes
		MaxBackups: 5,
		MaxAge:     5, // days
		Compress:   true,
	}

}

func init() {
	loggingStderr = true
	loggingW = nil
	loggingLevel = PanicLevel
}
