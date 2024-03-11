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

// An arena type that can retain strong pointers from data that is stored inside
// it.
type safeArena struct {
	// POD (Plain Old Data) values all go into this slab group.
	podSlabs safeSlabGroup
	// Slabs that hold arbitrary types, one slab group for each type. These
	// groups will hold *only* values of that type, and the underlying buffers
	// are created as slices of that type so they will be recognized by the GC.
	typedSlabs map[reflect.Type]*safeSlabGroup
	// Number of slots to create typed slab groups with for each new type added.
	initialTypedSlots int
}

func makeSafeArena() *safeArena {
	return makeSafeArenaWithOptions(4096, 64)
}

func makeSafeArenaWithOptions(initialBytes int, initialTypedSlots int) *safeArena {
	return &safeArena{
		podSlabs: safeSlabGroup{
			bufs: []safeSlab{makeSafeSlab[byte](initialBytes)},
		},
		initialTypedSlots: initialTypedSlots,
	}
}

// Simple layout information for a given type
type layout struct {
	size  uintptr
	align uintptr
	isPOD bool
	ty    reflect.Type
}

func getLayout[T any]() layout {
	var size, align uintptr
	var mayContainPointer bool
	var tType reflect.Type
	switch any((*T)(nil)).(type) {
	case *byte, *int8, *bool:
		size, align = 1, 1
	case *int16, *uint16:
		size, align = 2, 2
	case *int32, *uint32, *float32:
		size, align = 4, 4
	case *int64, *uint64, *float64:
		size, align = 8, 8
	case *int, *uint:
		size, align = intSize, intAlignment
	case *uintptr:
		size, align = ptrSize, ptrAlignment
	case *complex64, *complex128:
		tType = reflect.TypeFor[T]()
		size, align = tType.Size(), uintptr(tType.Align())
	default:
		tType = reflect.TypeFor[T]()
		size, align = tType.Size(), uintptr(tType.Align())
		switch tType.Kind() {
		case reflect.Struct, reflect.Pointer, reflect.String, reflect.Slice,
			reflect.Map, reflect.Interface:
			mayContainPointer = true
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16,
			reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8,
			reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32,
			reflect.Float64, reflect.Complex64, reflect.Complex128:
			mayContainPointer = false
		default:
			mayContainPointer = true
		}
	}
	return layout{
		size:  size,
		align: align,
		isPOD: !mayContainPointer,
		ty:    tType,
	}
}

func getPODLayout[T any]() layout {
	var t T
	return layout{
		size:  unsafe.Sizeof(t),
		align: unsafe.Alignof(t),
		isPOD: true,
	}
}

func newInSafeArena[T any](arena *safeArena, n int) []T {
	var ptr *T
	tLayout := getLayout[T]()
	if tLayout.isPOD {
		ptr = (*T)(arena.podSlabs.newPODSlice(tLayout, n))
	} else {
		group := arena.typedSlabs[tLayout.ty]
		if group == nil {
			newGroup := makeSafeSlabGroup[T](arena.initialTypedSlots)
			arena.typedSlabs[tLayout.ty] = &newGroup
			group = &newGroup
		}
		ptr = (*T)(group.get(tLayout.size, tLayout.align))
		if ptr == nil {
			growSlabGroup[T](group, tLayout.size, n)
			ptr = (*T)(group.getFast(tLayout.size))
			if ptr == nil {
				// This should never happen
				panic("slab allocation failed!")
			}
		}
	}
	return unsafe.Slice(ptr, n)
}

// Makes space for any type in the arena, with a user-declared guarantee that
// the type contains no pointers.
func newPODInSafeArena[T any](arena *safeArena, n int) []T {
	tLayout := getLayout[T]()
	ptr := (*T)(arena.podSlabs.newPODSlice(tLayout, n))
	return unsafe.Slice(ptr, n)
}

// TODO(widders): Sprintf

func (sa *safeArena) reset() {
	sa.podSlabs.reset()
	for _, slab := range sa.typedSlabs {
		slab.reset()
	}
}

type safeSlabGroup struct {
	bufs []safeSlab
	// Total number of allocated bytes in this slab group
	totalBytes int
	// Index of the first slab that hasn't become full
	firstWithFreeSpace int
}

func makeSafeSlabGroup[T any](initialSlots int) safeSlabGroup {
	var t T
	return safeSlabGroup{
		bufs:               []safeSlab{makeSafeSlab[T](initialSlots)},
		totalBytes:         initialSlots * int(unsafe.Sizeof(t)),
		firstWithFreeSpace: 0,
	}
}

func (sg *safeSlabGroup) getFast(size uintptr) unsafe.Pointer {
	for i := sg.firstWithFreeSpace; i < len(sg.bufs); i++ {
		ptr := sg.bufs[i].getFast(size)
		if ptr != nil {
			return ptr
		}
		if sg.bufs[i].seemsFull() {
			// Swap this "seems full now" slab to the front of the range of
			// slabs with free space
			sg.bufs[sg.firstWithFreeSpace], sg.bufs[i] = sg.bufs[i], sg.bufs[sg.firstWithFreeSpace]
			// Don't try to allocate in that slab any more, it seems full
			sg.firstWithFreeSpace++
		}
	}
	return nil // No space found, slab group needs to grow
}

func (sg *safeSlabGroup) get(size uintptr, align uintptr) unsafe.Pointer {
	for i := sg.firstWithFreeSpace; i < len(sg.bufs); i++ {
		ptr := sg.bufs[i].get(size, align)
		if ptr != nil {
			return ptr
		}
		if sg.bufs[i].seemsFull() {
			// Swap this "seems full now" slab to the front of the range of
			// slabs that still have free space
			sg.bufs[sg.firstWithFreeSpace], sg.bufs[i] = sg.bufs[i], sg.bufs[sg.firstWithFreeSpace]
			// Then bump the counter so we don't try to allocate in that slab
			// any more, since it seems full
			sg.firstWithFreeSpace++
		}
	}
	return nil // No space found, slab group needs to grow
}

// Allocates a slice in the slab group with the given layout and returns the
// pointer to its front. This will grow the slab group as necessary.
//
// Only call this method on a POD slab group! It will allocate slabs as []byte,
// and they will only be suitable for POD values.
func (sg *safeSlabGroup) newPODSlice(tLayout layout, n int) unsafe.Pointer {
	ptr := sg.get(tLayout.size, tLayout.align)
	if ptr == nil {
		// When making slots for POD types, we need to make sure there is
		// at least space for n+1 (unaligned) values of T in case the new
		// slab is not sufficiently aligned for T; otherwise we could end up
		// being unable to fit the value in the new slab after correcting
		// alignment.
		growSlabGroup[byte](sg, 1, (n+1)*int(tLayout.size))
		ptr = sg.getFast(tLayout.size)
		if ptr == nil {
			// This should never happen
			panic("slab allocation failed!")
		}
	}
	return ptr
}

func (sg *safeSlabGroup) reset() {
	// TODO(widders): deal with high water marks, deleting slabs if they were
	//  not close to being reached this time
	panic("todo")
}

func growSlabGroup[T any](sg *safeSlabGroup, tSize uintptr, newSlots int) {
	currentTotalSlots := sg.totalBytes / int(tSize)
	if currentTotalSlots > newSlots {
		newSlots = currentTotalSlots
	}
	sg.bufs = append(sg.bufs, makeSafeSlab[T](newSlots))
	sg.totalBytes += newSlots * int(tSize)
}

type safeSlab struct {
	buf    unsafe.Pointer
	size   int
	offset uintptr
}

func makeSafeSlab[T any](slots int) safeSlab {
	var t T
	return safeSlab{
		// We can still retain ownership of the buffer by punning it as another
		// type, and we can still bump-allocate the precise typed slots of the
		// slice if we only ever put that specific type in it.
		buf:    unsafe.Pointer(&make([]T, slots)[0]),
		size:   slots * int(unsafe.Sizeof(t)),
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
