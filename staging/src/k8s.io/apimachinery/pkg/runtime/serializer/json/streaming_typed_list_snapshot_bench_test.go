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
	stdjson "encoding/json"
	"math/rand"
	"testing"

	"sigs.k8s.io/randfill"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testapigroupv1 "k8s.io/apimachinery/pkg/apis/testapigroup/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type benchmarkPointerCarpList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []*testapigroupv1.Carp `json:"items"`
}

func (l *benchmarkPointerCarpList) DeepCopyObject() runtime.Object {
	return l
}

type benchmarkRuntimeObjectList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []runtime.Object `json:"items"`
}

func (l *benchmarkRuntimeObjectList) DeepCopyObject() runtime.Object {
	return l
}

type benchmarkRawExtensionList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []runtime.RawExtension `json:"items"`
}

func (l *benchmarkRawExtensionList) DeepCopyObject() runtime.Object {
	return l
}

type valueRuntimeCarp testapigroupv1.Carp

func (valueRuntimeCarp) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (c valueRuntimeCarp) DeepCopyObject() runtime.Object {
	return c
}

type benchmarkValueRuntimeCarpList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []valueRuntimeCarp `json:"items"`
}

func (l *benchmarkValueRuntimeCarpList) DeepCopyObject() runtime.Object {
	return l
}

func BenchmarkStreamEncodeSnapshotPointerCarpList(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{name: "one", count: 1},
		{name: "thousand", count: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			items := snapshotBenchmarkCarps(b, tc.count)
			pointers := make([]*testapigroupv1.Carp, len(items))
			for i := range items {
				pointers[i] = &items[i]
			}
			benchmarkStreamEncodeSnapshotBranch(b, &benchmarkPointerCarpList{Items: pointers})
		})
	}
}

func BenchmarkStreamEncodeSnapshotRuntimeObjectList(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{name: "one", count: 1},
		{name: "thousand", count: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			items := snapshotBenchmarkCarps(b, tc.count)
			objects := make([]runtime.Object, len(items))
			for i := range items {
				objects[i] = &items[i]
			}
			benchmarkStreamEncodeSnapshotBranch(b, &benchmarkRuntimeObjectList{Items: objects})
		})
	}
}

func BenchmarkStreamEncodeSnapshotValueRuntimeObjectList(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{name: "one", count: 1},
		{name: "thousand", count: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			carps := snapshotBenchmarkCarps(b, tc.count)
			items := make([]valueRuntimeCarp, len(carps))
			for i := range carps {
				items[i] = valueRuntimeCarp(carps[i])
			}
			benchmarkStreamEncodeSnapshotBranch(b, &benchmarkValueRuntimeCarpList{Items: items})
		})
	}
}

func BenchmarkStreamEncodeSnapshotRawExtensionList(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{name: "one", count: 1},
		{name: "thousand", count: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			carps := snapshotBenchmarkCarps(b, tc.count)
			items := make([]runtime.RawExtension, len(carps))
			for i := range carps {
				raw, err := stdjson.Marshal(&carps[i])
				if err != nil {
					b.Fatalf("marshal Carp item: %v", err)
				}
				items[i] = runtime.RawExtension{Raw: raw}
			}
			benchmarkStreamEncodeSnapshotBranch(b, &benchmarkRawExtensionList{Items: items})
		})
	}
}

func snapshotBenchmarkCarps(b *testing.B, count int) []testapigroupv1.Carp {
	b.Helper()

	disableFuzzFieldsV1 := func(field *metav1.FieldsV1, c randfill.Continue) {}
	fuzzMap := func(kvs map[string]interface{}, c randfill.Continue) {
		kvs[c.String(0)] = c.Bool()
		kvs[c.String(0)] = c.Uint64()
		kvs[c.String(0)] = c.String(0)
	}
	f := randfill.New().RandSource(rand.NewSource(12345)).Funcs(disableFuzzFieldsV1, fuzzMap)
	items := make([]testapigroupv1.Carp, count)
	for i := range items {
		f.Fill(&items[i])
	}
	return items
}

func benchmarkStreamEncodeSnapshotBranch(b *testing.B, list runtime.Object) {
	b.Helper()

	var buffer bytes.Buffer
	ok, err := streamEncodeCollections(list, &buffer)
	if err != nil {
		b.Fatalf("streaming encode: %v", err)
	}
	if !ok {
		b.Fatal("expected streaming encoder to encode list")
	}
	expectedLength := buffer.Len()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		buffer.Reset()
		ok, err := streamEncodeCollections(list, &buffer)
		if err != nil {
			b.Fatalf("streaming encode: %v", err)
		}
		if !ok {
			b.Fatal("expected streaming encoder to encode list")
		}
		if got := buffer.Len(); got != expectedLength {
			b.Fatalf("encoded length = %d, want %d", got, expectedLength)
		}
	}
}
