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
	"errors"
	"testing"

	"github.com/containernetworking/cni/pkg/invoke"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type metricsExecFake struct {
	err     error
	findErr error
}

var _ invoke.Exec = &metricsExecFake{}

func (e *metricsExecFake) ExecPlugin(_ context.Context, _ string, _ []byte, _ []string) ([]byte, error) {
	return []byte("{}"), e.err
}

func (e *metricsExecFake) FindInPath(_ string, _ []string) (string, error) {
	return "", e.findErr
}

func (e *metricsExecFake) Decode(_ []byte) (version.PluginInfo, error) {
	return nil, nil
}

func TestMetricsExec(t *testing.T) {
	g := gomega.NewWithT(t)
	metrics := newTestMetrics()
	failure := errors.New("plugin failed")

	_, err := newMetricsExec(&metricsExecFake{}, metrics).ExecPlugin(
		context.Background(), "/opt/cni/bin/sriov", nil, []string{"CNI_COMMAND=ADD"},
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = newMetricsExec(&metricsExecFake{err: failure}, metrics).ExecPlugin(
		context.Background(), "/opt/cni/bin/sriov", nil, []string{"CNI_COMMAND=ADD"},
	)
	g.Expect(err).To(gomega.MatchError(failure))

	_, err = newMetricsExec(&metricsExecFake{}, metrics).ExecPlugin(
		context.Background(), "/opt/cni/bin/sriov", nil, nil,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = newMetricsExec(&metricsExecFake{}, metrics).FindInPath("sriov", nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = newMetricsExec(&metricsExecFake{findErr: failure}, metrics).FindInPath("sriov", nil)
	g.Expect(err).To(gomega.MatchError(failure))

	g.Expect(counterValue(g, metrics.cniExecCounter.WithLabelValues("sriov", "ADD", metricOutcomeSuccess))).To(gomega.Equal(1.0))
	g.Expect(counterValue(g, metrics.cniExecCounter.WithLabelValues("sriov", "ADD", metricOutcomeFailure))).To(gomega.Equal(1.0))
	g.Expect(counterValue(g, metrics.cniExecCounter.WithLabelValues("sriov", unknownCNICommand, metricOutcomeSuccess))).To(gomega.Equal(1.0))
	g.Expect(counterValue(g, metrics.cniLookupCounter.WithLabelValues("sriov", metricOutcomeSuccess))).To(gomega.Equal(1.0))
	g.Expect(counterValue(g, metrics.cniLookupCounter.WithLabelValues("sriov", metricOutcomeFailure))).To(gomega.Equal(1.0))
	g.Expect(histogramSampleCount(g, metrics.cniExecDuration.WithLabelValues("sriov", "ADD"))).To(gomega.Equal(uint64(2)))
}

func newTestMetrics() *Metrics {
	return &Metrics{
		cniLookupCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_cni_plugin_lookup_total"},
			[]string{"plugin", "outcome"},
		),
		cniExecCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_cni_plugin_execution_total"},
			[]string{"plugin", "command", "outcome"},
		),
		cniExecDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "test_cni_plugin_execution_duration_seconds"},
			[]string{"plugin", "command"},
		),
	}
}

func counterValue(g gomega.Gomega, counter prometheus.Counter) float64 {
	metric := &dto.Metric{}
	g.Expect(counter.Write(metric)).To(gomega.Succeed())
	return metric.GetCounter().GetValue()
}

func histogramSampleCount(g gomega.Gomega, observer prometheus.Observer) uint64 {
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		g.Expect(ok).To(gomega.BeTrue())
		return 0
	}

	dtoMetric := &dto.Metric{}
	g.Expect(metric.Write(dtoMetric)).To(gomega.Succeed())
	return dtoMetric.GetHistogram().GetSampleCount()
}
