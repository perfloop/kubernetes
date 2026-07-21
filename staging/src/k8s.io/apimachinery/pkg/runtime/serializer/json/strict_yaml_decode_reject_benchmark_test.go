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

package json_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func strictYAMLMetadataRejectDuplicatePayload(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "apiVersion:\n  - v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-metadata-reject-%d\ndata:\n", variant)
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprintf(&data, "  duplicate: first-%d\n  duplicate: second-%d\n", variant, variant)
	return data.Bytes()
}

func strictYAMLNaNRejectPayload(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-nan-reject-%d\ndata:\n", variant)
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprint(&data, "  invalid: .nan\n")
	return data.Bytes()
}

func requireStrictYAMLMetadataInterpretError(t testing.TB, obj runtime.Object, gvk *schema.GroupVersionKind, err error) {
	t.Helper()

	if obj != nil || gvk != nil {
		t.Fatalf("Decode returned obj=%T gvk=%v with a metadata interpretation error", obj, gvk)
	}
	const want = "couldn't get version/kind; json parse error: json: cannot unmarshal array into Go struct field .apiVersion of type string"
	if err == nil || err.Error() != want {
		t.Fatalf("Decode returned %v, want %q", err, want)
	}
}

func requireStrictYAMLNaNConversionError(t testing.TB, obj runtime.Object, gvk *schema.GroupVersionKind, err error) {
	t.Helper()

	if obj != nil || gvk != nil {
		t.Fatalf("Decode returned obj=%T gvk=%v with a YAML-to-JSON conversion error", obj, gvk)
	}
	const want = "json: unsupported value: NaN"
	if err == nil || err.Error() != want {
		t.Fatalf("Decode returned %v, want %q", err, want)
	}
}

func TestDecodeStrictYAMLMergeMatchesRegularYAML(t *testing.T) {
	data := []byte("base: &base\n  apiVersion: v1\n  kind: ConfigMap\n!!merge \"\\x3c\\x3c\": *base\ndata:\n  value: retained\n")

	strictInto := &unstructured.Unstructured{}
	strictObj, strictGVK, strictErr := newYAMLDecoder(true).Decode(data, nil, strictInto)
	regularInto := &unstructured.Unstructured{}
	regularObj, regularGVK, regularErr := newYAMLDecoder(false).Decode(data, nil, regularInto)

	if strictErr != nil {
		t.Fatalf("strict Decode returned %v", strictErr)
	}
	if regularErr != nil {
		t.Fatalf("regular Decode returned %v", regularErr)
	}
	if strictObj != strictInto || regularObj != regularInto {
		t.Fatalf("Decode returned strict=%T regular=%T instead of the provided unstructured objects", strictObj, regularObj)
	}
	if strictGVK == nil || regularGVK == nil || *strictGVK != *regularGVK {
		t.Fatalf("Decode returned strict GVK=%v regular GVK=%v", strictGVK, regularGVK)
	}
	if !reflect.DeepEqual(strictInto.Object, regularInto.Object) {
		t.Fatalf("strict Decode object %#v differs from regular Decode object %#v", strictInto.Object, regularInto.Object)
	}
}

func BenchmarkDecodeStrictYAMLUnstructuredMetadataErrorDuplicate10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLMetadataRejectDuplicatePayload(strictYAMLDecodePayloadSize, 0),
		strictYAMLMetadataRejectDuplicatePayload(strictYAMLDecodePayloadSize, 1),
	}
	decoder := newYAMLDecoder(true)
	into := &unstructured.Unstructured{}

	obj, gvk, err := decoder.Decode(payloads[0], nil, into)
	requireStrictYAMLMetadataInterpretError(b, obj, gvk, err)

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, into)
		i++
	}
	b.StopTimer()
	requireStrictYAMLMetadataInterpretError(b, obj, gvk, err)
}

func BenchmarkDecodeStrictYAMLUnstructuredNaN10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLNaNRejectPayload(strictYAMLDecodePayloadSize, 0),
		strictYAMLNaNRejectPayload(strictYAMLDecodePayloadSize, 1),
	}
	decoder := newYAMLDecoder(true)
	into := &unstructured.Unstructured{}

	obj, gvk, err := decoder.Decode(payloads[0], nil, into)
	requireStrictYAMLNaNConversionError(b, obj, gvk, err)

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, into)
		i++
	}
	b.StopTimer()
	requireStrictYAMLNaNConversionError(b, obj, gvk, err)
}
