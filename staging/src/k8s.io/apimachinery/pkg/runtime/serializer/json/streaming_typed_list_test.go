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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testapigroupv1 "k8s.io/apimachinery/pkg/apis/testapigroup/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type rawExtensionList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []runtime.RawExtension `json:"items"`
}

func (l *rawExtensionList) DeepCopyObject() runtime.Object {
	return l
}

func TestStreamingRawExtensionListMatchesNormal(t *testing.T) {
	list := &rawExtensionList{
		TypeMeta: metav1.TypeMeta{Kind: "RawExtensionList", APIVersion: "example.test/v1"},
		ListMeta: metav1.ListMeta{ResourceVersion: "12345"},
		Items: []runtime.RawExtension{
			{Raw: []byte(`{"apiVersion":"example.test/v1","kind":"RawItem","metadata":{"name":"raw"}}`)},
			{Object: &testapigroupv1.Carp{TypeMeta: metav1.TypeMeta{Kind: "Carp", APIVersion: "example.test/v1"}}},
			{},
		},
	}

	normal := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{})
	streaming := NewSerializerWithOptions(DefaultMetaFactory, nil, nil, SerializerOptions{StreamingCollectionsEncoding: true})

	var normalBuffer bytes.Buffer
	if err := normal.Encode(list, &normalBuffer); err != nil {
		t.Fatalf("normal encode: %v", err)
	}

	var streamingBuffer writeCountingBuffer
	if err := streaming.Encode(list, &streamingBuffer); err != nil {
		t.Fatalf("streaming encode: %v", err)
	}
	if streamingBuffer.writeCount <= 1 {
		t.Fatalf("streaming encoder made %d writes, want more than one", streamingBuffer.writeCount)
	}
	if got, want := streamingBuffer.String(), normalBuffer.String(); got != want {
		t.Errorf("streaming output differs from normal output:\nstreaming: %s\nnormal: %s", got, want)
	}
}
