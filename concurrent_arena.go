// SPDX-License-Identifier: Apache-2.0

package nuke

import (
	"reflect"
	"sync"
	"unsafe"
)

type concurrentArena struct {
	mtx sync.Mutex
	a   Arena
}

// NewConcurrentArena returns an arena that is safe to be accessed concurrently
// from multiple goroutines.
func NewConcurrentArena(a Arena) Arena {
	if _, isAlready := a.(*concurrentArena); isAlready {
		return a
	}
	return &concurrentArena{a: a}
}

func (a *concurrentArena) getTyped(ty reflect.Type, n int) unsafe.Pointer {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.a.getTyped(ty, n)
}

func (a *concurrentArena) getPOD(size uintptr, align uintptr) unsafe.Pointer {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.a.getPOD(size, align)
}

func (a *concurrentArena) Reset() {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.a.Reset()
}
