package wavelet

import (
	"crypto/sha1"
	"encoding/binary"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/pem-avl"
	"reflect"
	"unsafe"
)

type prefixedStore struct {
	database.Store

	prefix []byte
}

var _ pem_avl.KVStore = (*prefixedStore)(nil)

func newPrefixedStore(store database.Store, prefix []byte) prefixedStore {
	return prefixedStore{Store: store, prefix: prefix}
}

func (s prefixedStore) Get(key []byte) []byte {
	ret, _ := s.Store.Get(merge(s.prefix, key))
	return ret
}

func (s prefixedStore) Has(key []byte) bool {
	ok, err := s.Store.Has(merge(s.prefix, key))

	if err != nil {
		panic(err)
	}

	return ok
}

func (s prefixedStore) Set(key []byte, value []byte) {
	err := s.Store.Put(merge(s.prefix, key), value)

	if err != nil {
		panic(err)
	}
}

func (s prefixedStore) Delete(key []byte) {
	err := s.Store.Delete(merge(s.prefix, key))

	if err != nil {
		panic(err)
	}
}

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
