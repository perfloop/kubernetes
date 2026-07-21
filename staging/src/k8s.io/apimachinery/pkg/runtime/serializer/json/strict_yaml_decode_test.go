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
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

const strictYAMLDecodePayloadSize = 10 * 1024

func strictYAMLConfigMap(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-decode-%d\n  namespace: benchmark\ndata:\n", variant)

	value := fmt.Sprintf("value-%d-%s", variant, strings.Repeat("x", 64))
	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: %q\n", i, value)
	}
	return data.Bytes()
}

func newYAMLDecoder(strict bool) *json.Serializer {
	scheme := runtime.NewScheme()
	return json.NewSerializerWithOptions(json.DefaultMetaFactory, nil, scheme, json.SerializerOptions{Yaml: true, Strict: strict})
}

func requireDecodedConfigMap(t testing.TB, obj runtime.Object, gvk *schema.GroupVersionKind, into *unstructured.Unstructured) {
	t.Helper()

	if obj != into {
		t.Fatalf("Decode returned %T instead of the provided unstructured object", obj)
	}
	wantGVK := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	if gvk == nil || *gvk != wantGVK {
		t.Fatalf("Decode returned GVK %v, want %v", gvk, wantGVK)
	}
	if into.GetName() == "" {
		t.Fatal("Decode did not preserve metadata.name")
	}
}

func requireConfigMapData(t testing.TB, into *unstructured.Unstructured, key string) {
	t.Helper()

	value, found, err := unstructured.NestedString(into.Object, "data", key)
	if err != nil || !found || value == "" {
		t.Fatalf("Decode did not preserve ConfigMap data: value=%q found=%t err=%v", value, found, err)
	}
}

func TestStrictYAMLConversionMatchesRegular(t *testing.T) {
	data := strictYAMLConfigMap(strictYAMLDecodePayloadSize, 0)

	regular, err := yaml.YAMLToJSON(data)
	if err != nil {
		t.Fatalf("YAMLToJSON returned error: %v", err)
	}
	strict, err := yaml.YAMLToJSONStrict(data)
	if err != nil {
		t.Fatalf("YAMLToJSONStrict returned error: %v", err)
	}
	if !bytes.Equal(strict, regular) {
		t.Fatal("YAMLToJSONStrict and YAMLToJSON returned different JSON for valid YAML")
	}
}

func TestDecodeStrictYAMLDuplicateDiagnostics(t *testing.T) {
	duplicateConfigMap := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-duplicate\ndata:\n  value: retained\n  value: retained\n")

	t.Run("returns decoded object with strict error", func(t *testing.T) {
		into := &unstructured.Unstructured{}
		obj, gvk, err := newYAMLDecoder(true).Decode(duplicateConfigMap, nil, into)
		if !runtime.IsStrictDecodingError(err) {
			t.Fatalf("Decode returned %v, want strict decoding error", err)
		}
		if !strings.Contains(err.Error(), `"value" already set in map`) {
			t.Fatalf("Decode returned %v, want duplicate YAML diagnostic", err)
		}
		requireDecodedConfigMap(t, obj, gvk, into)
		value, found, nestedErr := unstructured.NestedString(into.Object, "data", "value")
		if nestedErr != nil || !found || value != "retained" {
			t.Fatalf("Decode did not retain the regular YAML result: value=%q found=%t err=%v", value, found, nestedErr)
		}
	})

	t.Run("unknown bypasses strict diagnostics", func(t *testing.T) {
		into := &runtime.Unknown{}
		obj, gvk, err := newYAMLDecoder(true).Decode(duplicateConfigMap, nil, into)
		if err != nil {
			t.Fatalf("Decode returned error for runtime.Unknown: %v", err)
		}
		if obj != into {
			t.Fatalf("Decode returned %T instead of runtime.Unknown", obj)
		}
		if gvk == nil || *gvk != (schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}) {
			t.Fatalf("Decode returned GVK %v, want v1 ConfigMap", gvk)
		}
		if !bytes.Equal(into.Raw, duplicateConfigMap) {
			t.Fatal("Decode did not preserve runtime.Unknown raw data")
		}
	})

	t.Run("metadata error precedes strict diagnostics", func(t *testing.T) {
		data := []byte("data:\n  value: retained\n  value: retained\n")
		_, _, err := newYAMLDecoder(true).Decode(data, nil, &unstructured.Unstructured{})
		if err == nil {
			t.Fatal("Decode returned nil error for YAML without kind")
		}
		if runtime.IsStrictDecodingError(err) {
			t.Fatalf("Decode returned strict error %v instead of missing kind error", err)
		}
		if !strings.Contains(err.Error(), "Object 'Kind' is missing") {
			t.Fatalf("Decode returned %v, want missing kind error", err)
		}
	})
}

func benchmarkDecodeYAMLConfigMap(b *testing.B, strict bool) {
	payloads := [][]byte{
		strictYAMLConfigMap(strictYAMLDecodePayloadSize, 0),
		strictYAMLConfigMap(strictYAMLDecodePayloadSize, 1),
	}
	decoder := newYAMLDecoder(strict)
	into := &unstructured.Unstructured{}

	obj, gvk, err := decoder.Decode(payloads[0], nil, into)
	if err != nil {
		b.Fatalf("Decode setup failed: %v", err)
	}
	requireDecodedConfigMap(b, obj, gvk, into)
	requireConfigMapData(b, into, "key-00000")

	var i int
	b.ResetTimer()
	for b.Loop() {
		obj, gvk, err = decoder.Decode(payloads[i&1], nil, into)
		if err != nil {
			b.Fatal(err)
		}
		i++
	}
	b.StopTimer()
	requireDecodedConfigMap(b, obj, gvk, into)
	requireConfigMapData(b, into, "key-00000")
}

func BenchmarkDecodeStrictYAML10KiB(b *testing.B) {
	benchmarkDecodeYAMLConfigMap(b, true)
}

func BenchmarkDecodeYAML10KiB(b *testing.B) {
	benchmarkDecodeYAMLConfigMap(b, false)
}
