package log

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/valyala/fastjson"
)

func ArenaHex(arena *fastjson.Arena, i interface{}) *fastjson.Value {
	return arenaHex(arena, i, false)
}

func arenaHex(arena *fastjson.Arena, i interface{}, omitempty bool) *fastjson.Value {
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

	if omitempty && len(bytes) == 0 {
		return nil
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

	for i := 0; i < len(keyValPair); i += 2 {
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
func ObjectAny(arena *fastjson.Arena, o *fastjson.Value, key string, val interface{}) error {

	var target *fastjson.Value

	var (
		parts     = strings.Split(key, ",")
		omitempty bool
	)

	// Parse additional arguments
	if len(parts) > 1 {
		switch parts[1] {
		case "omitempty":
			omitempty = true
		}
	}

	switch val := val.(type) {
	case *fastjson.Value:
		if omitempty && val == nil {
			return nil
		}
		target = val

	case []byte, [16]byte, [32]byte, [64]byte:
		var value = arenaHex(arena, val, omitempty)
		if value == nil {
			return nil
		}
		target = value

	case string:
		if omitempty && val == "" {
			return nil
		}
		target = arena.NewString(val)

	case int:
		if omitempty && val == 0 {
			return nil
		}
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

		if omitempty && i64 == 0 {
			return nil
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

		if omitempty && u64 == 0 {
			return nil
		}

		target = arena.NewNumberString(strconv.FormatUint(u64, 10))

	case bool:
		if val {
			target = arena.NewTrue()
		} else {
			if omitempty {
				return nil
			}

			target = arena.NewFalse()
		}

	case float64:
		if omitempty && val == 0 {
			return nil
		}
		target = arena.NewNumberFloat64(val)

	case float32:
		if omitempty && val == 0 {
			return nil
		}
		target = arena.NewNumberFloat64(float64(val))

	default:
		panic("BUG: Unknown dst type when marshaling `" + key + "`")
	}

	o.Set(parts[0], target)
	return nil
}
