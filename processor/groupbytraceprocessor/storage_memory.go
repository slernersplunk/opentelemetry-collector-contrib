// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package groupbytraceprocessor

import (
	"context"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opentelemetry.io/collector/consumer/pdata"
)

type memoryStorage struct {
	sync.RWMutex
	content                   map[string][]pdata.ResourceSpans
	stopped                   bool
	stoppedLock               sync.RWMutex
	metricsCollectionInterval time.Duration
}

var _ storage = (*memoryStorage)(nil)

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{
		content:                   make(map[string][]pdata.ResourceSpans),
		metricsCollectionInterval: time.Second,
	}
}

func (st *memoryStorage) createOrAppend(traceID pdata.TraceID, rs pdata.ResourceSpans) error {
	sTraceID := traceID.HexString()

	st.Lock()
	defer st.Unlock()

	if _, ok := st.content[sTraceID]; !ok {
		st.content[sTraceID] = []pdata.ResourceSpans{}
	}

	newRS := pdata.NewResourceSpans()
	rs.CopyTo(newRS)
	st.content[sTraceID] = append(st.content[sTraceID], newRS)

	return nil
}
func (st *memoryStorage) get(traceID pdata.TraceID) ([]pdata.ResourceSpans, error) {
	sTraceID := traceID.HexString()

	st.RLock()
	defer st.RUnlock()

	rss, ok := st.content[sTraceID]
	if !ok {
		return nil, nil
	}

	var result []pdata.ResourceSpans
	for _, rs := range rss {
		newRS := pdata.NewResourceSpans()
		rs.CopyTo(newRS)
		result = append(result, newRS)
	}

	return result, nil
}

// delete will return a reference to a ResourceSpans. Changes to the returned object may not be applied
// to the version in the storage.
func (st *memoryStorage) delete(traceID pdata.TraceID) ([]pdata.ResourceSpans, error) {
	sTraceID := traceID.HexString()

	st.Lock()
	defer st.Unlock()

	rss := st.content[sTraceID]
	var result []pdata.ResourceSpans
	for _, rs := range rss {
		newRS := pdata.NewResourceSpans()
		rs.CopyTo(newRS)
		result = append(result, newRS)
	}
	delete(st.content, sTraceID)

	return result, nil
}

func (st *memoryStorage) start() error {
	go st.periodicMetrics()
	return nil
}

func (st *memoryStorage) shutdown() error {
	st.stoppedLock.Lock()
	defer st.stoppedLock.Unlock()
	st.stopped = true
	return nil
}

func (st *memoryStorage) periodicMetrics() {
	numTraces := st.count()
	stats.Record(context.Background(), mNumTracesInMemory.M(int64(numTraces)))

	st.stoppedLock.RLock()
	stopped := st.stopped
	st.stoppedLock.RUnlock()
	if stopped {
		return
	}

	time.AfterFunc(st.metricsCollectionInterval, func() {
		st.periodicMetrics()
	})
}

func (st *memoryStorage) count() int {
	st.RLock()
	defer st.RUnlock()
	return len(st.content)
}
