package wavelet

import (
	"crypto/sha1"
	"encoding/binary"
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

func writeUint64(a uint64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, a)

	return bytes
}

func readUint64(a []byte) uint64 {
	return binary.LittleEndian.Uint64(a)
}

func writeBoolean(a bool) []byte {
	if a {
		return []byte{0x1}
	} else {
		return []byte{0x0}
	}
}

func readBoolean(a []byte) bool {
	if len(a) > 0 && a[0] == 0x1 {
		return true
	}

	return false
}

func hash(a string) uint64 {
	sha1Hash := sha1.Sum(writeBytes(a))
	return binary.LittleEndian.Uint64(sha1Hash[0:8])
}

func merge(a ...[]byte) (result []byte) {
	for _, arr := range a {
		result = append(result[:], arr[:]...)
	}

	return
}
