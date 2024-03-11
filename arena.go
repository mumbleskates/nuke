// SPDX-License-Identifier: Apache-2.0

package nuke

import (
	"unsafe"
)

// Arena is an interface that describes a memory allocation arena.
type Arena interface {
	// Reset resets the arena's state.
	Reset()
}

// New allocates memory for a value of type T using the provided Arena.
// If the arena is non-nil, it returns a  *T pointer with memory allocated from the arena.
// If passed arena is nil, it allocates memory using Go's built-in new function.
func New[T any](a Arena) *T {
	if a != nil {
		var x T
		if ptr := make(unsafe.Sizeof(x), unsafe.Alignof(x)); ptr != nil {
			return (*T)(ptr)
		}
	}
	return new(T)
}

// TODO(widders): public apis are the only generic part. they have to do the
//  real allocations themselves in those functions, and the arena types have to
//  support all of those actions via private interface methods.

// MakeSlice creates a slice of type T with a given length and capacity,
// using the provided Arena for memory allocation.
// If the arena is non-nil, it returns a slice with memory allocated from the arena.
// Otherwise, it returns a slice using Go's built-in make function.
func MakeSlice[T any](a Arena, len, cap int) []T {
	if a != nil {
		var x T
		bufSize := int(unsafe.Sizeof(x)) * cap
		if ptr := (*T)(a.Alloc(uintptr(bufSize), unsafe.Alignof(x))); ptr != nil {
			s := unsafe.Slice(ptr, cap)
			return s[:len]
		}
	}
	return make([]T, len, cap)
}
