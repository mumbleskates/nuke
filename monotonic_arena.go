// SPDX-License-Identifier: Apache-2.0

package nuke

import (
	"reflect"
	"unsafe"
)

type monotonicArena struct {
	buffers          []*monotonicBuffer
	releaseWhenReset bool
}

type monotonicBuffer struct {
	ptr    unsafe.Pointer
	offset uintptr
	size   uintptr
}

func newMonotonicBuffer(size int) *monotonicBuffer {
	return &monotonicBuffer{size: uintptr(size)}
}

func (s *monotonicBuffer) alloc(size, alignment uintptr) (unsafe.Pointer, bool) {
	if s.ptr == nil {
		buf := make([]byte, s.size) // allocate monotonic buffer lazily
		s.ptr = unsafe.Pointer(unsafe.SliceData(buf))
	}

	// Align the address of the region we will hand out
	var realign uintptr
	if alignment != 1 {
		misalignment := (uintptr(s.ptr) + s.offset) % alignment
		if misalignment != 0 {
			realign = alignment - misalignment
		}
	}
	allocSize := size + realign

	if s.availableBytes() < allocSize {
		return nil, false
	}
	ptr := unsafe.Pointer(uintptr(s.ptr) + s.offset + realign)
	s.offset += allocSize

	return ptr, true
}

func (s *monotonicBuffer) reset(release bool) {
	if s.offset == 0 {
		return
	}
	s.offset = 0

	if release {
		s.ptr = nil
	} else {
		s.zeroOutBuffer()
	}
}

func (s *monotonicBuffer) zeroOutBuffer() {
	buf := (*byte)(s.ptr)
	b := unsafe.Slice(buf, s.offset)

	// This piece of code will be translated into a runtime.memclrNoHeapPointers
	// invocation by the compiler, which is an assembler optimized implementation.
	// Architecture specific code can be found at src/runtime/memclr_$GOARCH.s
	// in Go source (since https://codereview.appspot.com/137880043).
	for i := range b {
		b[i] = 0
	}
}

func (s *monotonicBuffer) availableBytes() uintptr {
	return s.size - s.offset
}

// NewMonotonicArena creates a new monotonic arena with a specified number of buffers and a buffer size.
func NewMonotonicArena(bufferSize, bufferCount int) Arena {
	a := &monotonicArena{}
	for i := 0; i < bufferCount; i++ {
		a.buffers = append(a.buffers, newMonotonicBuffer(bufferSize))
	}
	return a
}

// NewMonotonicArena creates a new monotonic arena with a specified number of buffers and a buffer size.
func NewMonotonicArenaWithDiscard(bufferSize, bufferCount int) Arena {
	a := &monotonicArena{releaseWhenReset: true}
	for i := 0; i < bufferCount; i++ {
		a.buffers = append(a.buffers, newMonotonicBuffer(bufferSize))
	}
	return a
}

// Alloc satisfies the Arena interface.
func (a *monotonicArena) getPOD(size uintptr, align uintptr) unsafe.Pointer {
	// TODO: this degenerates as the buffers all fill up
	for i := 0; i < len(a.buffers); i++ {
		ptr, ok := a.buffers[i].alloc(size, align)
		if ok {
			return ptr
		}
	}
	return nil
}

func (a *monotonicArena) getTyped(ty reflect.Type, n int) unsafe.Pointer {
	if isPOD(ty) {
		return a.getPOD(ty.Size()*uintptr(n), uintptr(ty.Align()))
	}
	// Allocate non-POD data outside the arena
	return reflect.MakeSlice(reflect.SliceOf(ty), n, n).UnsafePointer()
}

// Reset satisfies the Arena interface.
func (a *monotonicArena) Reset() {
	for _, s := range a.buffers {
		s.reset(a.releaseWhenReset)
	}
}
