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
)

func strictYAMLMetadataRejectDuplicate(size, variant int) []byte {
	var data bytes.Buffer
	fmt.Fprintf(&data, "apiVersion:\n  - v1\nkind: ConfigMap\nmetadata:\n  name: strict-yaml-metadata-reject-%d\ndata:\n", variant)

	for i := 0; data.Len() < size; i++ {
		fmt.Fprintf(&data, "  key-%05d: value-%d\n", i, variant)
	}
	fmt.Fprintf(&data, "  duplicate: first-%d\n  duplicate: second-%d\n", variant, variant)
	return data.Bytes()
}

func requireMetadataInterpretError(t testing.TB, obj runtime.Object, gvk *schema.GroupVersionKind, err error) {
	t.Helper()

	if obj != nil || gvk != nil {
		t.Fatalf("Decode returned obj=%T gvk=%v with a metadata interpretation error", obj, gvk)
	}
	const want = "couldn't get version/kind; json parse error: json: cannot unmarshal array into Go struct field .apiVersion of type string"
	if err == nil || err.Error() != want {
		t.Fatalf("Decode returned %v, want %q", err, want)
	}
}

func TestDecodeStrictYAMLMetadataInterpretError(t *testing.T) {
	payload := strictYAMLMetadataRejectDuplicate(strictYAMLDecodePayloadSize, 0)
	obj, gvk, err := newYAMLDecoder(true).Decode(payload, nil, &unstructured.Unstructured{})
	requireMetadataInterpretError(t, obj, gvk, err)
}

func TestDecodeStrictYAMLDuplicatePreservesLastValue(t *testing.T) {
	data := []byte("apiVersion: v1\nkind: ConfigMap\ndata:\n  value: first\n  value: second\n")
	into := &unstructured.Unstructured{}
	obj, gvk, err := newYAMLDecoder(true).Decode(data, nil, into)
	if !runtime.IsStrictDecodingError(err) {
		t.Fatalf("Decode returned %v, want strict decoding error", err)
	}
	if obj != into {
		t.Fatalf("Decode returned %T instead of the provided unstructured object", obj)
	}
	if gvk == nil || *gvk != (schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}) {
		t.Fatalf("Decode returned GVK %v, want v1 ConfigMap", gvk)
	}
	value, found, nestedErr := unstructured.NestedString(into.Object, "data", "value")
	if nestedErr != nil || !found || value != "second" {
		t.Fatalf("Decode retained value=%q found=%t err=%v, want last duplicate value", value, found, nestedErr)
	}
}

func TestDecodeStrictYAMLRootDuplicatePreservesMetadata(t *testing.T) {
	data := []byte("apiVersion:\n  - v1\napiVersion: v1\nkind: ConfigMap\ndata:\n  value: retained\n")
	into := &unstructured.Unstructured{}
	obj, gvk, err := newYAMLDecoder(true).Decode(data, nil, into)
	if !runtime.IsStrictDecodingError(err) {
		t.Fatalf("Decode returned %v, want strict decoding error", err)
	}
	if obj != into {
		t.Fatalf("Decode returned %T instead of the provided unstructured object", obj)
	}
	wantGVK := &schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	if gvk == nil || *gvk != *wantGVK {
		t.Fatalf("Decode returned GVK %v, want %v", gvk, wantGVK)
	}
}
