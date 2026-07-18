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
	// Keep the serial encoder and response writer on one OS thread so the
	// thread CPU clock is scoped to the timed operation.
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()

	codecs, list := newStreamingPodListBenchmark(b)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/pods", nil)
	req.Header.Set("Accept", "application/json")
	expectedLength := writeStreamingPodListToRecorder(b, codecs, list, req)

	startCPU, err := currentThreadCPUTime()
	if err != nil {
		b.Fatalf("read start thread CPU time: %v", err)
	}
	operations := 0
	for b.Loop() {
		if got := writeStreamingPodListToRecorder(b, codecs, list, req); got != expectedLength {
			b.Fatalf("response length = %d, want %d", got, expectedLength)
		}
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
