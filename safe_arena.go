package nuke

import (
	"reflect"
	"unsafe"
)

var (
	byteType = reflect.TypeFor[byte]()
)

// TODO(widders): recursive assert check traversing a type with reflect to make
//  sure it is POD for users to use

type Options struct {
	InitialBytes      int
	InitialTypedSlots int
}

func MakeSafeArena() Arena {
	return MakeSafeArenaWithOptions(Options{
		InitialBytes:      4096,
		InitialTypedSlots: 64,
	})
}

func MakeSafeArenaWithOptions(options Options) Arena {
	return &safeArena{
		podSlabs: safeSlabGroup{
			slabs: []safeSlab{makeSafeSlab(byteType, options.InitialBytes)},
		},
		initialTypedSlots: options.InitialTypedSlots,
	}
}

// Gets basic layout information for T. This includes its size and alignment,
// whether it is guaranteed to be a POD type (Plain Old Data with no pointers),
// and will also include the reflected type of T if it may be needed.
func isPOD(ty reflect.Type) bool {
	switch ty.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8,
		reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	default:
		return false
	}
}

// TODO(widders): Sprintf

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

// Reset the arena, erasing all data written to it. This will gradually
// shrink the arena's owned memory if much of its capacity was unused.
func (sa *safeArena) Reset() {
	sa.podSlabs.reset()
	for _, slab := range sa.typedSlabs {
		slab.reset()
	}
}

func (sa *safeArena) getTyped(ty reflect.Type, n int) unsafe.Pointer {
	if isPOD(ty) {
		return sa.podSlabs.getPOD(ty, n)
	} else {
		// When inserting typed data, we have to manage growing the slab groups
		// directly to ensure that the slabs will be allocated with the correct
		// type information.
		group := sa.typedSlabs[ty]
		if group == nil {
			newGroup := makeSafeSlabGroup(ty, sa.initialTypedSlots)
			sa.typedSlabs[ty] = &newGroup
			group = &newGroup
		}
		return group.getTyped(ty, n)
	}
}

func (sa *safeArena) getPOD(ty reflect.Type, n int) unsafe.Pointer {
	return sa.podSlabs.getPOD(ty, n)
}

type safeSlabGroup struct {
	slabs []safeSlab
	// Total number of allocated bytes in this slab group
	totalBytes int
	// Index of the first slab that hasn't become full
	firstWithFreeSpace int
}

func makeSafeSlabGroup(ty reflect.Type, initialSlots int) safeSlabGroup {
	firstSlab := makeSafeSlab(ty, initialSlots)
	return safeSlabGroup{
		slabs:              []safeSlab{firstSlab},
		totalBytes:         firstSlab.size,
		firstWithFreeSpace: 0,
	}
}

// Allocates a slice in the slab group with the given type and returns the
// pointer to its front. This will grow the slab group as necessary.
//
// Only call this method on a typed slab group! It will allocate slabs as []ty,
// and they will only be suitable for that type.
func (sg *safeSlabGroup) getTyped(ty reflect.Type, n int) unsafe.Pointer {
	ptr := sg.getAlwaysAligned(ty.Size() * uintptr(n))
	if ptr == nil {
		sg.grow(ty, n)
		ptr = sg.getAlwaysAligned(ty.Size() * uintptr(n))
		if ptr == nil {
			// This should never happen
			panic("slab allocation failed!")
		}
	}
	return ptr
}

// Allocates a slice in the slab group with the given type and returns the
// pointer to its front. This will grow the slab group as necessary.
//
// Only call this method on a POD slab group! It will allocate slabs as []byte,
// and they will only be suitable for POD values.
func (sg *safeSlabGroup) getPOD(ty reflect.Type, n int) unsafe.Pointer {
	size, align := ty.Size(), ty.Align()
	ptr := sg.getWithAlignment(size, uintptr(align))
	if ptr == nil {
		// When making new slabs for POD types, we need to make sure there is
		// at least space for n+1 (unaligned) values of T in case the new
		// slab is not sufficiently aligned for T; otherwise we could end up
		// being unable to fit the value in the new slab after correcting
		// alignment.
		sg.grow(byteType, (n+1)*int(size))
		ptr = sg.getWithAlignment(size, uintptr(align))
		if ptr == nil {
			// This should never happen
			panic("slab allocation failed!")
		}
	}
	return ptr
}

func (sg *safeSlabGroup) getWithAlignment(size uintptr, align uintptr) unsafe.Pointer {
	for i := sg.firstWithFreeSpace; i < len(sg.slabs); i++ {
		ptr := sg.slabs[i].getWithAlignment(size, align)
		if ptr != nil {
			return ptr
		}
		if sg.slabs[i].seemsFull() {
			// Swap this "seems full now" slab to the front of the range of
			// slabs that still have free space
			sg.slabs[sg.firstWithFreeSpace], sg.slabs[i] = sg.slabs[i], sg.slabs[sg.firstWithFreeSpace]
			// Then bump the counter so we don't try to allocate in that slab
			// any more, since it seems full
			sg.firstWithFreeSpace++
		}
	}
	return nil // No space found, slab group needs to grow
}

// Gets a pointer to a value by a trivial bump. Suitable only for alignment=1
// values and values in slabs that are allocated as []T and only ever contain T.
func (sg *safeSlabGroup) getAlwaysAligned(size uintptr) unsafe.Pointer {
	for i := sg.firstWithFreeSpace; i < len(sg.slabs); i++ {
		ptr := sg.slabs[i].getAlwaysAligned(size)
		if ptr != nil {
			return ptr
		}
		if sg.slabs[i].seemsFull() {
			// Swap this "seems full now" slab to the front of the range of
			// slabs with free space
			sg.slabs[sg.firstWithFreeSpace], sg.slabs[i] = sg.slabs[i], sg.slabs[sg.firstWithFreeSpace]
			// Don't try to allocate in that slab any more, it seems full
			sg.firstWithFreeSpace++
		}
	}
	return nil // No space found, slab group needs to grow
}

func (sg *safeSlabGroup) reset() {
	var highWaterMarkBytes int
	for i := range sg.slabs {
		// Sum up the bytes that were used in the slabs
		highWaterMarkBytes += sg.slabs[i].reset()
	}
	// Gradually shrink the slab group if it has been going mostly unused,
	// removing only the last slab (which is likely to be the largest) but
	// always retaining the initial slab.
	if len(sg.slabs) > 1 && highWaterMarkBytes*4 < sg.totalBytes {
		lastSlab := &sg.slabs[len(sg.slabs)-1]
		// Dereference the last slab and remove it from the group
		sg.totalBytes -= lastSlab.size
		*lastSlab = safeSlab{}
		sg.slabs = sg.slabs[:len(sg.slabs)-1]
	}
}

func (sg *safeSlabGroup) grow(ty reflect.Type, minNewSlots int) {
	currentTotalSlots := sg.totalBytes / int(ty.Size())
	newSlots := minNewSlots
	if currentTotalSlots > minNewSlots {
		// At minimum we double the total capacity of the slab group
		newSlots = currentTotalSlots
	}
	sg.slabs = append(sg.slabs, makeSafeSlab(ty, newSlots))
	sg.totalBytes += newSlots * int(ty.Size())
}

type safeSlab struct {
	buf    unsafe.Pointer
	size   int
	offset uintptr
}

func makeSafeSlab(ty reflect.Type, slots int) safeSlab {
	// We can still retain ownership of the buffer by punning it as another
	// type, and we can still bump-allocate the precise typed slots of the slice
	// if we only ever put that specific type in it; the buffer is therefore
	// owned as an unsafe.Pointer to the first element of the slice, and size is
	// the number of bytes in that slice.
	return safeSlab{
		buf:    reflect.MakeSlice(reflect.SliceOf(ty), slots, slots).UnsafePointer(),
		size:   slots * int(ty.Size()),
		offset: 0,
	}
}

func (s *safeSlab) getAlwaysAligned(size uintptr) unsafe.Pointer {
	if s.offset+size > uintptr(s.size) {
		return nil // not enough space
	}
	newPointer := unsafe.Add(s.buf, s.offset)
	s.offset += size
	return newPointer
}

func (s *safeSlab) getWithAlignment(size uintptr, align uintptr) unsafe.Pointer {
	newOffset := s.offset + size
	var realign uintptr = 0
	if align != 1 {
		misalignment := (uintptr(s.buf) + newOffset) % align
		if misalignment != 0 {
			realign = align - misalignment
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

// Clear and reset the slab, returning how many bytes were used.
func (s *safeSlab) reset() (usedBytes int) {
	// Zero all the bytes we have handed out from the buffer so far
	b := unsafe.Slice(s.buf, s.offset)
	for i := range b {
		b[i] = 0
	}
	usedBytes = int(s.offset)
	// Reset the offset to the beginning of the buffer again
	s.offset = 0
	return
}

func (s *safeSlab) seemsFull() bool {
	// Slab is more than 3/4 full
	return s.offset > uintptr(s.size*3/4)
}
