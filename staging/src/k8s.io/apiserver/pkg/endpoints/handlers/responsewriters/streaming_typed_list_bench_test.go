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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
)

func BenchmarkWriteObjectNegotiatedStreamingPodList(b *testing.B) {
	codecs, lists := newStreamingPodListBenchmark(b)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := 0
		if r.URL.Query().Get("variant") == "1" {
			index = 1
		}
		writeStreamingPodListResponse(codecs, lists[index], w, r)
	}))
	defer server.Close()

	client := server.Client()
	urls := []string{server.URL + "?variant=0", server.URL + "?variant=1"}
	expectedLengths := make([]int64, len(urls))
	for i, url := range urls {
		expectedLengths[i] = requestStreamingPodList(b, client, url)
	}

	index := 0
	for b.Loop() {
		if got, want := requestStreamingPodList(b, client, urls[index]), expectedLengths[index]; got != want {
			b.Fatalf("response %d length = %d, want %d", index, got, want)
		}
		index = (index + 1) % len(urls)
	}
}

func newStreamingPodListBenchmark(b *testing.B) (runtime.NegotiatedSerializer, []*v1.PodList) {
	b.Helper()

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		b.Fatalf("add core/v1 to scheme: %v", err)
	}
	codecs := serializer.NewCodecFactory(scheme, serializer.WithStreamingCollectionEncodingToJSON())
	lists := []*v1.PodList{
		benchmarkItems(b, 1000),
		benchmarkItems(b, 1000),
	}
	lists[0].ResourceVersion = "100"
	lists[1].ResourceVersion = "101"
	return codecs, lists
}

func writeStreamingPodListResponse(codecs runtime.NegotiatedSerializer, list *v1.PodList, w http.ResponseWriter, req *http.Request) {
	WriteObjectNegotiated(
		codecs,
		negotiation.DefaultEndpointRestrictions,
		schema.GroupVersion{Version: "v1"},
		w,
		req,
		http.StatusOK,
		list,
		false,
	)
}

func requestStreamingPodList(b *testing.B, client *http.Client, url string) int64 {
	b.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		b.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		b.Fatalf("send request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		b.Fatalf("response status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got, want := resp.Header.Get("Content-Type"), "application/json"; got != want {
		resp.Body.Close()
		b.Fatalf("response content type = %q, want %q", got, want)
	}
	length, err := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		b.Fatalf("read response: %v", err)
	}
	if closeErr != nil {
		b.Fatalf("close response: %v", closeErr)
	}
	return length
}
