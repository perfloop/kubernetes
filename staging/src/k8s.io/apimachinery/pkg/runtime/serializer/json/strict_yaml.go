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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	yamlv2 "go.yaml.in/yaml/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// strictYAMLToJSON converts YAML once. For duplicate-key errors it retains the
// partial YAML object without converting it: yaml.v2 retains the first duplicate
// value, whereas regular YAML decoding retains the last one.
func strictYAMLToJSON(data []byte) ([]byte, interface{}, error, error) {
	var yamlObj interface{}
	strictErr := yamlv2.UnmarshalStrict(data, &yamlObj)
	if strictErr != nil {
		return nil, yamlObj, strictErr, nil
	}

	jsonObj, err := yamlToJSONableObject(yamlObj)
	if err != nil {
		return nil, nil, nil, err
	}
	jsonData, err := json.Marshal(jsonObj)
	if err != nil {
		return nil, nil, nil, err
	}
	return jsonData, nil, nil, nil
}

// yamlToJSONableObject mirrors sigs.k8s.io/yaml's conversion with no target type.
func yamlToJSONableObject(yamlObj interface{}) (interface{}, error) {
	switch typedYAMLObj := yamlObj.(type) {
	case map[interface{}]interface{}:
		strMap := make(map[string]interface{}, len(typedYAMLObj))
		for key, value := range typedYAMLObj {
			keyString, err := yamlMapKeyToString(key, value)
			if err != nil {
				return nil, err
			}
			jsonValue, err := yamlToJSONableObject(value)
			if err != nil {
				return nil, err
			}
			strMap[keyString] = jsonValue
		}
		return strMap, nil
	case []interface{}:
		array := make([]interface{}, len(typedYAMLObj))
		for i, value := range typedYAMLObj {
			jsonValue, err := yamlToJSONableObject(value)
			if err != nil {
				return nil, err
			}
			array[i] = jsonValue
		}
		return array, nil
	default:
		return yamlObj, nil
	}
}

func yamlMapKeyToString(key, value interface{}) (string, error) {
	switch typedKey := key.(type) {
	case string:
		return typedKey, nil
	case int:
		return strconv.Itoa(typedKey), nil
	case int64:
		return strconv.FormatInt(typedKey, 10), nil
	case float64:
		keyString := strconv.FormatFloat(typedKey, 'g', -1, 32)
		switch keyString {
		case "+Inf":
			keyString = ".inf"
		case "-Inf":
			keyString = "-.inf"
		case "NaN":
			keyString = ".nan"
		}
		return keyString, nil
	case bool:
		return strconv.FormatBool(typedKey), nil
	default:
		return "", fmt.Errorf("unsupported map key of type: %s, key: %+#v, value: %+#v", reflect.TypeOf(key), key, value)
	}
}

func partialStrictYAMLMayHaveMetadataError(yamlObj interface{}) bool {
	root, ok := yamlObj.(map[interface{}]interface{})
	if !ok {
		return true
	}

	for key, value := range root {
		keyString, ok := key.(string)
		if !ok {
			continue
		}
		switch keyString {
		case "apiVersion":
			switch typedValue := value.(type) {
			case nil:
			case string:
				if _, err := schema.ParseGroupVersion(typedValue); err != nil {
					return true
				}
			default:
				return true
			}
		case "kind":
			if value != nil {
				if _, ok := value.(string); !ok {
					return true
				}
			}
		}
	}
	return false
}

func strictYAMLMetadataJSON(yamlObj interface{}) ([]byte, error) {
	root, ok := yamlObj.(map[interface{}]interface{})
	if !ok {
		return nil, fmt.Errorf("expected a YAML mapping, got %T", yamlObj)
	}

	metadata := make(map[string]interface{}, 2)
	for key, value := range root {
		keyString, ok := key.(string)
		if !ok || (keyString != "apiVersion" && keyString != "kind") {
			continue
		}
		jsonValue, err := yamlToJSONableObject(value)
		if err != nil {
			return nil, err
		}
		metadata[keyString] = jsonValue
	}
	return json.Marshal(metadata)
}

func canUsePartialStrictYAMLMetadata(data []byte, strictErr error, meta MetaFactory) bool {
	if !isSimpleMetaFactory(meta) || !isDuplicateYAMLError(strictErr) {
		return false
	}

	apiVersionCount := 0
	kindCount := 0
	sawRootKey := false
	for len(data) > 0 {
		line := data
		if newline := bytes.IndexByte(line, '\n'); newline >= 0 {
			line, data = line[:newline], line[newline+1:]
		} else {
			data = nil
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		if line[0] == ' ' {
			continue
		}

		key, ok := simpleYAMLRootKey(line)
		if !ok {
			return false
		}
		sawRootKey = true
		switch key {
		case "apiVersion":
			apiVersionCount++
		case "kind":
			kindCount++
		}
	}
	return sawRootKey && apiVersionCount <= 1 && kindCount <= 1
}

func isSimpleMetaFactory(meta MetaFactory) bool {
	switch meta.(type) {
	case SimpleMetaFactory, *SimpleMetaFactory:
		return true
	default:
		return false
	}
}

func isDuplicateYAMLError(err error) bool {
	var typeErr *yamlv2.TypeError
	if !errors.As(err, &typeErr) || len(typeErr.Errors) == 0 {
		return false
	}
	for _, message := range typeErr.Errors {
		if !strings.Contains(message, "already set in map") {
			return false
		}
	}
	return true
}

func simpleYAMLRootKey(line []byte) (string, bool) {
	colon := bytes.IndexByte(line, ':')
	if colon <= 0 || (colon+1 < len(line) && line[colon+1] != ' ' && line[colon+1] != '#') {
		return "", false
	}
	key := line[:colon]
	for i, character := range key {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_' {
			continue
		}
		if i > 0 && (character >= '0' && character <= '9' || character == '-') {
			continue
		}
		return "", false
	}
	return string(key), true
}
