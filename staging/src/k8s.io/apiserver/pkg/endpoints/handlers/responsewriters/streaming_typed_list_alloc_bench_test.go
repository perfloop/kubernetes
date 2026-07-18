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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

func BenchmarkWriteObjectNegotiatedStreamingPodListAllocs(b *testing.B) {
	codecs, list := newStreamingPodListAllocationBenchmark(b)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/pods", nil)
	req.Header.Set("Accept", "application/json")
	expectedLength := writeStreamingPodListToRecorder(b, codecs, list, req)

	b.ReportAllocs()
	for b.Loop() {
		if got := writeStreamingPodListToRecorder(b, codecs, list, req); got != expectedLength {
			b.Fatalf("response length = %d, want %d", got, expectedLength)
		}
	}
}

func newStreamingPodListAllocationBenchmark(b *testing.B) (runtime.NegotiatedSerializer, *v1.PodList) {
	b.Helper()

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		b.Fatalf("add core/v1 to scheme: %v", err)
	}
	codecs := serializer.NewCodecFactory(scheme, serializer.WithStreamingCollectionEncodingToJSON())
	list := benchmarkItems(b, 1000)
	list.ResourceVersion = "100"
	return codecs, list
}
