// SPDX-License-Identifier: Apache-2.0

package nuke

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// mockArena is a simple implementation of the Arena interface for testing purposes.
// It simply allocates memory using Go's built-in make function.
type mockArena struct{}

func (m *mockArena) getTyped(ty reflect.Type, n int) unsafe.Pointer {
	return unsafe.Pointer(&make([]byte, int(ty.Size())*n)[0])
}

func (m *mockArena) getPOD(size uintptr, _ uintptr) unsafe.Pointer {
	return unsafe.Pointer(&make([]byte, int(size))[0])
}

func (m *mockArena) Reset() {
	// Implementation can be empty for this test
}

// TestSliceAppendWithArena tests the SliceAppend function using a mockArena.
func TestSliceAppendWithArena(t *testing.T) {
	a := &mockArena{}

	s := Make[int](a, 3, 3)
	s[0] = 1
	s[1] = 2
	s[2] = 3

	data := []int{4, 5}

	// Append using the mockArena
	result := SliceAppend[int](a, s, data...)

	// Expected slice after appending
	expected := []int{1, 2, 3, 4, 5}

	// Compare the result with the expected slice
	require.Equal(t, expected, result)
}
