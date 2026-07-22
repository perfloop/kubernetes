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
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
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

// NewItemIterator returns a validated iterator over the Items slice referenced
// by itemsPtr. It reports whether Items is nil. RawExtension and value-receiver
// items are converted before it returns, while pointer-receiver items retain
// their Items backing array.
func NewItemIterator(itemsPtr interface{}) (ItemIterator, bool, error) {
	extractor, itemsNil, err := newItemExtractor(itemsPtr)
	if err != nil || itemsNil {
		return ItemIterator{}, itemsNil, err
	}
	if err := extractor.validate(); err != nil {
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

func newItemExtractor(itemsPtr interface{}) (itemExtractor, bool, error) {
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

func (e itemExtractor) requiresSnapshot() bool {
	return e.isRawExtension || e.implementsObject
}

func (e itemExtractor) validate() error {
	if e.items.Len() == 0 || e.requiresSnapshot() {
		return nil
	}
	raw := e.items.Index(0)
	if raw.Addr().Type().Implements(objectType) {
		return nil
	}
	return fmt.Errorf("item[%v]: Expected object, got %#v(%s)", 0, raw.Interface(), raw.Kind())
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
