package api

import (
	"encoding/hex"
	"strconv"

	"github.com/valyala/fastjson"
)

// Only supports string, []byte (hex), int, uint64, float64 and bool
func arenaSet(arena *fastjson.Arena, o *fastjson.Value, key string, value interface{}) {
	var v *fastjson.Value

	switch value := value.(type) {
	case string:
		v = arena.NewString(value)
	case []byte:
		v = arenaNewHex(arena, value)
	case int:
		v = arena.NewNumberInt(value)
	case uint64:
		v = arenaNewUint(arena, value)
	case float64:
		v = arena.NewNumberFloat64(value)
	case uint8:
		v = arena.NewNumberInt(int(value))
	case bool:
		if value {
			v = arena.NewTrue()
		} else {
			v = arena.NewFalse()
		}
	case *fastjson.Value:
		v = value
	default:
		panic("Unsupported type")
	}

	o.Set(key, v)
}

func arenaSets(arena *fastjson.Arena, o *fastjson.Value, kvPair ...interface{}) {
	for i := 0; i < len(kvPair); i += 2 {
		arenaSet(arena, o, kvPair[i].(string), kvPair[i+1])
	}
}

func arenaNewUint(arena *fastjson.Arena, u uint64) *fastjson.Value {
	return arena.NewNumberString(strconv.FormatUint(u, 10))
}

func arenaNewHex(arena *fastjson.Arena, h []byte) *fastjson.Value {
	return arena.NewString(hex.EncodeToString(h))
}
