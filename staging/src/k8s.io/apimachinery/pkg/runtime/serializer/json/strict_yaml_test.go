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

	"sigs.k8s.io/yaml"
)

func TestYAMLToJSONWithDuplicateDetection(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("apiVersion: v1\nkind: ConfigMap\ndata:\n  enabled: true\n  replicas: 3\n  nested:\n    name: example\n"),
		[]byte("apiVersion: v1\nkind: List\nitems:\n  - kind: ConfigMap\n    data:\n      one: 1\n      two: 2\n"),
		[]byte("true: false\n1: value\n1.5: decimal\n"),
	} {
		expected, err := yaml.YAMLToJSONStrict(data)
		if err != nil {
			t.Fatalf("YAMLToJSONStrict(%q): %v", data, err)
		}

		actual, hasDuplicate, ok, actualErr := yamlToJSONWithDuplicateDetection(data)
		if actualErr != nil || !ok || hasDuplicate {
			t.Fatalf("yamlToJSONWithDuplicateDetection(%q) returned err=%v ok=%t duplicate=%t", data, actualErr, ok, hasDuplicate)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("yamlToJSONWithDuplicateDetection(%q) = %s, want %s", data, actual, expected)
		}
	}
}

func TestYAMLToJSONWithDuplicateDetectionFallsBackForNonMappings(t *testing.T) {
	for _, data := range [][]byte{[]byte(""), []byte("null\n"), []byte("[]\n"), []byte("{}\n")} {
		if _, _, ok, err := yamlToJSONWithDuplicateDetection(data); err != nil || ok {
			t.Fatalf("yamlToJSONWithDuplicateDetection(%q) returned err=%v ok=%t for a non-mapping", data, err, ok)
		}
	}
}

func TestYAMLToJSONWithDuplicateDetectionFallsBackForMergeSyntax(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("base: &base\n  apiVersion: v1\n<<: *base\n"),
		[]byte("base: &base\n  apiVersion: v1\n!!merge \"\\x3c\\x3c\": *base\n"),
		[]byte("%TAG !e! tag:yaml.org,2002:\n---\nbase: &base\n  apiVersion: v1\n!e!merge \"\\x3c\\x3c\": *base\n"),
	} {
		if _, _, ok, err := yamlToJSONWithDuplicateDetection(data); err != nil || ok {
			t.Fatalf("yamlToJSONWithDuplicateDetection(%q) returned err=%v ok=%t for merge syntax", data, err, ok)
		}
	}
}

func TestYAMLToJSONWithDuplicateDetectionReturnsJSONConversionError(t *testing.T) {
	_, _, ok, err := yamlToJSONWithDuplicateDetection([]byte("apiVersion: v1\nkind: ConfigMap\nvalue: .nan\n"))
	if ok || err == nil || err.Error() != "json: unsupported value: NaN" {
		t.Fatalf("yamlToJSONWithDuplicateDetection returned ok=%t err=%v", ok, err)
	}
}

func TestYAMLToJSONWithDuplicateDetectionRetainsLastValue(t *testing.T) {
	data := []byte("apiVersion: v1\nkind: ConfigMap\ndata:\n  value: first\n  value: second\n")
	expected, err := yaml.YAMLToJSON(data)
	if err != nil {
		t.Fatalf("YAMLToJSON: %v", err)
	}

	actual, hasDuplicate, ok, actualErr := yamlToJSONWithDuplicateDetection(data)
	if actualErr != nil || !ok || !hasDuplicate {
		t.Fatalf("yamlToJSONWithDuplicateDetection returned err=%v ok=%t duplicate=%t", actualErr, ok, hasDuplicate)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("yamlToJSONWithDuplicateDetection = %s, want %s", actual, expected)
	}
}
