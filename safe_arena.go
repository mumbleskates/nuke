package nuke

import (
	"reflect"
	"unsafe"
)

const (
	ptrSize      = unsafe.Sizeof((*byte)(nil))
	ptrAlignment = unsafe.Alignof((*byte)(nil))
	intSize      = unsafe.Sizeof(int(0))
	intAlignment = unsafe.Alignof(int(0))
)

type safeArena struct {
	// Plain Old Data buffers, with alignments 1, 2, 4, 8, and everything else.
	// The index of the group used comes from alignmentGroupIndex
	podSlabs   safeSlabGroup
	typedSlabs map[reflect.Type]*safeSlabGroup
	// TODO(widders): ownership and growth
}

func newInSafeArena[T any](arena *safeArena) *T {
	var group *safeSlabGroup
	var size, align uintptr
	switch any((*T)(nil)).(type) {
	case *byte, *int8, *bool:
		size, align = 1, 1
		group = &arena.podSlabs
	case *int16, *uint16:
		size, align = 2, 2
		align = 2
		group = &arena.podSlabs
	case *int32, *uint32, *float32:
		size, align = 4, 4
		align = 4
		group = &arena.podSlabs
	case *int64, *uint64, *float64:
		size, align = 8, 8
		group = &arena.podSlabs
	case *int, *uint:
		size, align = intSize, intAlignment
		group = &arena.podSlabs
	case *uintptr:
		size, align = ptrSize, ptrAlignment
		group = &arena.podSlabs
	case *complex64, *complex128:
		tType := reflect.TypeFor[T]()
		size, align = tType.Size(), uintptr(tType.Align())
		group = &arena.podSlabs
	default:
		tType := reflect.TypeFor[T]()
		group = arena.typedSlabs[tType]
		if group == nil {
			group = &safeSlabGroup{}
			arena.typedSlabs[tType] = group
		}
		// Typed slabs don't require extra work to align them, as the slab was
		// allocated as a slice of T. It is therefore already guaranteed to be
		// aligned and will continue to be as it is only used to hold the one
		// type.
		return (*T)(group.getFast(tType.Size()))
	}
	return (*T)(group.get(size, align))
}

// Makes space for any type in the arena, with a user-guarantee that the type
// contains no pointers.
func newPODInSafeArena[T any](arena *safeArena) *T {
	var t T
	size, align := unsafe.Sizeof(t), unsafe.Alignof(t)
	return (*T)(arena.podSlabs.get(size, align))
}

// TODO(widders): slices
// TODO(widders): Sprintf

func (sa *safeArena) reset() {
	sa.podSlabs.reset()
	for _, slab := range sa.typedSlabs {
		slab.reset()
	}
}

type safeSlabGroup struct {
	bufs []safeSlab
	// Index of the first slab that hasn't become full
	firstWithFreeSpace int
}

func (sg *safeSlabGroup) getFast(size uintptr) unsafe.Pointer {
	for i := sg.firstWithFreeSpace; i < len(sg.bufs); i++ {
		// TODO(widders): try getting from each one; if it seems full swap it
		//  back and increment firstWithFreeSpace
	}
	// TODO(widders): make another slab. actually this will happen outside, bc
	//  we don't know how to create the memory for a slab without knowing its
	//  type.
	panic("todo")
}

func (sg *safeSlabGroup) get(size uintptr, alignment uintptr) unsafe.Pointer {
	panic("todo")
}

func (sg *safeSlabGroup) reset() {
	// TODO(widders): deal with high water marks, deleting slabs if they were
	//  not close to being reached this time
	panic("todo")
}

type safeSlab struct {
	buf    unsafe.Pointer
	size   int
	offset uintptr
}

func makeSafeSlab(size int, alignment uintptr) safeSlab {
	// We take ownership of an allocated byte buffer and only keep its head
	// pointer. We could keep the slice itself, but we want to track the offset
	// of the next allocation, not re-slice the front of the slice. If we slice
	// off the front of the buffer, we can't get it offset, so effectively one of
	// the words of the []byte slice would be wasted.
	slab := safeSlab{
		buf:    unsafe.Pointer(&make([]byte, size)[0]),
		size:   size,
		offset: 0,
	}
	return slab
}

func makeTypedSafeSlab[T any](size int) safeSlab {
	// We want to allocate the smallest slice that is at least `size` bytes
	typeSize := int(reflect.TypeFor[T]().Size())
	targetSlots := size / typeSize
	if typeSize*targetSlots < size {
		targetSlots++
	}
	actualSize := typeSize * targetSlots
	return safeSlab{
		// We can still retain ownership of the buffer by punning it as another
		// type, and we can still bump-allocate the precise typed slots of the
		// slice if we only ever put that specific type in it.
		buf:    unsafe.Pointer(&make([]T, targetSlots)[0]),
		size:   actualSize,
		offset: 0,
	}
}

func (s *safeSlab) getFast(size uintptr) unsafe.Pointer {
	if s.offset+size > uintptr(s.size) {
		return nil // not enough space
	}
	newPointer := unsafe.Add(s.buf, s.offset)
	s.offset += size
	return newPointer
}

func (s *safeSlab) get(size uintptr, alignment uintptr) unsafe.Pointer {
	newOffset := s.offset + size
	var realign uintptr = 0
	if alignment != 1 {
		misalignment := (uintptr(s.buf) + newOffset) % alignment
		if misalignment != 0 {
			realign = alignment - misalignment
		}
		newOffset += realign
	}
	if newOffset > uintptr(s.size) {
		return nil // not enough space
	}
	newPointer := unsafe.Add(s.buf, s.offset+realign)
	s.offset = newOffset
	return newPointer
}

func (s *safeSlab) reset() {
	// Zero all the bytes we have handed out from the buffer so far
	b := unsafe.Slice(s.buf, s.offset)
	for i := range b {
		b[i] = 0
	}
	// Reset the offset to the beginning of the buffer again
	s.offset = 0
}

func (s *safeSlab) seemsFull() bool {
	// Slab is more than 3/4 full
	return s.offset > uintptr(s.size*3/4)
}
