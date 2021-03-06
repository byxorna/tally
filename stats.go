// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tally

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var (
	capabilitiesNone = &capabilities{
		reporting: false,
		tagging:   false,
	}
	capabilitiesReportingNoTagging = &capabilities{
		reporting: true,
		tagging:   false,
	}
	capabilitiesReportingTagging = &capabilities{
		reporting: true,
		tagging:   true,
	}
)

type capabilities struct {
	reporting bool
	tagging   bool
}

func (c *capabilities) Reporting() bool {
	return c.reporting
}

func (c *capabilities) Tagging() bool {
	return c.tagging
}

type counter struct {
	prev int64
	curr int64
}

func newCounter() *counter {
	return &counter{}
}

func (c *counter) Inc(v int64) {
	atomic.AddInt64(&c.curr, v)
}

func (c *counter) report(name string, tags map[string]string, r StatsReporter) {
	curr := atomic.LoadInt64(&c.curr)

	prev := atomic.LoadInt64(&c.prev)
	if prev == curr {
		return
	}
	atomic.StoreInt64(&c.prev, curr)
	r.ReportCounter(name, tags, curr-prev)
}

func (c *counter) snapshot() int64 {
	return atomic.LoadInt64(&c.curr) - atomic.LoadInt64(&c.prev)
}

type gauge struct {
	updated uint64
	curr    uint64
}

func newGauge() *gauge {
	return &gauge{}
}

func (g *gauge) Update(v float64) {
	atomic.StoreUint64(&g.curr, math.Float64bits(v))
	atomic.StoreUint64(&g.updated, 1)
}

func (g *gauge) report(name string, tags map[string]string, r StatsReporter) {
	if atomic.SwapUint64(&g.updated, 0) == 1 {
		r.ReportGauge(name, tags, math.Float64frombits(atomic.LoadUint64(&g.curr)))
	}
}

func (g *gauge) snapshot() float64 {
	return math.Float64frombits(atomic.LoadUint64(&g.curr))
}

// NB(jra3): timers are a little special because they do no aggregate any data
// at the timer level. The reporter buffers may timer entries and periodically
// flushes.
type timer struct {
	name       string
	tags       map[string]string
	reporter   StatsReporter
	unreported timerValues
}

type timerValues struct {
	sync.RWMutex
	values []time.Duration
}

func newTimer(name string, tags map[string]string, r StatsReporter) *timer {
	t := &timer{
		name:     name,
		tags:     tags,
		reporter: r,
	}
	if r == nil {
		t.reporter = &timerNoReporterSink{timer: t}
	}
	return t
}

func (t *timer) Start() Stopwatch {
	return Stopwatch{start: globalClock.Now(), timer: t}
}

func (t *timer) Record(interval time.Duration) {
	t.reporter.ReportTimer(t.name, t.tags, interval)
}

func (t *timer) snapshot() []time.Duration {
	t.unreported.RLock()
	snap := make([]time.Duration, len(t.unreported.values))
	for i := range t.unreported.values {
		snap[i] = t.unreported.values[i]
	}
	t.unreported.RUnlock()
	return snap
}

type timerNoReporterSink struct {
	sync.RWMutex
	timer *timer
}

func (r *timerNoReporterSink) ReportCounter(name string, tags map[string]string, value int64) {
}
func (r *timerNoReporterSink) ReportGauge(name string, tags map[string]string, value float64) {
}
func (r *timerNoReporterSink) ReportTimer(name string, tags map[string]string, interval time.Duration) {
	r.timer.unreported.Lock()
	r.timer.unreported.values = append(r.timer.unreported.values, interval)
	r.timer.unreported.Unlock()
}
func (r *timerNoReporterSink) Capabilities() Capabilities {
	return capabilitiesReportingTagging
}
func (r *timerNoReporterSink) Flush() {
}

// NullStatsReporter is an implementatin of StatsReporter than simply does nothing.
var NullStatsReporter StatsReporter = nullStatsReporter{}

func (r nullStatsReporter) ReportCounter(name string, tags map[string]string, value int64) {
}
func (r nullStatsReporter) ReportGauge(name string, tags map[string]string, value float64) {
}
func (r nullStatsReporter) ReportTimer(name string, tags map[string]string, interval time.Duration) {
}
func (r nullStatsReporter) Capabilities() Capabilities {
	return capabilitiesReportingNoTagging
}
func (r nullStatsReporter) Flush() {
}

type nullStatsReporter struct{}
