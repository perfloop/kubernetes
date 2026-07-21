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

	yamlv2 "go.yaml.in/yaml/v2"
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

func TestYAMLToJSONWithDuplicateDetectionConvertsNestedMappings(t *testing.T) {
	data := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: nested\ndata:\n  value: retained\n")

	var yamlObj yamlv2.MapSlice
	if err := yamlv2.Unmarshal(data, &yamlObj); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	var nestedMappings int
	for _, item := range yamlObj {
		if item.Key != "metadata" && item.Key != "data" {
			continue
		}
		if _, ok := item.Value.(yamlv2.MapSlice); !ok {
			t.Fatalf("yaml MapSlice value for %q has type %T, want yaml.MapSlice", item.Key, item.Value)
		}
		nestedMappings++
	}
	if nestedMappings != 2 {
		t.Fatalf("yaml MapSlice found %d nested mappings, want 2", nestedMappings)
	}

	expected, err := yaml.YAMLToJSONStrict(data)
	if err != nil {
		t.Fatalf("YAMLToJSONStrict: %v", err)
	}
	actual, hasDuplicate, ok, err := yamlToJSONWithDuplicateDetection(data)
	if err != nil || !ok || hasDuplicate {
		t.Fatalf("yamlToJSONWithDuplicateDetection returned err=%v ok=%t duplicate=%t", err, ok, hasDuplicate)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("yamlToJSONWithDuplicateDetection = %s, want %s", actual, expected)
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

func TestYAMLToJSONWithDuplicateDetectionFallsBackForRootSequences(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("- key: value\n"),
		[]byte("---\n- key: value\n"),
		[]byte("# a sequence follows\n- key: value\n"),
	} {
		if _, _, ok, err := yamlToJSONWithDuplicateDetection(data); err != nil || ok {
			t.Fatalf("yamlToJSONWithDuplicateDetection(%q) returned err=%v ok=%t for a root sequence", data, err, ok)
		}
	}
}

func TestYAMLToJSONWithDuplicateDetectionFallsBackForComplexMapKeySyntax(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("? [key]\n: value\n"),
		[]byte("{[key]: value}\n"),
		[]byte("key: &key value\n? *key\n: value\n"),
	} {
		if _, _, ok, err := yamlToJSONWithDuplicateDetection(data); err != nil || ok {
			t.Fatalf("yamlToJSONWithDuplicateDetection(%q) returned err=%v ok=%t for complex map key syntax", data, err, ok)
		}
	}
}

func TestYAMLToJSONWithDuplicateDetectionReturnsParserError(t *testing.T) {
	data := []byte("apiVersion: v1\nkind: \"unterminated\n")
	_, expectedErr := yaml.YAMLToJSON(data)
	_, _, ok, actualErr := yamlToJSONWithDuplicateDetection(data)
	if ok || expectedErr == nil || actualErr == nil || actualErr.Error() != expectedErr.Error() {
		t.Fatalf("yamlToJSONWithDuplicateDetection returned ok=%t err=%v, want %v", ok, actualErr, expectedErr)
	}
}

func TestYAMLToJSONWithDuplicateDetectionReturnsUnsupportedScalarMapKeyError(t *testing.T) {
	data := []byte("null: value\n")
	_, expectedErr := yaml.YAMLToJSON(data)
	_, _, ok, actualErr := yamlToJSONWithDuplicateDetection(data)
	if ok || expectedErr == nil || actualErr == nil || actualErr.Error() != expectedErr.Error() {
		t.Fatalf("yamlToJSONWithDuplicateDetection returned ok=%t err=%v, want %v", ok, actualErr, expectedErr)
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
