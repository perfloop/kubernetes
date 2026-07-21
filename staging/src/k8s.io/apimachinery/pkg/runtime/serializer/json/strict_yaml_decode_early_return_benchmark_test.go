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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func strictYAMLMissingKindDuplicate(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "metadata:\n  name: strict-yaml-reject-%d\ndata:\n", variant)

	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprintf(&data, "  duplicate: first-%d\n  duplicate: second-%d\n", variant, variant)
	return data.Bytes()
}

func requireDecodedUnknown(t testing.TB, obj runtime.Object, gvk *schema.GroupVersionKind, into *runtime.Unknown, wantRaw []byte) {
	t.Helper()

	if obj != into {
		t.Fatalf("Decode returned %T instead of runtime.Unknown", obj)
	}
	wantGVK := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	if gvk == nil || *gvk != wantGVK {
		t.Fatalf("Decode returned GVK %v, want %v", gvk, wantGVK)
	}
	if !bytes.Equal(into.Raw, wantRaw) {
		t.Fatal("Decode did not preserve runtime.Unknown raw data")
	}
}

func requireMissingKind(t testing.TB, obj runtime.Object, gvk *schema.GroupVersionKind, err error) {
	t.Helper()

	if obj != nil {
		t.Fatalf("Decode returned object %T with a missing kind", obj)
	}
	if !runtime.IsMissingKind(err) {
		t.Fatalf("Decode returned %v, want missing kind error", err)
	}
	if gvk == nil || *gvk != (schema.GroupVersionKind{}) {
		t.Fatalf("Decode returned GVK %v, want an empty GVK", gvk)
	}
}

func TestDecodeStrictYAMLEarlyReturns(t *testing.T) {
	t.Run("runtime.Unknown", func(t *testing.T) {
		payload := strictYAMLConfigMap(strictYAMLDecodePayloadSize, 0)
		into := &runtime.Unknown{}
		obj, gvk, err := newYAMLDecoder(true).Decode(payload, nil, into)
		if err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		requireDecodedUnknown(t, obj, gvk, into, payload)
	})

	t.Run("missing kind after duplicate", func(t *testing.T) {
		payload := strictYAMLMissingKindDuplicate(strictYAMLDecodePayloadSize, 0)
		obj, gvk, err := newYAMLDecoder(true).Decode(payload, nil, nil)
		requireMissingKind(t, obj, gvk, err)
	})
}

func BenchmarkDecodeStrictYAMLUnknown10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLConfigMap(strictYAMLDecodePayloadSize, 0),
		strictYAMLConfigMap(strictYAMLDecodePayloadSize, 1),
	}
	decoder := newYAMLDecoder(true)
	into := &runtime.Unknown{}

	obj, gvk, err := decoder.Decode(payloads[0], nil, into)
	if err != nil {
		b.Fatalf("Decode setup failed: %v", err)
	}
	requireDecodedUnknown(b, obj, gvk, into, payloads[0])

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, into)
		i++
	}
	b.StopTimer()
	requireDecodedUnknown(b, obj, gvk, into, payloads[(i-1)&1])
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkDecodeStrictYAMLMissingKindDuplicate10KiB(b *testing.B) {
	payloads := [][]byte{
		strictYAMLMissingKindDuplicate(strictYAMLDecodePayloadSize, 0),
		strictYAMLMissingKindDuplicate(strictYAMLDecodePayloadSize, 1),
	}
	decoder := newYAMLDecoder(true)

	obj, gvk, err := decoder.Decode(payloads[0], nil, nil)
	requireMissingKind(b, obj, gvk, err)

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, nil)
		i++
	}
	b.StopTimer()
	requireMissingKind(b, obj, gvk, err)
}
