/*
Copyright 2023 The Kubernetes Authors.

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
	"reflect"
	goruntime "runtime"
	"strconv"
	"testing"
	"weak"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	testapigroupv1 "k8s.io/apimachinery/pkg/apis/testapigroup/v1"
	internallist "k8s.io/apimachinery/pkg/internal/list"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	fakeObjectItemsNum = 1000
	exemptObjectIndex  = fakeObjectItemsNum / 4
)

type SampleSpec struct {
	Flied int
}

type FooSpec struct {
	Flied int
}

type FooList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []Foo
}

func (s *FooList) DeepCopyObject() runtime.Object { panic("unimplemented") }

type SampleList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []Sample
}

func (s *SampleList) DeepCopyObject() runtime.Object { panic("unimplemented") }

type RawExtensionList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []runtime.RawExtension
}

func (l RawExtensionList) DeepCopyObject() runtime.Object { panic("unimplemented") }

// NOTE: Foo struct itself is the implementer of runtime.Object.
type Foo struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec FooSpec
}

func (f Foo) GetObjectKind() schema.ObjectKind {
	tm := f.TypeMeta
	return &tm
}

func (f Foo) DeepCopyObject() runtime.Object { panic("unimplemented") }

// NOTE: the pointer of Sample that is the implementer of runtime.Object.
// the behavior is similar to our corev1.Pod. corev1.Node
type Sample struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec SampleSpec
}

func (s *Sample) GetObjectKind() schema.ObjectKind {
	tm := s.TypeMeta
	return &tm
}

func (s *Sample) DeepCopyObject() runtime.Object { panic("unimplemented") }

func fakeSampleList(numItems int) *SampleList {
	out := &SampleList{
		Items: make([]Sample, numItems),
	}

	for i := range out.Items {
		out.Items[i] = Sample{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "sample.org/v1",
				Kind:       "Sample",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      strconv.Itoa(i),
				Namespace: "default",
				Labels: map[string]string{
					"label-key-1": "label-value-1",
				},
				Annotations: map[string]string{
					"annotations-key-1": "annotations-value-1",
				},
			},
			Spec: SampleSpec{
				Flied: i,
			},
		}
	}
	return out
}

func fakeExtensionList(numItems int) *RawExtensionList {
	out := &RawExtensionList{
		Items: make([]runtime.RawExtension, numItems),
	}

	for i := range out.Items {
		out.Items[i] = runtime.RawExtension{
			Object: &Foo{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "sample.org/v2",
					Kind:       "Sample",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      strconv.Itoa(i),
					Namespace: "default",
					Labels: map[string]string{
						"label-key-1": "label-value-1",
					},
					Annotations: map[string]string{
						"annotations-key-1": "annotations-value-1",
					},
				},
				Spec: FooSpec{
					Flied: i,
				},
			},
		}
	}
	return out
}

func fakeUnstructuredList(numItems int) runtime.Unstructured {
	out := &unstructured.UnstructuredList{
		Items: make([]unstructured.Unstructured, numItems),
	}

	for i := range out.Items {
		out.Items[i] = unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"creationTimestamp": nil,
					"name":              strconv.Itoa(i),
				},
				"spec": map[string]interface{}{
					"hostname": "example.com",
				},
				"status": map[string]interface{}{},
			},
		}
	}
	return out
}

func fakeFooList(numItems int) *FooList {
	out := &FooList{
		Items: make([]Foo, numItems),
	}

	for i := range out.Items {
		out.Items[i] = Foo{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "foo.org/v1",
				Kind:       "Foo",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      strconv.Itoa(i),
				Namespace: "default",
				Labels: map[string]string{
					"label-key-1": "label-value-1",
				},
				Annotations: map[string]string{
					"annotations-key-1": "annotations-value-1",
				},
			},
			Spec: FooSpec{
				Flied: i,
			},
		}
	}
	return out
}

func TestEachList(t *testing.T) {
	tests := []struct {
		name            string
		generateFunc    func(num int) (list runtime.Object)
		expectObjectNum int
	}{
		{
			name: "StructReceiverList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeFooList(num)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
		{
			name: "PointerReceiverList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeSampleList(num)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
		{
			name: "RawExtensionList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeExtensionList(num)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
		{
			name: "UnstructuredList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeUnstructuredList(fakeObjectItemsNum)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("EachListItem", func(t *testing.T) {
				expectObjectNames := map[string]struct{}{}
				for i := 0; i < tc.expectObjectNum; i++ {
					expectObjectNames[strconv.Itoa(i)] = struct{}{}
				}
				list := tc.generateFunc(tc.expectObjectNum)
				err := EachListItem(list, func(object runtime.Object) error {
					o, err := Accessor(object)
					if err != nil {
						return err
					}
					delete(expectObjectNames, o.GetName())
					return nil
				})
				if err != nil {
					t.Errorf("each list item %#v: %v", list, err)
				}
				if len(expectObjectNames) != 0 {
					t.Fatal("expectObjectNames should be empty")
				}
			})
			t.Run("EachListItemWithAlloc", func(t *testing.T) {
				expectObjectNames := map[string]struct{}{}
				for i := 0; i < tc.expectObjectNum; i++ {
					expectObjectNames[strconv.Itoa(i)] = struct{}{}
				}
				list := tc.generateFunc(tc.expectObjectNum)
				err := EachListItemWithAlloc(list, func(object runtime.Object) error {
					o, err := Accessor(object)
					if err != nil {
						return err
					}
					delete(expectObjectNames, o.GetName())
					return nil
				})
				if err != nil {
					t.Errorf("each list %#v with alloc: %v", list, err)
				}
				if len(expectObjectNames) != 0 {
					t.Fatal("expectObjectNames should be empty")
				}
			})
		})
	}
}

func TestExtractList(t *testing.T) {
	tests := []struct {
		name            string
		generateFunc    func(num int) (list runtime.Object)
		expectObjectNum int
	}{
		{
			name: "StructReceiverList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeFooList(num)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
		{
			name: "PointerReceiverList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeSampleList(num)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
		{
			name: "RawExtensionList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeExtensionList(num)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
		{
			name: "UnstructuredList",
			generateFunc: func(num int) (list runtime.Object) {
				return fakeUnstructuredList(fakeObjectItemsNum)
			},
			expectObjectNum: fakeObjectItemsNum,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("ExtractList", func(t *testing.T) {
				expectObjectNames := map[string]struct{}{}
				for i := 0; i < tc.expectObjectNum; i++ {
					expectObjectNames[strconv.Itoa(i)] = struct{}{}
				}
				list := tc.generateFunc(tc.expectObjectNum)
				objs, err := ExtractList(list)
				if err != nil {
					t.Fatalf("extract list %#v: %v", list, err)
				}
				for i := range objs {
					var (
						o   metav1.Object
						err error
						obj = objs[i]
					)

					if reflect.TypeOf(obj).Kind() == reflect.Struct {
						copy := reflect.New(reflect.TypeOf(obj))
						copy.Elem().Set(reflect.ValueOf(obj))
						o, err = Accessor(copy.Interface())
					} else {
						o, err = Accessor(obj)
					}
					if err != nil {
						t.Fatalf("Accessor object %#v: %v", obj, err)
					}
					delete(expectObjectNames, o.GetName())
				}
				if len(expectObjectNames) != 0 {
					t.Fatal("expectObjectNames should be empty")
				}
			})
			t.Run("ExtractListWithAlloc", func(t *testing.T) {
				expectObjectNames := map[string]struct{}{}
				for i := 0; i < tc.expectObjectNum; i++ {
					expectObjectNames[strconv.Itoa(i)] = struct{}{}
				}
				list := tc.generateFunc(tc.expectObjectNum)
				objs, err := ExtractListWithAlloc(list)
				if err != nil {
					t.Fatalf("extract list with alloc: %v", err)
				}
				for i := range objs {
					var (
						o   metav1.Object
						err error
						obj = objs[i]
					)
					if reflect.TypeOf(obj).Kind() == reflect.Struct {
						copy := reflect.New(reflect.TypeOf(obj))
						copy.Elem().Set(reflect.ValueOf(obj))
						o, err = Accessor(copy.Interface())
					} else {
						o, err = Accessor(obj)
					}
					if err != nil {
						t.Fatalf("Accessor object %#v: %v", obj, err)
					}
					delete(expectObjectNames, o.GetName())
				}
				if len(expectObjectNames) != 0 {
					t.Fatal("expectObjectNames should be empty")
				}
			})
		})
	}
}

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
			list: &rawExtensionList{Items: []runtime.RawExtension{
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
			list:    &invalidList{Items: []string{"not a runtime object"}},
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
		list := &rawExtensionList{Items: []runtime.RawExtension{{Object: first}, {Object: second}}}

		want, err := ExtractList(list)
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
		list := &valueList{Items: []valueObject{{Name: "first"}, {Name: "second"}}}
		want, err := ExtractList(list)
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
		list.Items[1] = valueObject{Name: "replacement"}

		if got := iteratorObjects(iterator); !reflect.DeepEqual(got, want) {
			t.Errorf("ItemIterator after mutation = %#v, want ExtractList snapshot %#v", got, want)
		}
	})

	t.Run("retains pointer receiver backing array", func(t *testing.T) {
		list := carpList("first", "second")
		want, err := ExtractList(list)
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
		list := carpList("first", "second")
		want, err := ExtractList(list)
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
		payload := &retentionPayload{data: make([]byte, 1024)}
		payloadRef := weak.Make(payload)
		list := &retentionList{
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
	itemsPtr, err := GetItemsPtr(list)
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

func carpList(names ...string) *testapigroupv1.CarpList {
	list := &testapigroupv1.CarpList{Items: make([]testapigroupv1.Carp, len(names))}
	for i, name := range names {
		list.Items[i].ObjectMeta.Name = name
	}
	return list
}

type rawExtensionList struct {
	metav1.TypeMeta
	metav1.ListMeta
	Items []runtime.RawExtension
}

func (*rawExtensionList) DeepCopyObject() runtime.Object { return nil }

type invalidList struct {
	Items []string
}

func (*invalidList) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (*invalidList) DeepCopyObject() runtime.Object   { return nil }

type valueList struct {
	Items []valueObject
}

func (*valueList) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (*valueList) DeepCopyObject() runtime.Object   { return nil }

type valueObject struct {
	Name string
}

func (valueObject) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (valueObject) DeepCopyObject() runtime.Object   { return nil }

type retentionList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items   []testapigroupv1.Carp
	payload *retentionPayload
}

func (*retentionList) DeepCopyObject() runtime.Object { return nil }

type retentionPayload struct {
	data []byte
}

func TestLenList(t *testing.T) {
	tests := []struct {
		name string
		list runtime.Object
		want int
	}{
		{
			name: "nil",
			list: nil,
			want: 0,
		},
		{
			name: "empty FooList",
			list: &FooList{},
			want: 0,
		},
		{
			name: "FooList",
			list: fakeFooList(fakeObjectItemsNum),
			want: fakeObjectItemsNum,
		},
		{
			name: "SampleList",
			list: fakeSampleList(fakeObjectItemsNum),
			want: fakeObjectItemsNum,
		},
		{
			name: "RawExtensionList",
			list: fakeExtensionList(fakeObjectItemsNum),
			want: fakeObjectItemsNum,
		},
		{
			name: "UnstructuredList",
			list: fakeUnstructuredList(fakeObjectItemsNum).(*unstructured.UnstructuredList),
			want: fakeObjectItemsNum,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LenList(tc.list); got != tc.want {
				t.Errorf("LenList() = %d, want %d", got, tc.want)
			}
		})
	}
}

func BenchmarkExtractListItem(b *testing.B) {
	tests := []struct {
		name string
		list runtime.Object
	}{
		{
			name: "StructReceiverList",
			list: fakeFooList(fakeObjectItemsNum),
		},
		{
			name: "PointerReceiverList",
			list: fakeSampleList(fakeObjectItemsNum),
		},
		{
			name: "RawExtensionList",
			list: fakeExtensionList(fakeObjectItemsNum),
		},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ExtractList(tc.list)
				if err != nil {
					b.Fatalf("ExtractList: %v", err)
				}
			}
			b.StopTimer()
		})
	}
}

func BenchmarkEachListItem(b *testing.B) {
	tests := []struct {
		name string
		list runtime.Object
	}{
		{
			name: "StructReceiverList",
			list: fakeFooList(fakeObjectItemsNum),
		},
		{
			name: "PointerReceiverList",
			list: fakeSampleList(fakeObjectItemsNum),
		},
		{
			name: "RawExtensionList",
			list: fakeExtensionList(fakeObjectItemsNum),
		},
		{
			name: "UnstructuredList",
			list: fakeUnstructuredList(fakeObjectItemsNum),
		},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := EachListItem(tc.list, func(object runtime.Object) error {
					return nil
				})
				if err != nil {
					b.Fatalf("EachListItem: %v", err)
				}
			}
			b.StopTimer()
		})
	}
}

func BenchmarkExtractListItemWithAlloc(b *testing.B) {
	tests := []struct {
		name string
		list runtime.Object
	}{
		{
			name: "StructReceiverList",
			list: fakeFooList(fakeObjectItemsNum),
		},
		{
			name: "PointerReceiverList",
			list: fakeSampleList(fakeObjectItemsNum),
		},
		{
			name: "RawExtensionList",
			list: fakeExtensionList(fakeObjectItemsNum),
		},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ExtractListWithAlloc(tc.list)
				if err != nil {
					b.Fatalf("ExtractListWithAlloc: %v", err)
				}
			}
			b.StopTimer()
		})
	}
}

func BenchmarkEachListItemWithAlloc(b *testing.B) {
	tests := []struct {
		name string
		list runtime.Object
	}{
		{
			name: "StructReceiverList",
			list: fakeFooList(fakeObjectItemsNum),
		},
		{
			name: "PointerReceiverList",
			list: fakeSampleList(fakeObjectItemsNum),
		},
		{
			name: "RawExtensionList",
			list: fakeExtensionList(fakeObjectItemsNum),
		},
		{
			name: "UnstructuredList",
			list: fakeUnstructuredList(fakeObjectItemsNum),
		},
	}
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := EachListItemWithAlloc(tc.list, func(object runtime.Object) error {
					return nil
				})
				if err != nil {
					b.Fatalf("EachListItemWithAlloc: %v", err)
				}
			}
			b.StopTimer()
		})
	}
}
