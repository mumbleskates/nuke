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
	getPOD(size uintptr, align uintptr, n int) unsafe.Pointer
}

// New allocates memory for a value of type T using the provided Arena.
// If the arena is non-nil, it returns a  *T pointer with memory allocated from
// the arena. If passed arena is nil, it allocates memory using Go's built-in
// new function.
func New[T any](arena Arena) *T {
	if arena != nil {
		return (*T)(arena.getTyped(reflect.TypeFor[T](), 1))
	}
	return new(T)
}

func NewPOD[T any](arena Arena) *T {
	if arena != nil {
		var t T
		return (*T)(arena.getPOD(unsafe.Sizeof(t), unsafe.Alignof(t), 1))
	}
	return new(T)
}

// MakeSlice creates a slice of type T with a given length and capacity,
// using the provided Arena for memory allocation.
// If the arena is non-nil, it returns a slice with memory allocated from the arena.
// Otherwise, it returns a slice using Go's built-in make function.
func Make[T any](arena Arena, n int, cap int) []T {
	if arena != nil {
		ptr := (*T)(arena.getTyped(reflect.TypeFor[T](), n))
		return unsafe.Slice(ptr, cap)[:n]
	}
	return make([]T, n, cap)
}

// Make space for any type in the arena, with a user-declared guarantee that
// the type contains no pointers.
func MakePOD[T any](arena Arena, n int, cap int) []T {
	if arena != nil {
		var t T
		ptr := (*T)(arena.getPOD(unsafe.Sizeof(t), unsafe.Alignof(t), n))
		return unsafe.Slice(ptr, cap)[:n]
	}
	return make([]T, n, cap)
}
