//go:build linux

/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package responsewriters

import (
	"net/http"
	"net/http/httptest"
	goruntime "runtime"
	"testing"

	"golang.org/x/sys/unix"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func BenchmarkWriteObjectNegotiatedStreamingPodListCPU(b *testing.B) {
	// This CPU measurement deliberately uses the direct response-writer path:
	// unlike the HTTP latency benchmark, no client or server goroutine can
	// share a process-wide counter with the operation under test.
	oldGOMAXPROCS := goruntime.GOMAXPROCS(1)
	defer goruntime.GOMAXPROCS(oldGOMAXPROCS)
	// Keep the serial encoder and response writer on one OS thread so the
	// thread CPU clock is scoped to the timed operation.
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	codecs, lists := newStreamingPodListBenchmark(b)
	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/pods?variant=0", nil),
		httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/pods?variant=1", nil),
	}
	for _, req := range requests {
		req.Header.Set("Accept", "application/json")
	}

	expectedLengths := make([]int, len(requests))
	for i, req := range requests {
		expectedLengths[i] = writeStreamingPodListToRecorder(b, codecs, lists[i], req)
	}

	startCPU, err := currentThreadCPUTime()
	if err != nil {
		b.Fatalf("read start thread CPU time: %v", err)
	}
	index := 0
	operations := 0
	for b.Loop() {
		if got, want := writeStreamingPodListToRecorder(b, codecs, lists[index], requests[index]), expectedLengths[index]; got != want {
			b.Fatalf("response %d length = %d, want %d", index, got, want)
		}
		index = (index + 1) % len(lists)
		operations++
	}
	endCPU, err := currentThreadCPUTime()
	if err != nil {
		b.Fatalf("read end thread CPU time: %v", err)
	}
	b.ReportMetric(float64(endCPU-startCPU)/float64(operations), "cpu-ns/op")
}

func writeStreamingPodListToRecorder(b *testing.B, codecs runtime.NegotiatedSerializer, list *v1.PodList, req *http.Request) int {
	b.Helper()

	recorder := httptest.NewRecorder()
	writeStreamingPodListResponse(codecs, list, recorder, req)
	if got, want := recorder.Code, http.StatusOK; got != want {
		b.Fatalf("response status = %d, want %d", got, want)
	}
	if got, want := recorder.Header().Get("Content-Type"), "application/json"; got != want {
		b.Fatalf("response content type = %q, want %q", got, want)
	}
	return recorder.Body.Len()
}

func currentThreadCPUTime() (int64, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &ts); err != nil {
		return 0, err
	}
	return ts.Sec*1e9 + ts.Nsec, nil
}
