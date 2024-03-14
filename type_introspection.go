package nuke

import (
	"fmt"
	"reflect"
)

// Fast check for whether a type is pointer-free. Assumes that any struct type
// may contain pointers for expediency.
func isPOD(ty reflect.Type) bool {
	switch ty.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8,
		reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	case reflect.Array:
		return isPOD(ty.Elem())
	default:
		return false
	}
}

// Perform full introspection of a type, asserting that it contains no pointers
// anywhere inside it. Any type that passes this assert is completely safe to
// hide from the garbage collector.
func AssertPlainOldData[T any]() {
	problem := assertPODImpl(reflect.TypeFor[T]())
	if problem != "" {
		panic(problem)
	}
}

// Assert that a type is pointerless, returning a non-empty string describing
// the issue if it is does contain pointers.
//
// This function recurses, but does not need to protect itself from infinite
// recursion: the only ways to create a truly recursive type that does not
// contain an infinite number of nested fields (and therefore become invalid)
// must contain pointers.
func assertPODImpl(ty reflect.Type) (problem string) {
	if isPOD(ty) {
		return
	}
	switch ty.Kind() {
	case reflect.Slice, reflect.String, reflect.Interface, reflect.Chan,
		reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer:
		tyVal := reflect.Zero(ty).Interface()
		return fmt.Sprintf("type %T contains pointers", tyVal)
	case reflect.Array:
		problem = assertPODImpl(ty.Elem())
		if problem != "" {
			problem = "array element " + problem
		}
	case reflect.Struct:
		for _, field := range reflect.VisibleFields(ty) {
			problem = assertPODImpl(field.Type)
			if problem != "" {
				tyVal := reflect.Zero(ty).Interface()
				problem = fmt.Sprintf(
					"struct %T field %q: %s",
					tyVal, field.Name, problem)
				return
			}
		}
	default:
	}
	return
}
