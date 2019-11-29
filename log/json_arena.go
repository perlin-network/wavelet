package log

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/valyala/fastjson"
)

func ArenaHex(arena *fastjson.Arena, i interface{}) *fastjson.Value {
	var bytes []byte

	switch i := i.(type) {
	case []byte:
		bytes = i
	case [16]byte:
		bytes = i[:]
	case [32]byte:
		bytes = i[:]
	case [64]byte:
		bytes = i[:]
	case string:
		bytes = []byte(i)
	default:
		panic("Unknown type")
	}

	return arena.NewString(hex.EncodeToString(bytes))
}

func MarshalObjectBatch(arena *fastjson.Arena, keyValPair ...interface{}) ([]byte, error) {
	o := arena.NewObject()

	if err := ObjectBatch(arena, o, keyValPair...); err != nil {
		return nil, err
	}

	return o.MarshalTo(nil), nil
}

func ObjectBatch(arena *fastjson.Arena, o *fastjson.Value, keyValPair ...interface{}) error {
	if (len(keyValPair) % 2) != 0 {
		panic("keyValPair is not even")
	}

	for i := 0; i < len(keyValPair); i++ {
		key, ok := keyValPair[i].(string)
		if !ok {
			panic(fmt.Sprintf("Key at index %d is not string", i))
		}

		if err := ObjectAny(arena, o, key, keyValPair[i+1]); err != nil {
			return err
		}
	}

	return nil
}

// ObjectAny sets a key-value into the existing object. It does as many types as
// ValueAny.
func ObjectAny(arena *fastjson.Arena, o *fastjson.Value, key string,
	val interface{}) error {

	var target *fastjson.Value

	switch val := val.(type) {
	case *fastjson.Value:
		target = val

	case []byte, [16]byte, [32]byte, [64]byte:
		target = ArenaHex(arena, val)

	case string:
		target = arena.NewString(val)

	case int:
		target = arena.NewNumberInt(val)
	case int8, int16, int32, int64:
		var i64 int64

		switch val := val.(type) {
		case int8:
			i64 = int64(val)
		case int16:
			i64 = int64(val)
		case int32:
			i64 = int64(val)
		case int64:
			i64 = val
		}

		target = arena.NewNumberString(strconv.FormatInt(i64, 10))

	case uint, uint8, uint16, uint32, uint64:
		var u64 uint64

		switch val := val.(type) {
		case uint:
			u64 = uint64(val)
		case uint8:
			u64 = uint64(val)
		case uint16:
			u64 = uint64(val)
		case uint32:
			u64 = uint64(val)
		case uint64:
			u64 = val
		}

		target = arena.NewNumberString(strconv.FormatUint(u64, 10))

	case bool:
		if val {
			target = arena.NewTrue()
		} else {
			target = arena.NewFalse()
		}

	case float64:
		target = arena.NewNumberFloat64(val)
	case float32:
		target = arena.NewNumberFloat64(float64(val))

	default:
		panic("BUG: Unknown dst type")
	}

	o.Set(key, target)
	return nil
}
