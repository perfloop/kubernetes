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

	yamlv2 "go.yaml.in/yaml/v2"
)

// yamlToJSONWithDuplicateDetection converts a YAML mapping while preserving
// duplicate keys long enough to detect them. It returns ok=false for YAML
// constructs that must use sigs.k8s.io/yaml's general conversion instead.
func yamlToJSONWithDuplicateDetection(data []byte) ([]byte, bool, bool, error) {
	if mayRequireRegularYAMLConversion(data) {
		return nil, false, false, nil
	}

	// Decoding into MapSlice also makes yaml.v2 retain MapSlice for nested
	// mappings, so recursive conversion observes every mapping entry and its
	// duplicate keys (see TestYAMLToJSONWithDuplicateDetectionConvertsNestedMappings).
	var yamlObj yamlv2.MapSlice
	if err := yamlv2.Unmarshal(data, &yamlObj); err != nil {
		var typeErr *yamlv2.TypeError
		if !errors.As(err, &typeErr) {
			// YAMLToJSON returns parser errors before conversion, and MapSlice
			// observes the same parser error without needing a second parse.
			return nil, false, false, err
		}
		return nil, false, false, nil
	} else if len(yamlObj) == 0 {
		return nil, false, false, nil
	}

	jsonObj, hasDuplicate, err := yamlToJSONableObject(yamlObj)
	if err != nil {
		if errors.Is(err, errYAMLConversionFallback) {
			return nil, false, false, nil
		}
		return nil, false, false, err
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

// mayRequireRegularYAMLConversion conservatively selects the regular
// converter for source syntax that MapSlice cannot prove equivalent. A merge
// key has scalar value "<<" (requiring '<' or an escape), or an explicit merge
// tag (requiring '!' or a tag directive '%'). Complex map keys require an
// explicit-key, flow-collection, or alias indicator. A leading '-' may be a
// root sequence or document marker; a comment preamble can hide either root
// shape, so both use the regular converter without scanning the full input.
func mayRequireRegularYAMLConversion(data []byte) bool {
	if mayStartWithYAMLSequenceOrDocument(data) {
		return true
	}
	return bytes.ContainsAny(data, "<!\\%?[{*")
}

func mayStartWithYAMLSequenceOrDocument(data []byte) bool {
	data = bytes.TrimLeft(data, " \t\r\n")
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	return len(data) > 0 && (data[0] == '-' || data[0] == '#')
}

var errYAMLConversionFallback = errors.New("YAML conversion requires the regular converter")

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
				return nil, false, fmt.Errorf("%w: multiple YAML map keys convert to JSON key %q", errYAMLConversionFallback, keyString)
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
		return nil, false, fmt.Errorf("%w: unexpected YAML map type %T", errYAMLConversionFallback, yamlObj)
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
