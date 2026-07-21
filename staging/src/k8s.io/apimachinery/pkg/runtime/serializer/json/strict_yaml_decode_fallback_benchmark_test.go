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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func strictYAMLMalformedPayload(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-malformed-%d\ndata:\n", variant)
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprint(&data, "  invalid: \"unterminated\n")
	return data.Bytes()
}

func strictYAMLRootSequencePayload(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "\xef\xbb\xbf# strict-yaml-root-sequence-%d\r---\r", variant)
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "- key-%05d: value-%d\r", i, variant)
	}
	return data.Bytes()
}

func strictYAMLUint64MapKeyPayload(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-uint64-%d\ndata:\n", variant)
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprint(&data, "18446744073709551615: retained\n")
	return data.Bytes()
}

func regularYAMLConversionError(t testing.TB, data []byte) string {
	t.Helper()

	if _, err := yaml.YAMLToJSON(data); err != nil {
		return err.Error()
	}
	t.Fatal("YAMLToJSON returned nil error")
	return ""
}

func regularYAMLRootSequenceError(t testing.TB, data []byte) string {
	t.Helper()

	obj, gvk, err := newYAMLDecoder(false).Decode(data, nil, &unstructured.Unstructured{})
	if obj != nil || gvk != nil || err == nil {
		t.Fatalf("regular Decode returned obj=%T gvk=%v err=%v, want metadata interpretation error", obj, gvk, err)
	}
	return err.Error()
}

func requireStrictYAMLReject(t testing.TB, wantError string, obj runtime.Object, gvk *schema.GroupVersionKind, err error) {
	t.Helper()

	if obj != nil || gvk != nil {
		t.Fatalf("Decode returned obj=%T gvk=%v with an expected rejection", obj, gvk)
	}
	if err == nil || err.Error() != wantError {
		t.Fatalf("Decode returned err=%v, want %q", err, wantError)
	}
}

func benchmarkStrictYAMLUnstructuredReject(b *testing.B, payloads [][]byte, wantError string) {
	decoder := newYAMLDecoder(true)
	into := &unstructured.Unstructured{}

	obj, gvk, err := decoder.Decode(payloads[0], nil, into)
	requireStrictYAMLReject(b, wantError, obj, gvk, err)

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, into)
		i++
	}
	b.StopTimer()
	requireStrictYAMLReject(b, wantError, obj, gvk, err)
}

func BenchmarkDecodeStrictYAMLUnstructuredMalformed10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLMalformedPayload(strictYAMLDecodePayloadSize, 0),
		strictYAMLMalformedPayload(strictYAMLDecodePayloadSize, 1),
	}
	benchmarkStrictYAMLUnstructuredReject(b, payloads, regularYAMLConversionError(b, payloads[0]))
}

func BenchmarkDecodeStrictYAMLUnstructuredRootSequence10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLRootSequencePayload(strictYAMLDecodePayloadSize, 0),
		strictYAMLRootSequencePayload(strictYAMLDecodePayloadSize, 1),
	}
	benchmarkStrictYAMLUnstructuredReject(b, payloads, regularYAMLRootSequenceError(b, payloads[0]))
}

func BenchmarkDecodeStrictYAMLUnstructuredUint64MapKey10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLUint64MapKeyPayload(strictYAMLDecodePayloadSize, 0),
		strictYAMLUint64MapKeyPayload(strictYAMLDecodePayloadSize, 1),
	}
	benchmarkStrictYAMLUnstructuredReject(b, payloads, regularYAMLConversionError(b, payloads[0]))
}
