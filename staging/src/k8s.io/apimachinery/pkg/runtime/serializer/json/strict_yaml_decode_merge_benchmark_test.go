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
)

func strictYAMLMergePayload(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "base: &base\n  apiVersion: v1\n  kind: ConfigMap\n  metadata:\n    name: strict-yaml-merge-%d\n  data:\n", variant)
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "    key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprint(&data, "!!merge \"\\x3c\\x3c\": *base\n")
	return data.Bytes()
}

func requireStrictYAMLMergeMatchesRegularYAML(t testing.TB, data []byte) {
	t.Helper()

	strictInto := &unstructured.Unstructured{}
	strictObj, strictGVK, strictErr := newYAMLDecoder(true).Decode(data, nil, strictInto)
	regularInto := &unstructured.Unstructured{}
	regularObj, regularGVK, regularErr := newYAMLDecoder(false).Decode(data, nil, regularInto)
	if strictErr != nil || regularErr != nil {
		t.Fatalf("Decode returned strict=%v regular=%v", strictErr, regularErr)
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

func BenchmarkDecodeStrictYAMLUnstructuredMerge10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLMergePayload(strictYAMLDecodePayloadSize, 0),
		strictYAMLMergePayload(strictYAMLDecodePayloadSize, 1),
	}
	requireStrictYAMLMergeMatchesRegularYAML(b, payloads[0])

	decoder := newYAMLDecoder(true)
	into := &unstructured.Unstructured{}
	var obj interface{}
	var err error
	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, _, err = decoder.Decode(payloads[i&1], nil, into)
		i++
	}
	b.StopTimer()
	if err != nil || obj != into {
		b.Fatalf("Decode returned obj=%T err=%v", obj, err)
	}
}
