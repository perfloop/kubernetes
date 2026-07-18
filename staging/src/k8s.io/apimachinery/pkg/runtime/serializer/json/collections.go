/*
Copyright 2025 The Kubernetes Authors.

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
	"io"
	"reflect"
	"sort"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/conversion"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	rawExtensionObjectType = reflect.TypeOf(runtime.RawExtension{})
	objectType             = reflect.TypeOf((*runtime.Object)(nil)).Elem()
)

// listItems retains either a direct value-element slice or the eager object
// snapshot required before the encoder invokes callbacks.
type listItems struct {
	direct   reflect.Value
	snapshot []runtime.Object
}

func streamEncodeCollections(obj runtime.Object, w io.Writer) (bool, error) {
	list, ok := obj.(*unstructured.UnstructuredList)
	if ok {
		return true, newStreamEncoder(w).encodeUnstructuredList(list)
	}
	if _, ok := obj.(json.Marshaler); ok {
		return false, nil
	}
	typeMeta, listMeta, items, err := getListMeta(obj)
	if err == nil {
		return true, newStreamEncoder(w).encodeList(typeMeta, listMeta, items)
	}
	return false, nil
}

// getListMeta implements list extraction logic for json stream serialization.
//
// Reason for a custom logic instead of reusing accessors from meta package:
// * Validate json tags to prevent incompatibility with json standard package.
// * ListMetaAccessor doesn't distinguish empty from nil value.
// * TypeAccessort reparsing "apiVersion" and serializing it with "{group}/{version}"
func getListMeta(list runtime.Object) (metav1.TypeMeta, metav1.ListMeta, listItems, error) {
	listValue, err := conversion.EnforcePtr(list)
	if err != nil {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, err
	}
	listType := listValue.Type()
	if listType.NumField() != 3 {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf("expected ListType to have 3 fields")
	}
	// TypeMeta
	typeMeta, ok := listValue.Field(0).Interface().(metav1.TypeMeta)
	if !ok {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf("expected TypeMeta field to have TypeMeta type")
	}
	if !listType.Field(0).Anonymous {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf(`expected TypeMeta json field tag to be embedded`)
	}
	if jsonTag, jsonTagExists := listType.Field(0).Tag.Lookup("json"); !jsonTagExists {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf(`expected TypeMeta json field tag`)
	} else if jsonTag != "" && jsonTag != ",inline" {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf(`expected TypeMeta json field tag to be "" or ",inline"`)
	}
	// ListMeta
	listMeta, ok := listValue.Field(1).Interface().(metav1.ListMeta)
	if !ok {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf("expected ListMeta field to have ListMeta type")
	}
	if listType.Field(1).Tag.Get("json") != "metadata,omitempty" {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf(`expected ListMeta json field tag to be "metadata,omitempty"`)
	}
	// Items
	itemsField := listType.Field(2)
	items := listValue.Field(2)
	directItems := itemsField.Name == "Items" && items.Kind() == reflect.Slice
	if directItems && items.Len() > 0 {
		elemType := items.Type().Elem()
		// Keep value-receiver runtime.Objects on ExtractList's eager snapshot;
		// only a pointer-receiver value element can retain its slice header.
		directItems = elemType != rawExtensionObjectType && !elemType.Implements(objectType) && reflect.PointerTo(elemType).Implements(objectType)
	}
	var result listItems
	if directItems {
		// Snapshot the slice header before invoking the caller's writer or an
		// item marshaler. This retains ExtractList's item sequence without
		// allocating an intermediate []runtime.Object for value elements.
		result.direct = items.Slice(0, items.Len())
	} else {
		// Snapshot elements that already implement runtime.Object before
		// invoking the caller's writer or an item marshaler. Unlike value
		// elements with pointer receivers, their backing-array slots can be
		// replaced after a write.
		result.snapshot, err = meta.ExtractList(list)
		if err != nil {
			return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, err
		}
	}
	if itemsField.Tag.Get("json") != "items" {
		return metav1.TypeMeta{}, metav1.ListMeta{}, listItems{}, fmt.Errorf(`expected Items json field tag to be "items"`)
	}
	return typeMeta, listMeta, result, nil
}

// streamEncoder encodes JSON values to w, reusing an internal buffer across
// values to avoid the fresh output allocation json.Marshal makes per call.
type streamEncoder struct {
	w    io.Writer
	buf  bytes.Buffer
	json *json.Encoder
}

func newStreamEncoder(w io.Writer) *streamEncoder {
	e := &streamEncoder{w: w}
	e.json = json.NewEncoder(&e.buf)
	return e
}

func (e *streamEncoder) encodeList(typeMeta metav1.TypeMeta, listMeta metav1.ListMeta, items listItems) error {
	// Start
	if _, err := e.w.Write([]byte(`{`)); err != nil {
		return err
	}

	// TypeMeta
	if typeMeta.Kind != "" {
		if err := e.encodeKeyValuePair("kind", typeMeta.Kind, []byte(",")); err != nil {
			return err
		}
	}
	if typeMeta.APIVersion != "" {
		if err := e.encodeKeyValuePair("apiVersion", typeMeta.APIVersion, []byte(",")); err != nil {
			return err
		}
	}

	// ListMeta
	if err := e.encodeKeyValuePair("metadata", listMeta, []byte(",")); err != nil {
		return err
	}

	// Items
	if items.direct.IsValid() {
		if err := e.encodeItems(items.direct); err != nil {
			return err
		}
	} else if err := e.encodeItemsObjectSlice(items.snapshot); err != nil {
		return err
	}

	// End
	_, err := e.w.Write([]byte("}\n"))
	return err
}

func (e *streamEncoder) encodeItems(items reflect.Value) (err error) {
	if items.IsNil() {
		return e.encodeKeyValuePair("items", nil, nil)
	}
	if _, err = e.w.Write([]byte(`"items":[`)); err != nil {
		return err
	}
	suffix := []byte(",")
	for i := 0; i < items.Len(); i++ {
		if i == items.Len()-1 {
			suffix = nil
		}
		item := items.Index(i).Addr().Interface().(runtime.Object)
		if err = e.encodeValue(item, suffix); err != nil {
			return err
		}
	}
	_, err = e.w.Write([]byte("]"))
	return err
}

// encodeItemsObjectSlice retains ExtractList's direct interface-slice traversal
// for representations that require an eager snapshot.
func (e *streamEncoder) encodeItemsObjectSlice(items []runtime.Object) (err error) {
	if items == nil {
		return e.encodeKeyValuePair("items", nil, nil)
	}
	if _, err = e.w.Write([]byte(`"items":[`)); err != nil {
		return err
	}
	suffix := []byte(",")
	for i, item := range items {
		if i == len(items)-1 {
			suffix = nil
		}
		if err = e.encodeValue(item, suffix); err != nil {
			return err
		}
	}
	_, err = e.w.Write([]byte("]"))
	return err
}

func (e *streamEncoder) encodeUnstructuredList(list *unstructured.UnstructuredList) error {
	_, err := e.w.Write([]byte(`{`))
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(list.Object)+1)
	for key := range list.Object {
		keys = append(keys, key)
	}
	if _, exists := list.Object["items"]; !exists {
		keys = append(keys, "items")
	}
	sort.Strings(keys)

	suffix := []byte(",")
	for i, key := range keys {
		if i == len(keys)-1 {
			suffix = nil
		}
		if key == "items" {
			err = e.encodeItemsUnstructuredSlice(list.Items, suffix)
		} else {
			err = e.encodeKeyValuePair(key, list.Object[key], suffix)
		}
		if err != nil {
			return err
		}
	}
	_, err = e.w.Write([]byte("}\n"))
	return err
}

func (e *streamEncoder) encodeItemsUnstructuredSlice(items []unstructured.Unstructured, suffix []byte) (err error) {
	_, err = e.w.Write([]byte(`"items":[`))
	if err != nil {
		return err
	}
	comma := []byte(",")
	for i, item := range items {
		if i == len(items)-1 {
			comma = nil
		}
		err := e.encodeValue(item.Object, comma)
		if err != nil {
			return err
		}
	}
	_, err = e.w.Write([]byte("]"))
	if err != nil {
		return err
	}
	if len(suffix) > 0 {
		_, err = e.w.Write(suffix)
	}
	return err
}

func (e *streamEncoder) encodeKeyValuePair(key string, value any, suffix []byte) (err error) {
	err = e.encodeValue(key, []byte(":"))
	if err != nil {
		return err
	}
	err = e.encodeValue(value, suffix)
	if err != nil {
		return err
	}
	return err
}

func (e *streamEncoder) encodeValue(value any, suffix []byte) error {
	e.buf.Reset()
	if err := e.json.Encode(value); err != nil {
		return err
	}
	// Encode appends a newline after the value; replace it with the suffix to
	// keep the output identical to json.Marshal's.
	e.buf.Truncate(e.buf.Len() - 1)
	e.buf.Write(suffix)
	_, err := e.w.Write(e.buf.Bytes())
	return err
}
