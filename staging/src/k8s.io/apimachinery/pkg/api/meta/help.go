/*
Copyright 2015 The Kubernetes Authors.

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

package meta

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// isListCache maintains a cache of types that are checked for lists
	// which is used by IsListType.
	// TODO: remove and replace with an interface check
	isListCache = struct {
		lock   sync.RWMutex
		byType map[reflect.Type]bool
	}{
		byType: make(map[reflect.Type]bool, 1024),
	}
)

// IsListType returns true if the provided Object has a slice called Items.
// TODO: Replace the code in this check with an interface comparison by
// creating and enforcing that lists implement a list accessor.
func IsListType(obj runtime.Object) bool {
	switch t := obj.(type) {
	case runtime.Unstructured:
		return t.IsList()
	}
	t := reflect.TypeOf(obj)

	isListCache.lock.RLock()
	ok, exists := isListCache.byType[t]
	isListCache.lock.RUnlock()

	if !exists {
		_, err := getItemsPtr(obj)
		ok = err == nil

		// cache only the first 1024 types
		isListCache.lock.Lock()
		if len(isListCache.byType) < 1024 {
			isListCache.byType[t] = ok
		}
		isListCache.lock.Unlock()
	}

	return ok
}

var (
	errExpectFieldItems = errors.New("no Items field in this object")
	errExpectSliceItems = errors.New("Items field must be a slice of objects")
)

// GetItemsPtr returns a pointer to the list object's Items member.
// If 'list' doesn't have an Items member, it's not really a list type
// and an error will be returned.
// This function will either return a pointer to a slice, or an error, but not both.
// TODO: this will be replaced with an interface in the future
func GetItemsPtr(list runtime.Object) (interface{}, error) {
	obj, err := getItemsPtr(list)
	if err != nil {
		return nil, fmt.Errorf("%T is not a list: %v", list, err)
	}
	return obj, nil
}

// getItemsPtr returns a pointer to the list object's Items member or an error.
func getItemsPtr(list runtime.Object) (interface{}, error) {
	v, err := conversion.EnforcePtr(list)
	if err != nil {
		return nil, err
	}

	items := v.FieldByName("Items")
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

// EachListItem invokes fn on each runtime.Object in the list. Any error immediately terminates
// the loop.
//
// If items passed to fn are retained for different durations, and you want to avoid
// retaining all items in obj as long as any item is referenced, use EachListItemWithAlloc instead.
func EachListItem(obj runtime.Object, fn func(runtime.Object) error) error {
	return eachListItem(obj, fn, false)
}

// EachListItemWithAlloc works like EachListItem, but avoids retaining references to the items slice in obj.
// It does this by making a shallow copy of non-pointer items in obj.
//
// If the items passed to fn are not retained, or are retained for the same duration, use EachListItem instead for memory efficiency.
func EachListItemWithAlloc(obj runtime.Object, fn func(runtime.Object) error) error {
	return eachListItem(obj, fn, true)
}

// allocNew: Whether shallow copy is required when the elements in Object.Items are struct
func eachListItem(obj runtime.Object, fn func(runtime.Object) error, allocNew bool) error {
	if unstructured, ok := obj.(runtime.Unstructured); ok {
		if allocNew {
			return unstructured.EachListItemWithAlloc(fn)
		}
		return unstructured.EachListItem(fn)
	}
	// TODO: Change to an interface call?
	itemsPtr, err := GetItemsPtr(obj)
	if err != nil {
		return err
	}
	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return err
	}
	len := items.Len()
	if len == 0 {
		return nil
	}
	takeAddr := false
	if elemType := items.Type().Elem(); elemType.Kind() != reflect.Pointer && elemType.Kind() != reflect.Interface {
		if !items.Index(0).CanAddr() {
			return fmt.Errorf("unable to take address of items in %T for EachListItem", obj)
		}
		takeAddr = true
	}

	for i := 0; i < len; i++ {
		raw := items.Index(i)
		if takeAddr {
			if allocNew {
				// shallow copy to avoid retaining a reference to the original list item
				itemCopy := reflect.New(raw.Type())
				// assign to itemCopy and type-assert
				itemCopy.Elem().Set(raw)
				// reflect.New will guarantee that itemCopy must be a pointer.
				raw = itemCopy
			} else {
				raw = raw.Addr()
			}
		}
		// raw must be a pointer or an interface
		// allocate a pointer is cheap
		switch item := raw.Interface().(type) {
		case *runtime.RawExtension:
			if err := fn(item.Object); err != nil {
				return err
			}
		case runtime.Object:
			if err := fn(item); err != nil {
				return err
			}
		default:
			obj, ok := item.(runtime.Object)
			if !ok {
				return fmt.Errorf("%v: item[%v]: Expected object, got %#v(%s)", obj, i, raw.Interface(), raw.Kind())
			}
			if err := fn(obj); err != nil {
				return err
			}
		}
	}
	return nil
}

// ExtractList returns obj's Items element as an array of runtime.Objects.
// Returns an error if obj is not a List type (does not have an Items member).
//
// If items in the returned list are retained for different durations, and you want to avoid
// retaining all items in obj as long as any item is referenced, use ExtractListWithAlloc instead.
func ExtractList(obj runtime.Object) ([]runtime.Object, error) {
	return extractList(obj, false)
}

// ExtractListWithAlloc works like ExtractList, but avoids retaining references to the items slice in obj.
// It does this by making a shallow copy of non-pointer items in obj.
//
// If the items in the returned list are not retained, or are retained for the same duration, use ExtractList instead for memory efficiency.
func ExtractListWithAlloc(obj runtime.Object) ([]runtime.Object, error) {
	return extractList(obj, true)
}

// ListItemIterator provides access to the objects ExtractList would return
// without always materializing a []runtime.Object result.
type ListItemIterator struct {
	extractor listItemExtractor
	items     []runtime.Object
}

// NewListItemIterator returns a validated iterator over obj's Items field. It returns
// true when obj's Items field is nil, in which case the returned iterator has length zero.
// It preserves ExtractList's item representations: conversions that capture an item value do
// so before this function returns, while pointer-based items retain their Items backing array.
func NewListItemIterator(obj runtime.Object) (ListItemIterator, bool, error) {
	extractor, itemsNil, err := newListItemExtractor(obj, false)
	if err != nil || itemsNil {
		return ListItemIterator{}, itemsNil, err
	}
	if err := extractor.validate(); err != nil {
		return ListItemIterator{}, false, err
	}
	if !extractor.requiresSnapshot() {
		// Keep the Items slice header stable just as ExtractList's pointers retain
		// the backing array that was present when extraction began.
		extractor.items = reflect.ValueOf(extractor.items.Interface())
		return ListItemIterator{extractor: extractor}, false, nil
	}
	iterator := ListItemIterator{extractor: extractor}
	iterator.items = make([]runtime.Object, extractor.items.Len())
	for i := range iterator.items {
		iterator.items[i] = extractor.item(i)
	}
	return iterator, false, nil
}

// Len returns the number of objects in the iterator.
func (i ListItemIterator) Len() int {
	if i.items != nil {
		return len(i.items)
	}
	if !i.extractor.items.IsValid() {
		return 0
	}
	return i.extractor.items.Len()
}

// Item returns the object at index. It panics if index is out of range.
func (i ListItemIterator) Item(index int) runtime.Object {
	if i.items != nil {
		return i.items[index]
	}
	return i.extractor.item(index)
}

// allocNew: Whether shallow copy is required when the elements in Object.Items are struct
func extractList(obj runtime.Object, allocNew bool) ([]runtime.Object, error) {
	extractor, itemsNil, err := newListItemExtractor(obj, allocNew)
	if err != nil || itemsNil {
		return nil, err
	}
	if err := extractor.validate(); err != nil {
		return nil, err
	}
	list := make([]runtime.Object, extractor.items.Len())
	for i := range list {
		list[i] = extractor.item(i)
	}
	return list, nil
}

type listItemExtractor struct {
	obj              runtime.Object
	items            reflect.Value
	isRawExtension   bool
	implementsObject bool
	allocNew         bool
}

func newListItemExtractor(obj runtime.Object, allocNew bool) (listItemExtractor, bool, error) {
	itemsPtr, err := GetItemsPtr(obj)
	if err != nil {
		return listItemExtractor{}, false, err
	}
	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return listItemExtractor{}, false, err
	}
	if items.IsNil() {
		return listItemExtractor{}, true, nil
	}
	elemType := items.Type().Elem()
	return listItemExtractor{
		obj:              obj,
		items:            items,
		isRawExtension:   elemType == rawExtensionObjectType,
		implementsObject: elemType.Implements(objectType),
		allocNew:         allocNew,
	}, false, nil
}

func (e listItemExtractor) requiresSnapshot() bool {
	return e.isRawExtension || e.implementsObject
}

func (e listItemExtractor) validate() error {
	if e.items.Len() == 0 || e.requiresSnapshot() {
		return nil
	}
	raw := e.items.Index(0)
	if e.allocNew {
		if reflect.PointerTo(raw.Type()).Implements(objectType) {
			return nil
		}
	} else if raw.Addr().Type().Implements(objectType) {
		return nil
	}
	return e.itemError(0, raw)
}

func (e listItemExtractor) item(i int) runtime.Object {
	raw := e.items.Index(i)
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
	case e.allocNew:
		// shallow copy to avoid retaining a reference to the original list item
		itemCopy := reflect.New(raw.Type())
		// assign to itemCopy and type-assert
		itemCopy.Elem().Set(raw)
		// reflect.New will guarantee that itemCopy must be a pointer.
		return itemCopy.Interface().(runtime.Object)
	default:
		return raw.Addr().Interface().(runtime.Object)
	}
}

func (e listItemExtractor) itemError(i int, raw reflect.Value) error {
	return fmt.Errorf("%v: item[%v]: Expected object, got %#v(%s)", e.obj, i, raw.Interface(), raw.Kind())
}

var (
	// objectSliceType is the type of a slice of Objects
	objectSliceType        = reflect.TypeOf([]runtime.Object{})
	objectType             = reflect.TypeOf((*runtime.Object)(nil)).Elem()
	rawExtensionObjectType = reflect.TypeOf(runtime.RawExtension{})
)

// LenList returns the length of this list or 0 if it is not a list.
func LenList(list runtime.Object) int {
	itemsPtr, err := GetItemsPtr(list)
	if err != nil {
		return 0
	}
	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return 0
	}
	return items.Len()
}

// SetList sets the given list object's Items member have the elements given in
// objects.
// Returns an error if list is not a List type (does not have an Items member),
// or if any of the objects are not of the right type.
func SetList(list runtime.Object, objects []runtime.Object) error {
	itemsPtr, err := GetItemsPtr(list)
	if err != nil {
		return err
	}
	items, err := conversion.EnforcePtr(itemsPtr)
	if err != nil {
		return err
	}
	if items.Type() == objectSliceType {
		items.Set(reflect.ValueOf(objects))
		return nil
	}
	slice := reflect.MakeSlice(items.Type(), len(objects), len(objects))
	for i := range objects {
		dest := slice.Index(i)
		if dest.Type() == rawExtensionObjectType {
			dest = dest.FieldByName("Object")
		}

		// check to see if you're directly assignable
		if reflect.TypeOf(objects[i]).AssignableTo(dest.Type()) {
			dest.Set(reflect.ValueOf(objects[i]))
			continue
		}

		src, err := conversion.EnforcePtr(objects[i])
		if err != nil {
			return err
		}
		if src.Type().AssignableTo(dest.Type()) {
			dest.Set(src)
		} else if src.Type().ConvertibleTo(dest.Type()) {
			dest.Set(src.Convert(dest.Type()))
		} else {
			return fmt.Errorf("item[%d]: can't assign or convert %v into %v", i, src.Type(), dest.Type())
		}
	}
	items.Set(slice)
	return nil
}
