// Copyright (c) 2026 Multus Authors
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

package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/version"
)

const (
	unknownCNICommand    = "UNKNOWN"
	metricOutcomeSuccess = "success"
	metricOutcomeFailure = "failure"
)

// metricsExec records CNI plugin lookup and execution metrics for the daemon.
type metricsExec struct {
	exec    invoke.Exec
	metrics *Metrics
}

var _ invoke.Exec = &metricsExec{}

func newMetricsExec(exec invoke.Exec, metrics *Metrics) invoke.Exec {
	if exec == nil {
		exec = &invoke.DefaultExec{
			RawExec: &invoke.RawExec{Stderr: os.Stderr},
		}
	}

	return &metricsExec{
		exec:    exec,
		metrics: metrics,
	}
}

func (e *metricsExec) ExecPlugin(ctx context.Context, pluginPath string, stdinData []byte, environ []string) ([]byte, error) {
	plugin := filepath.Base(pluginPath)
	command := cniCommand(environ)
	start := time.Now()
	stdout, err := e.exec.ExecPlugin(ctx, pluginPath, stdinData, environ)

	outcome := metricOutcomeSuccess
	if err != nil {
		outcome = metricOutcomeFailure
	}
	e.metrics.cniExecCounter.WithLabelValues(plugin, command, outcome).Inc()
	e.metrics.cniExecDuration.WithLabelValues(plugin, command).Observe(time.Since(start).Seconds())

	return stdout, err
}

func (e *metricsExec) FindInPath(plugin string, paths []string) (string, error) {
	pluginPath, err := e.exec.FindInPath(plugin, paths)

	outcome := metricOutcomeSuccess
	if err != nil {
		outcome = metricOutcomeFailure
	}
	e.metrics.cniLookupCounter.WithLabelValues(plugin, outcome).Inc()

	return pluginPath, err
}

func (e *metricsExec) Decode(jsonBytes []byte) (version.PluginInfo, error) {
	return e.exec.Decode(jsonBytes)
}

func cniCommand(environ []string) string {
	for _, entry := range environ {
		command, found := strings.CutPrefix(entry, "CNI_COMMAND=")
		if found {
			return command
		}
	}
	return unknownCNICommand
}
