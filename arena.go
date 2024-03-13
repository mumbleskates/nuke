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
	getPOD(reflect.Type, int) unsafe.Pointer
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
		return (*T)(arena.getPOD(reflect.TypeFor[T](), 1))
	}
	return new(T)
}

// MakeSlice creates a slice of type T with a given length and capacity,
// using the provided Arena for memory allocation.
// If the arena is non-nil, it returns a slice with memory allocated from the arena.
// Otherwise, it returns a slice using Go's built-in make function.
func Make[T any](arena Arena, n int) []T {
	if arena != nil {
		ptr := (*T)(arena.getTyped(reflect.TypeFor[T](), n))
		return unsafe.Slice(ptr, n)
	}
	return make([]T, n)
}

// Make space for any type in the arena, with a user-declared guarantee that
// the type contains no pointers.
func MakePOD[T any](arena Arena, n int) []T {
	if arena != nil {
		ptr := (*T)(arena.getPOD(reflect.TypeFor[T](), n))
		return unsafe.Slice(ptr, n)
	}
	return make([]T, n)
}
