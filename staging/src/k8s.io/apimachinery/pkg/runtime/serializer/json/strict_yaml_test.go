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

func TestStrictYAMLToJSON(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("apiVersion: v1\nkind: ConfigMap\ndata:\n  enabled: true\n  replicas: 3\n  nested:\n    name: example\n"),
		[]byte("apiVersion: v1\nkind: List\nitems:\n  - kind: ConfigMap\n    data:\n      one: 1\n      two: 2\n"),
		[]byte("true: false\n1: value\n1.5: decimal\n"),
	} {
		expected, err := yaml.YAMLToJSONStrict(data)
		if err != nil {
			t.Fatalf("YAMLToJSONStrict(%q): %v", data, err)
		}

		actual, partialYAML, strictErr, conversionErr := strictYAMLToJSON(data)
		if partialYAML != nil || strictErr != nil || conversionErr != nil {
			t.Fatalf("strictYAMLToJSON(%q) returned strict=%v conversion=%v", data, strictErr, conversionErr)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("strictYAMLToJSON(%q) = %s, want %s", data, actual, expected)
		}
	}
}

func TestCanUsePartialStrictYAMLMetadata(t *testing.T) {
	nestedDuplicate := []byte("apiVersion:\n  - v1\nkind: ConfigMap\ndata:\n  duplicate: first\n  duplicate: second\n")
	_, partialYAML, strictErr, conversionErr := strictYAMLToJSON(nestedDuplicate)
	if partialYAML == nil || strictErr == nil || conversionErr != nil {
		t.Fatalf("strictYAMLToJSON returned strict=%v conversion=%v, want duplicate error", strictErr, conversionErr)
	}
	if !partialStrictYAMLMayHaveMetadataError(partialYAML) {
		t.Fatal("partialStrictYAMLMayHaveMetadataError rejected an apiVersion array")
	}
	if !canUsePartialStrictYAMLMetadata(nestedDuplicate, strictErr, SimpleMetaFactory{}) {
		t.Fatal("canUsePartialStrictYAMLMetadata rejected a nested duplicate with stable root metadata")
	}

	missingKind := []byte("apiVersion: v1\ndata:\n  duplicate: first\n  duplicate: second\n")
	_, partialYAML, strictErr, conversionErr = strictYAMLToJSON(missingKind)
	if partialYAML == nil || strictErr == nil || conversionErr != nil {
		t.Fatalf("strictYAMLToJSON returned strict=%v conversion=%v, want duplicate error", strictErr, conversionErr)
	}
	if partialStrictYAMLMayHaveMetadataError(partialYAML) {
		t.Fatal("partialStrictYAMLMayHaveMetadataError accepted a valid apiVersion without kind")
	}

	rootDuplicate := []byte("apiVersion: v1\napiVersion:\n  - v1\nkind: ConfigMap\n")
	_, partialYAML, strictErr, conversionErr = strictYAMLToJSON(rootDuplicate)
	if partialYAML == nil || strictErr == nil || conversionErr != nil {
		t.Fatalf("strictYAMLToJSON returned strict=%v conversion=%v, want duplicate error", strictErr, conversionErr)
	}
	if canUsePartialStrictYAMLMetadata(rootDuplicate, strictErr, SimpleMetaFactory{}) {
		t.Fatal("canUsePartialStrictYAMLMetadata accepted duplicate root apiVersion")
	}
}
