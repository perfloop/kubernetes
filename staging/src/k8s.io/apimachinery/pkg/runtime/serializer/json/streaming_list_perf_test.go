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

package json

import (
	"bytes"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testapigroupv1 "k8s.io/apimachinery/pkg/apis/testapigroup/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestStreamingCollectionsItemConversions(t *testing.T) {
	normal := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{})
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})

	tests := []struct {
		name            string
		list            runtime.Object
		expectStreaming bool
	}{
		{
			name: "nil items",
			list: &testapigroupv1.CarpList{
				TypeMeta: metav1.TypeMeta{Kind: "CarpList", APIVersion: "testapigroup.k8s.io/v1"},
				Items:    nil,
			},
			expectStreaming: true,
		},
		{
			name: "empty items",
			list: &testapigroupv1.CarpList{
				TypeMeta: metav1.TypeMeta{Kind: "CarpList", APIVersion: "testapigroup.k8s.io/v1"},
				Items:    []testapigroupv1.Carp{},
			},
			expectStreaming: true,
		},
		{
			name: "raw extension object and raw",
			list: &streamingRawExtensionList{
				Items: []runtime.RawExtension{
					{Object: &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "object"}}},
					{Raw: []byte(`{"metadata":{"name":"raw"}}`)},
					{},
				},
			},
			expectStreaming: true,
		},
		{
			name: "invalid item type falls back before writing",
			list: &streamingInvalidItemsList{
				Items: []string{"not a runtime object"},
			},
			expectStreaming: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var expected bytes.Buffer
			if err := normal.Encode(tc.list, &expected); err != nil {
				t.Fatalf("normal encoder: %v", err)
			}

			var actual writeCountingBuffer
			if err := streaming.Encode(tc.list, &actual); err != nil {
				t.Fatalf("streaming encoder: %v", err)
			}
			if !bytes.Equal(actual.Bytes(), expected.Bytes()) {
				t.Errorf("streaming output = %q, want %q", actual.String(), expected.String())
			}
			if tc.expectStreaming && actual.writeCount <= 1 {
				t.Errorf("streaming encoder wrote %d times, want more than one", actual.writeCount)
			}
			if !tc.expectStreaming && actual.writeCount != 1 {
				t.Errorf("fallback encoder wrote %d times, want one", actual.writeCount)
			}
		})
	}
}

type streamingRawExtensionList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []runtime.RawExtension `json:"items"`
}

func (*streamingRawExtensionList) DeepCopyObject() runtime.Object { return nil }

type streamingInvalidItemsList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []string `json:"items"`
}

func (*streamingInvalidItemsList) DeepCopyObject() runtime.Object { return nil }

func BenchmarkSerializerEncodeStreamingCarpList(b *testing.B) {
	for _, itemCount := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("Items%d", itemCount), func(b *testing.B) {
			streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})
			normal := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{})
			lists := []*testapigroupv1.CarpList{
				benchmarkCarpList(itemCount, "benchmark-a"),
				benchmarkCarpList(itemCount, "benchmark-b"),
			}
			expected := make([][]byte, len(lists))
			for i, list := range lists {
				var encoded bytes.Buffer
				if err := normal.Encode(list, &encoded); err != nil {
					b.Fatalf("normal encoder: %v", err)
				}
				expected[i] = bytes.Clone(encoded.Bytes())
			}

			var encoded bytes.Buffer
			var lastExpected []byte
			i := 0
			b.ReportAllocs()
			for b.Loop() {
				listIndex := i & 1
				encoded.Reset()
				if err := streaming.Encode(lists[listIndex], &encoded); err != nil {
					b.Fatalf("streaming encoder: %v", err)
				}
				lastExpected = expected[listIndex]
				i++
			}
			if !bytes.Equal(encoded.Bytes(), lastExpected) {
				b.Fatalf("streaming output = %q, want %q", encoded.String(), lastExpected)
			}
		})
	}
}

func benchmarkCarpList(itemCount int, resourceVersion string) *testapigroupv1.CarpList {
	list := &testapigroupv1.CarpList{
		TypeMeta: metav1.TypeMeta{Kind: "CarpList", APIVersion: "testapigroup.k8s.io/v1"},
		ListMeta: metav1.ListMeta{ResourceVersion: resourceVersion},
		Items:    make([]testapigroupv1.Carp, itemCount),
	}
	for i := range list.Items {
		list.Items[i].ObjectMeta.Name = "pod"
		list.Items[i].ObjectMeta.Namespace = "default"
	}
	return list
}
