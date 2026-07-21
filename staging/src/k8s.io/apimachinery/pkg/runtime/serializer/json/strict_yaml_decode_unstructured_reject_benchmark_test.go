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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDecodeStrictYAMLUnstructuredMissingKind(t *testing.T) {
	payload := strictYAMLMissingKindDuplicate(strictYAMLDecodePayloadSize, 0)
	into := &unstructured.Unstructured{}
	obj, gvk, err := newYAMLDecoder(true).Decode(payload, nil, into)
	requireMissingKind(t, obj, gvk, err)
}

func BenchmarkDecodeStrictYAMLUnstructuredMissingKindDuplicate10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLMissingKindDuplicate(strictYAMLDecodePayloadSize, 0),
		strictYAMLMissingKindDuplicate(strictYAMLDecodePayloadSize, 1),
	}
	decoder := newYAMLDecoder(true)
	into := &unstructured.Unstructured{}

	obj, gvk, err := decoder.Decode(payloads[0], nil, into)
	requireMissingKind(b, obj, gvk, err)

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, into)
		i++
	}
	b.StopTimer()
	requireMissingKind(b, obj, gvk, err)
}
