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

// Package list provides internal access to typed list items.
package list

import (
	"errors"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	errExpectFieldItems = errors.New("no Items field in this object")
	errExpectSliceItems = errors.New("Items field must be a slice of objects")

	objectType       = reflect.TypeOf((*runtime.Object)(nil)).Elem()
	rawExtensionType = reflect.TypeOf(runtime.RawExtension{})
)

// ItemIterator provides the objects meta.ExtractList would return without
// materializing a []runtime.Object result for pointer-receiver item lists.
// RawExtension and value-receiver items are snapshotted to preserve ExtractList
// semantics. Streaming serializers validate list items before writing output.
type ItemIterator struct {
	extractor itemExtractor
	items     []runtime.Object
}

// NewItemIterator returns a validated iterator over obj's Items field. It
// reports whether Items is nil. RawExtension and value-receiver items are
// converted before it returns, while pointer-receiver items retain their Items
// backing array.
func NewItemIterator(obj runtime.Object) (ItemIterator, bool, error) {
	extractor, itemsNil, err := newItemExtractor(obj)
	if err != nil || itemsNil {
		return ItemIterator{}, itemsNil, err
	}
	if err := extractor.validate(obj); err != nil {
		return ItemIterator{}, false, err
	}
	if !extractor.requiresSnapshot() {
		// Keep the Items slice header stable just as ExtractList's pointers retain
		// the backing array that was present when extraction began.
		extractor.items = reflect.ValueOf(extractor.items.Interface())
		return ItemIterator{extractor: extractor}, false, nil
	}
	iterator := ItemIterator{items: make([]runtime.Object, extractor.items.Len())}
	for i := range iterator.items {
		iterator.items[i] = extractor.item(i)
	}
	return iterator, false, nil
}

// Len returns the number of objects in the iterator.
func (i ItemIterator) Len() int {
	if i.items != nil {
		return len(i.items)
	}
	if !i.extractor.items.IsValid() {
		return 0
	}
	return i.extractor.items.Len()
}

// Item returns the object at index. It panics if index is out of range.
func (i ItemIterator) Item(index int) runtime.Object {
	if i.items != nil {
		return i.items[index]
	}
	return i.extractor.item(index)
}

type itemExtractor struct {
	items            reflect.Value
	isRawExtension   bool
	implementsObject bool
}

func newItemExtractor(obj runtime.Object) (itemExtractor, bool, error) {
	itemsPtr, err := GetItemsPtr(obj)
	if err != nil {
		return itemExtractor{}, false, err
	}
	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return itemExtractor{}, false, err
	}
	if items.IsNil() {
		return itemExtractor{}, true, nil
	}
	elemType := items.Type().Elem()
	return itemExtractor{
		items:            items,
		isRawExtension:   elemType == rawExtensionType,
		implementsObject: elemType.Implements(objectType),
	}, false, nil
}

// GetItemsPtr returns a pointer to the list object's Items member.
// If list does not have an Items member, it returns an error.
func GetItemsPtr(list runtime.Object) (interface{}, error) {
	items, err := getItemsPtr(list)
	if err != nil {
		return nil, fmt.Errorf("%T is not a list: %v", list, err)
	}
	return items, nil
}

func getItemsPtr(list runtime.Object) (interface{}, error) {
	value, err := conversion.EnforcePtr(list)
	if err != nil {
		return nil, err
	}

	items := value.FieldByName("Items")
	if !items.IsValid() {
		return nil, errExpectFieldItems
	}
	switch items.Kind() {
	case reflect.Interface, reflect.Pointer:
		target := reflect.TypeOf(items.Interface()).Elem()
		if target.Kind() != reflect.Slice {
			return nil, errExpectSliceItems
		}
		return items.Interface(), nil
	case reflect.Slice:
		return items.Addr().Interface(), nil
	default:
		return nil, errExpectSliceItems
	}
}

func (e itemExtractor) requiresSnapshot() bool {
	return e.isRawExtension || e.implementsObject
}

func (e itemExtractor) validate(obj runtime.Object) error {
	if e.items.Len() == 0 || e.requiresSnapshot() {
		return nil
	}
	raw := e.items.Index(0)
	if raw.Addr().Type().Implements(objectType) {
		return nil
	}
	return fmt.Errorf("%v: item[%v]: Expected object, got %#v(%s)", obj, 0, raw.Interface(), raw.Kind())
}

func (e itemExtractor) item(index int) runtime.Object {
	raw := e.items.Index(index)
	switch {
	case e.isRawExtension:
		item := raw.Interface().(runtime.RawExtension)
		switch {
		case item.Object != nil:
			return item.Object
		case item.Raw != nil:
			// TODO: Set ContentEncoding and ContentType correctly.
			return &runtime.Unknown{Raw: item.Raw}
		default:
			return nil
		}
	case e.implementsObject:
		return raw.Interface().(runtime.Object)
	default:
		return raw.Addr().Interface().(runtime.Object)
	}
}
