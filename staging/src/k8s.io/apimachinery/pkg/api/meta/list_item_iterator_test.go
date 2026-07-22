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

package meta_test

import (
	"reflect"
	goruntime "runtime"
	"testing"
	"weak"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testapigroupv1 "k8s.io/apimachinery/pkg/apis/testapigroup/v1"
	internallist "k8s.io/apimachinery/pkg/internal/list"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestListItemIterator(t *testing.T) {
	rawObject := &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "object"}}
	raw := []byte(`{"metadata":{"name":"raw"}}`)
	tests := []struct {
		name     string
		list     runtime.Object
		itemsNil bool
		want     []runtime.Object
		wantErr  bool
	}{
		{
			name:     "nil items",
			list:     &testapigroupv1.CarpList{},
			itemsNil: true,
		},
		{
			name: "empty items",
			list: &testapigroupv1.CarpList{Items: []testapigroupv1.Carp{}},
			want: []runtime.Object{},
		},
		{
			name: "raw extensions",
			list: &iteratorRawExtensionList{Items: []runtime.RawExtension{
				{Object: rawObject},
				{Raw: raw},
				{},
			}},
			want: []runtime.Object{
				rawObject,
				&runtime.Unknown{Raw: raw},
				nil,
			},
		},
		{
			name:    "invalid items",
			list:    &iteratorInvalidList{Items: []string{"not a runtime object"}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iterator, itemsNil, err := newListItemIterator(tc.list)
			if (err != nil) != tc.wantErr {
				t.Fatalf("internallist.NewItemIterator() error = %v, want error = %t", err, tc.wantErr)
			}
			if itemsNil != tc.itemsNil {
				t.Errorf("internallist.NewItemIterator() itemsNil = %t, want %t", itemsNil, tc.itemsNil)
			}
			if tc.wantErr || itemsNil {
				if itemsNil && iterator.Len() != 0 {
					t.Errorf("Len() = %d, want 0", iterator.Len())
				}
				return
			}
			if got := iteratorObjects(iterator); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ItemIterator = %#v, want %#v", got, tc.want)
			}
		})
	}

	t.Run("snapshots raw extension items", func(t *testing.T) {
		first := &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "first"}}
		second := &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "second"}}
		replacement := &testapigroupv1.Carp{ObjectMeta: metav1.ObjectMeta{Name: "replacement"}}
		list := &iteratorRawExtensionList{Items: []runtime.RawExtension{{Object: first}, {Object: second}}}

		want, err := meta.ExtractList(list)
		if err != nil {
			t.Fatalf("ExtractList: %v", err)
		}
		iterator, itemsNil, err := newListItemIterator(list)
		if err != nil {
			t.Fatalf("NewItemIterator: %v", err)
		}
		if itemsNil {
			t.Fatal("internallist.NewItemIterator() itemsNil = true, want false")
		}
		list.Items[1].Object = replacement

		if got := iteratorObjects(iterator); !reflect.DeepEqual(got, want) {
			t.Errorf("ItemIterator after mutation = %#v, want ExtractList snapshot %#v", got, want)
		}
	})

	t.Run("snapshots value receiver items", func(t *testing.T) {
		list := &iteratorValueList{Items: []iteratorValueObject{{Name: "first"}, {Name: "second"}}}
		want, err := meta.ExtractList(list)
		if err != nil {
			t.Fatalf("ExtractList: %v", err)
		}
		iterator, itemsNil, err := newListItemIterator(list)
		if err != nil {
			t.Fatalf("NewItemIterator: %v", err)
		}
		if itemsNil {
			t.Fatal("internallist.NewItemIterator() itemsNil = true, want false")
		}
		list.Items[1] = iteratorValueObject{Name: "replacement"}

		if got := iteratorObjects(iterator); !reflect.DeepEqual(got, want) {
			t.Errorf("ItemIterator after mutation = %#v, want ExtractList snapshot %#v", got, want)
		}
	})

	t.Run("retains pointer receiver backing array", func(t *testing.T) {
		list := iteratorCarpListWithNames("first", "second")
		want, err := meta.ExtractList(list)
		if err != nil {
			t.Fatalf("ExtractList: %v", err)
		}
		iterator, itemsNil, err := newListItemIterator(list)
		if err != nil {
			t.Fatalf("NewItemIterator: %v", err)
		}
		if itemsNil {
			t.Fatal("internallist.NewItemIterator() itemsNil = true, want false")
		}
		list.Items = []testapigroupv1.Carp{{ObjectMeta: metav1.ObjectMeta{Name: "replacement"}}}

		if got := iteratorObjects(iterator); !reflect.DeepEqual(got, want) {
			t.Errorf("ItemIterator after replacing Items = %#v, want ExtractList snapshot %#v", got, want)
		}
	})

	t.Run("matches pointer receiver item mutation", func(t *testing.T) {
		list := iteratorCarpListWithNames("first", "second")
		want, err := meta.ExtractList(list)
		if err != nil {
			t.Fatalf("ExtractList: %v", err)
		}
		iterator, itemsNil, err := newListItemIterator(list)
		if err != nil {
			t.Fatalf("NewItemIterator: %v", err)
		}
		if itemsNil {
			t.Fatal("internallist.NewItemIterator() itemsNil = true, want false")
		}
		list.Items[1].ObjectMeta.Name = "replacement"

		if got := iteratorObjects(iterator); !reflect.DeepEqual(got, want) {
			t.Errorf("ItemIterator after item mutation = %#v, want ExtractList result %#v", got, want)
		}
	})

	t.Run("does not retain list container", func(t *testing.T) {
		payload := &iteratorRetentionPayload{data: make([]byte, 1024)}
		payloadRef := weak.Make(payload)
		list := &iteratorRetentionList{
			Items: []testapigroupv1.Carp{
				{ObjectMeta: metav1.ObjectMeta{Name: "first"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "second"}},
			},
			payload: payload,
		}
		iterator, itemsNil, err := newListItemIterator(list)
		if err != nil {
			t.Fatalf("NewItemIterator: %v", err)
		}
		if itemsNil {
			t.Fatal("internallist.NewItemIterator() itemsNil = true, want false")
		}
		list.Items = []testapigroupv1.Carp{{ObjectMeta: metav1.ObjectMeta{Name: "replacement"}}}
		payload = nil
		list = nil

		// Keep the iterator live through GC while no reference to the list remains.
		for i := 0; i < 10 && payloadRef.Value() != nil; i++ {
			goruntime.GC()
		}
		goruntime.KeepAlive(iterator)
		if payloadRef.Value() != nil {
			t.Fatal("iterator retained payload reachable only through list")
		}
		for index, want := range []string{"first", "second"} {
			item, ok := iterator.Item(index).(*testapigroupv1.Carp)
			if !ok {
				t.Fatalf("Item(%d) = %T, want *testapigroupv1.Carp", index, iterator.Item(index))
			}
			if item.Name != want {
				t.Errorf("Item(%d) name = %q, want %q", index, item.Name, want)
			}
		}
	})
}

func newListItemIterator(list runtime.Object) (internallist.ItemIterator, bool, error) {
	itemsPtr, err := meta.GetItemsPtr(list)
	if err != nil {
		return internallist.ItemIterator{}, false, err
	}
	return internallist.NewItemIterator(itemsPtr)
}

func iteratorObjects(iterator internallist.ItemIterator) []runtime.Object {
	objects := make([]runtime.Object, iterator.Len())
	for i := range objects {
		objects[i] = iterator.Item(i)
	}
	return objects
}

func iteratorCarpListWithNames(names ...string) *testapigroupv1.CarpList {
	list := &testapigroupv1.CarpList{Items: make([]testapigroupv1.Carp, len(names))}
	for i, name := range names {
		list.Items[i].ObjectMeta.Name = name
	}
	return list
}

type iteratorRawExtensionList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []runtime.RawExtension
}

func (*iteratorRawExtensionList) DeepCopyObject() runtime.Object { return nil }

type iteratorInvalidList struct {
	Items []string
}

func (*iteratorInvalidList) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (*iteratorInvalidList) DeepCopyObject() runtime.Object   { return nil }

type iteratorValueList struct {
	Items []iteratorValueObject
}

func (*iteratorValueList) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (*iteratorValueList) DeepCopyObject() runtime.Object   { return nil }

type iteratorValueObject struct {
	Name string
}

func (iteratorValueObject) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (iteratorValueObject) DeepCopyObject() runtime.Object   { return nil }

type iteratorRetentionList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items   []testapigroupv1.Carp
	payload *iteratorRetentionPayload
}

func (*iteratorRetentionList) DeepCopyObject() runtime.Object { return nil }

type iteratorRetentionPayload struct {
	data []byte
}
