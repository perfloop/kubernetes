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
	"fmt"
	"reflect"
	"strconv"

	yamlv2 "go.yaml.in/yaml/v2"
)

// yamlToJSONWithDuplicateDetection converts a YAML mapping while preserving
// duplicate keys long enough to detect them. It returns ok=false for YAML
// constructs that must use sigs.k8s.io/yaml's general conversion instead.
func yamlToJSONWithDuplicateDetection(data []byte) ([]byte, bool, bool, error) {
	if mayContainYAMLMergeSyntax(data) {
		return nil, false, false, nil
	}

	var yamlObj yamlv2.MapSlice
	if err := yamlv2.Unmarshal(data, &yamlObj); err != nil || len(yamlObj) == 0 {
		return nil, false, false, nil
	}

	jsonObj, hasDuplicate, err := yamlToJSONableObject(yamlObj)
	if err != nil {
		return nil, false, false, nil
	}
	jsonData, err := json.Marshal(jsonObj)
	if err != nil {
		// This is the same JSON marshal performed by sigs.k8s.io/yaml after
		// conversion, so returning it directly avoids reparsing a YAML value
		// that cannot be represented as JSON (for example, .nan).
		return nil, false, false, err
	}
	return jsonData, hasDuplicate, true, nil
}

// mayContainYAMLMergeSyntax conservatively selects the regular converter for
// every source spelling that yaml.v2 can resolve as a merge key. A merge key
// has scalar value "<<" (which requires '<' or an escape), or an explicit
// merge tag (which requires '!' or a tag directive '%').
func mayContainYAMLMergeSyntax(data []byte) bool {
	return bytes.ContainsAny(data, "<!\\%")
}

// yamlToJSONableObject mirrors sigs.k8s.io/yaml's conversion with no target
// type while retaining duplicate information from yaml.MapSlice.
func yamlToJSONableObject(yamlObj interface{}) (interface{}, bool, error) {
	switch typedYAMLObj := yamlObj.(type) {
	case yamlv2.MapSlice:
		strMap := make(map[string]interface{}, len(typedYAMLObj))
		seenKeys := make(map[interface{}]struct{}, len(typedYAMLObj))
		jsonKeys := make(map[string]interface{}, len(typedYAMLObj))
		hasDuplicate := false
		for _, item := range typedYAMLObj {
			keyString, err := yamlMapKeyToString(item.Key, item.Value)
			if err != nil {
				return nil, false, err
			}
			if previousKey, found := jsonKeys[keyString]; found && !reflect.DeepEqual(previousKey, item.Key) {
				return nil, false, fmt.Errorf("multiple YAML map keys convert to JSON key %q", keyString)
			}
			jsonKeys[keyString] = item.Key
			if _, found := seenKeys[item.Key]; found {
				hasDuplicate = true
			}
			seenKeys[item.Key] = struct{}{}

			jsonValue, valueHasDuplicate, err := yamlToJSONableObject(item.Value)
			if err != nil {
				return nil, false, err
			}
			strMap[keyString] = jsonValue
			hasDuplicate = hasDuplicate || valueHasDuplicate
		}
		return strMap, hasDuplicate, nil
	case []interface{}:
		array := make([]interface{}, len(typedYAMLObj))
		hasDuplicate := false
		for i, value := range typedYAMLObj {
			jsonValue, valueHasDuplicate, err := yamlToJSONableObject(value)
			if err != nil {
				return nil, false, err
			}
			array[i] = jsonValue
			hasDuplicate = hasDuplicate || valueHasDuplicate
		}
		return array, hasDuplicate, nil
	case map[interface{}]interface{}, map[string]interface{}:
		return nil, false, fmt.Errorf("unexpected YAML map type %T", yamlObj)
	default:
		return yamlObj, false, nil
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
