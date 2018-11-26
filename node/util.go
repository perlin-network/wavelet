package node

import (
	"reflect"
	"unsafe"
)

// writeBytes converts string to a byte slice without memory allocation.
//
// Note it may break if string and/or slice header will change
// in the future go versions.
func writeBytes(a string) []byte {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&a))
	bh := reflect.SliceHeader{
		Data: sh.Data,
		Len:  sh.Len,
		Cap:  sh.Len,
	}
	return *(*[]byte)(unsafe.Pointer(&bh))
}

func writeString(a []byte) string {
	return *(*string)(unsafe.Pointer(&a))
}