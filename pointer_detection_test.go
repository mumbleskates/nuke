package nuke_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ortuman/nuke"
)

func TestPointerDetection(t *testing.T) {
	nuke.AssertPlainOldData[int]()
	nuke.AssertPlainOldData[[3]bool]()
	nuke.AssertPlainOldData[[3][3]float64]()

	assert.PanicsWithValue(t, `type string contains pointers`, func() {
		nuke.AssertPlainOldData[string]()
	})
	assert.PanicsWithValue(t, `type []bool contains pointers`, func() {
		nuke.AssertPlainOldData[[]bool]()
	})
	assert.PanicsWithValue(t,
		`array element type chan int contains pointers`,
		func() {
			nuke.AssertPlainOldData[[3]chan int]()
		})

	type Foo[T any, U any] struct {
		Public  T
		private U
	}
	nuke.AssertPlainOldData[Foo[int, int]]()
	assert.PanicsWithValue(t,
		`struct nuke_test.Foo[string,int] field "Public": type string contains pointers`,
		func() {
			nuke.AssertPlainOldData[Foo[string, int]]()
		})
	assert.PanicsWithValue(t,
		`struct nuke_test.Foo[int,*int] field "private": type *int contains pointers`,
		func() {
			nuke.AssertPlainOldData[Foo[int, *int]]()
		})
}
