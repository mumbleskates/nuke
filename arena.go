// SPDX-License-Identifier: Apache-2.0

package nuke

import (
	"reflect"
	"unsafe"
)

// Arena is an interface that describes a memory allocation arena.
type Arena interface {
	// Reset resets the arena's state.
	Reset()

	getTyped(reflect.Type, int) unsafe.Pointer
	getPOD(size uintptr, align uintptr) unsafe.Pointer
}

// New allocates memory for a value of type T using the provided Arena.
// If the arena is non-nil, it returns a  *T pointer with memory allocated from
// the arena. If passed arena is nil, it allocates memory using Go's built-in
// new function.
func New[T any](arena Arena) (res *T) {
	if arena != nil {
		res = (*T)(arena.getTyped(reflect.TypeFor[T](), 1))
	}
	if res == nil {
		return new(T)
	}
	return
}

func NewPOD[T any](arena Arena) (res *T) {
	if arena != nil {
		var t T
		res = (*T)(arena.getPOD(unsafe.Sizeof(t), unsafe.Alignof(t)))
	}
	if res == nil {
		return new(T)
	}
	return
}

// MakeSlice creates a slice of type T with a given length and capacity,
// using the provided Arena for memory allocation.
// If the arena is non-nil, it returns a slice with memory allocated from the arena.
// Otherwise, it returns a slice using Go's built-in make function.
func Make[T any](arena Arena, length int, cap int) []T {
	var ptr *T
	if arena != nil {
		ptr = (*T)(arena.getTyped(reflect.TypeFor[T](), cap))
	}
	if ptr == nil {
		return make([]T, length, cap)
	}
	return unsafe.Slice(ptr, cap)[:length]
}

// Make space for any type in the arena, with a user-declared guarantee that
// the type contains no pointers.
func MakePOD[T any](arena Arena, length int, cap int) []T {
	var ptr *T
	if arena != nil {
		var t T
		ptr = (*T)(arena.getPOD(unsafe.Sizeof(t)*uintptr(cap), unsafe.Alignof(t)))
	}
	if ptr == nil {
		return make([]T, length, cap)
	}
	return unsafe.Slice(ptr, cap)[:length]
}
