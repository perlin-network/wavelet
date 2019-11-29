package log

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

var (
	ErrInvalidHexLength = errors.New("Invalid hex bytes length")
	ErrDoesNotExist     = errors.New("Key does not exist")
)

type ErrUnmarshalFail struct {
	JSONValue  string
	Key        string
	Underlying error
}

func NewErrUnmarshalErr(v *fastjson.Value, keys []string, err error) *ErrUnmarshalFail {
	var key = "."
	if len(keys) > 0 {
		key = strings.Join(keys, ".")
	}

	return &ErrUnmarshalFail{
		JSONValue:  v.String(),
		Key:        key,
		Underlying: err,
	}
}

func NewErrUnmarshal(v *fastjson.Value, key []string) *ErrUnmarshalFail {
	return NewErrUnmarshalErr(v, key, errors.New(""))
}

func NewErrUnmarshalCustom(v *fastjson.Value, key []string, err string) *ErrUnmarshalFail {
	return NewErrUnmarshalErr(v, key, errors.New(err))
}

func (err *ErrUnmarshalFail) Error() string {
	var Error = err.Underlying.Error()
	if Error == "" {
		return "Error unmarshalling key " + err.Key + ": Unknown error"
	}

	return "Error unmarshalling key " + err.Key + ": " + Error
}

func ValueBatch(value *fastjson.Value, keyDstPair ...interface{}) error {
	if (len(keyDstPair) % 2) != 0 {
		return NewErrUnmarshalCustom(value, nil, "keyDstPair is not even")
	}

	for i := 0; i < len(keyDstPair); i += 2 {
		var keys []string

		switch ks := keyDstPair[i].(type) {
		case string:
			keys = append(keys, ks)
		case []string:
			keys = ks
		default:
			return fmt.Errorf("Index %d doesn't have a proper value", i)
		}

		if err := ValueAny(value, keyDstPair[i+1], keys...); err != nil {
			return errors.Wrap(err, "Index "+strconv.Itoa(i)+" failed")
		}
	}

	return nil
}

// ValueAny unmarshals primitives: *string, *int{,8,16,32,64}, *bool,
// *float{,32,64}, *uint{,8,16,32,64} {[]byte, [16,32,64]byte} hex types.
// This function panics on unknown dst types.
func ValueAny(value *fastjson.Value, dst interface{}, key ...string) error {
	val := value.Get(key...)
	if val == nil {
		return NewErrUnmarshalErr(value, key, ErrDoesNotExist)
	}

	// If the value is nil, leave the dst alone.
	if val.Type() == fastjson.TypeNull {
		return nil
	}

	switch dst := dst.(type) {
	case []byte, [16]byte, [32]byte, [64]byte:
		return ValueHex(value, dst, key...)

	case *[]byte, *[16]byte, *[32]byte, *[64]byte:
		b, err := val.StringBytes()
		if err != nil {
			return NewErrUnmarshalErr(val, key, err)
		}

		switch dst := dst.(type) {
		case *[]byte:
			h, err := hex.DecodeString(string(b))
			if err != nil {
				return NewErrUnmarshalErr(val, key, err)
			}

			*dst = h
			return nil

		case *[16]byte:
			return ValueHex(value, *dst, key...)
		case *[32]byte:
			return ValueHex(value, *dst, key...)
		case *[64]byte:
			return ValueHex(value, *dst, key...)
		}

	case *string:
		if val.Type() != fastjson.TypeString {
			return NewErrUnmarshalCustom(val, key, "Not a string")
		}

		b, err := val.StringBytes()
		if err != nil {
			return NewErrUnmarshalErr(val, key, err)
		}

		*dst = string(b)

	case *int, *int8, *int16, *int32, *int64:
		if val.Type() != fastjson.TypeNumber {
			return NewErrUnmarshalCustom(val, key, "Not a number")
		}

		i64, err := val.Int64()
		if err != nil {
			return NewErrUnmarshalErr(val, key, err)
		}

		switch dst := dst.(type) {
		case *int:
			*dst = int(i64)
		case *int8:
			*dst = int8(i64)
		case *int16:
			*dst = int16(i64)
		case *int32:
			*dst = int32(i64)
		case *int64:
			*dst = i64
		}

	case *uint, *uint8, *uint16, *uint32, *uint64:
		if val.Type() != fastjson.TypeNumber {
			return NewErrUnmarshalCustom(val, key, "Not a number")
		}

		u64, err := val.Uint64()
		if err != nil {
			return NewErrUnmarshalErr(val, key, err)
		}

		switch dst := dst.(type) {
		case *uint:
			*dst = uint(u64)
		case *uint8:
			*dst = uint8(u64)
		case *uint16:
			*dst = uint16(u64)
		case *uint32:
			*dst = uint32(u64)
		case *uint64:
			*dst = u64
		}

	case *bool:
		b, err := val.Bool()
		if err != nil {
			return NewErrUnmarshalCustom(val, key, "Not a bool")
		}

		*dst = b

	case *float32, *float64:
		if val.Type() != fastjson.TypeNumber {
			return NewErrUnmarshalCustom(val, key, "Not a number")
		}

		f64, err := val.Float64()
		if err != nil {
			return NewErrUnmarshalErr(val, key, err)
		}

		switch dst := dst.(type) {
		case *float32:
			*dst = float32(f64)
		case *float64:
			*dst = f64
		}

	case func([]byte) error:
		b, err := val.StringBytes()
		if err != nil {
			return NewErrUnmarshalErr(val, key, err)
		}

		dst(b)

	default:
		return NewErrUnmarshalCustom(val, key, "Unknown dst type")
	}

	return nil
}

func ValueString(value *fastjson.Value, key ...string) string {
	return string(value.GetStringBytes(key...))
}

func ValueHex(value *fastjson.Value, dst interface{}, key ...string) error {
	var bytes []byte

	switch dst := dst.(type) {
	case []byte:
		bytes = dst
	case [16]byte:
		bytes = dst[:]
	case [32]byte:
		bytes = dst[:]
	case [64]byte:
		bytes = dst[:]
	default:
		panic("Unknown type")
	}

	val := value.Get(key...)
	if val == nil {
		return NewErrUnmarshalErr(value, key, ErrDoesNotExist)
	}

	b, err := val.StringBytes()
	if err != nil {
		return NewErrUnmarshalErr(val, key, err)
	}

	i, err := hex.Decode(bytes, b)
	if err != nil {
		return NewErrUnmarshalErr(val, key, err)
	}

	if i != len(bytes) {
		return NewErrUnmarshalErr(val, key, ErrInvalidHexLength)
	}

	return nil
}

func ValueUint64(value *fastjson.Value, key ...string) (uint64, error) {
	v := value.Get(key...)
	if v == nil {
		return 0, NewErrUnmarshalErr(value, key, ErrDoesNotExist)
	}

	u, err := v.Uint64()
	if err != nil {
		return 0, NewErrUnmarshalErr(value, key, err)
	}

	return u, nil
}

func ValueBase64(value *fastjson.Value, key ...string) ([]byte, error) {
	v := value.Get(key...)
	if v == nil {
		return nil, NewErrUnmarshalErr(value, key, ErrDoesNotExist)
	}

	b, err := v.StringBytes()
	if err != nil {
		return nil, NewErrUnmarshalErr(v, key, err)
	}

	dec := make([]byte, base64.StdEncoding.DecodedLen(len(b)))

	_, err = base64.StdEncoding.Decode(dec, b)
	return dec, err
}

func ValueTime(value *fastjson.Value, timeFmt string, key ...string) (*time.Time, error) {
	v := value.Get(key...)
	if v == nil {
		return nil, NewErrUnmarshalErr(value, key, ErrDoesNotExist)
	}

	b, err := v.StringBytes()
	if err != nil {
		return nil, NewErrUnmarshalErr(v, key, err)
	}

	t, err := time.Parse(string(b), timeFmt)
	if err != nil {
		return nil, NewErrUnmarshalErr(v, key, err)
	}

	return &t, nil
}
