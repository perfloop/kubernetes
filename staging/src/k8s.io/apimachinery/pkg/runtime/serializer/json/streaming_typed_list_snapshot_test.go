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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testapigroupv1 "k8s.io/apimachinery/pkg/apis/testapigroup/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type pointerCarpList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []*testapigroupv1.Carp `json:"items"`
}

func (l *pointerCarpList) DeepCopyObject() runtime.Object {
	return l
}

type runtimeObjectList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []runtime.Object `json:"items"`
}

func (l *runtimeObjectList) DeepCopyObject() runtime.Object {
	return l
}

type valueRuntimeObject struct {
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`
}

func (valueRuntimeObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (o valueRuntimeObject) DeepCopyObject() runtime.Object {
	return o
}

type valueRuntimeObjectList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []valueRuntimeObject `json:"items"`
}

func (l *valueRuntimeObjectList) DeepCopyObject() runtime.Object {
	return l
}

type mutateAfterFirstWriteBuffer struct {
	bytes.Buffer
	mutate func()
	writes int
}

func (b *mutateAfterFirstWriteBuffer) Write(p []byte) (int, error) {
	n, err := b.Buffer.Write(p)
	if b.writes == 0 {
		b.mutate()
	}
	b.writes++
	return n, err
}

func streamingItemNames(t *testing.T, data []byte) []string {
	t.Helper()

	var output struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := stdjson.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode streaming output: %v", err)
	}
	names := make([]string, len(output.Items))
	for i := range output.Items {
		names[i] = output.Items[i].Metadata.Name
	}
	return names
}

func testStreamingTypedListSnapshotsItemsBeforeWriterCallback(t *testing.T) {
	list := &testapigroupv1.CarpList{
		Items: []testapigroupv1.Carp{
			{ObjectMeta: metav1.ObjectMeta{Name: "first"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "second"}},
		},
	}
	writer := &mutateAfterFirstWriteBuffer{
		mutate: func() {
			list.Items = []testapigroupv1.Carp{{ObjectMeta: metav1.ObjectMeta{Name: "replacement"}}}
		},
	}
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})
	if err := streaming.Encode(list, writer); err != nil {
		t.Fatalf("streaming encode: %v", err)
	}
	got := streamingItemNames(t, writer.Bytes())
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("streaming item sequence = %v, want %v", got, want)
	}
}

func testStreamingPointerListSnapshotsItemsBeforeWriterCallback(t *testing.T) {
	list := &pointerCarpList{
		Items: []*testapigroupv1.Carp{
			{ObjectMeta: metav1.ObjectMeta{Name: "first"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "second"}},
		},
	}
	writer := &mutateAfterFirstWriteBuffer{
		mutate: func() {
			list.Items[1] = &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "replacement"}}
		},
	}
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})
	if err := streaming.Encode(list, writer); err != nil {
		t.Fatalf("streaming encode: %v", err)
	}

	got := streamingItemNames(t, writer.Bytes())
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("streaming item sequence = %v, want %v", got, want)
	}
}

func testStreamingRuntimeObjectListSnapshotsItemsBeforeWriterCallback(t *testing.T) {
	list := &runtimeObjectList{
		Items: []runtime.Object{
			&testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "first"}},
			&testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "second"}},
		},
	}
	writer := &mutateAfterFirstWriteBuffer{
		mutate: func() {
			list.Items[1] = &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "replacement"}}
		},
	}
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})
	if err := streaming.Encode(list, writer); err != nil {
		t.Fatalf("streaming encode: %v", err)
	}

	got := streamingItemNames(t, writer.Bytes())
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("streaming item sequence = %v, want %v", got, want)
	}
}

func testStreamingValueRuntimeObjectListSnapshotsItemsBeforeWriterCallback(t *testing.T) {
	list := &valueRuntimeObjectList{
		Items: []valueRuntimeObject{
			{Metadata: metav1.ObjectMeta{Name: "first"}},
			{Metadata: metav1.ObjectMeta{Name: "second"}},
		},
	}
	writer := &mutateAfterFirstWriteBuffer{
		mutate: func() {
			list.Items[1] = valueRuntimeObject{Metadata: metav1.ObjectMeta{Name: "replacement"}}
		},
	}
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})
	if err := streaming.Encode(list, writer); err != nil {
		t.Fatalf("streaming encode: %v", err)
	}

	got := streamingItemNames(t, writer.Bytes())
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("streaming item sequence = %v, want %v", got, want)
	}
}

func testStreamingRawExtensionListSnapshotsItemsBeforeWriterCallback(t *testing.T) {
	list := &rawExtensionList{
		Items: []runtime.RawExtension{{Raw: []byte(`{"metadata":{"name":"first"}}`)}},
	}
	writer := &mutateAfterFirstWriteBuffer{
		mutate: func() {
			list.Items[0].Raw = []byte(`{"metadata":{"name":"replacement"}}`)
		},
	}
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})
	if err := streaming.Encode(list, writer); err != nil {
		t.Fatalf("streaming encode: %v", err)
	}

	got := streamingItemNames(t, writer.Bytes())
	if want := []string{"first"}; !reflect.DeepEqual(got, want) {
		t.Errorf("streaming item sequence = %v, want %v", got, want)
	}
}
