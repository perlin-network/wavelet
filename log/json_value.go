package log

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/valyala/fastjson"
)

var ErrInvalidHexLength = errors.New("Invalid hex bytes length")

type ErrUnmarshalFail struct {
	JSONValue  string
	Key        string
	Underlying error
}

func NewErrUnmarshalFail(v *fastjson.Value, key string, err error) *ErrUnmarshalFail {
	return &ErrUnmarshalFail{
		JSONValue:  v.String(),
		Key:        key,
		Underlying: err,
	}
}

func (err *ErrUnmarshalFail) Error() string {
	return "Error unmarshalling key " + err.Key + ": " + err.Underlying.Error()
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

	i, err := hex.Decode(bytes, value.GetStringBytes(key...))
	if err != nil {
		return NewErrUnmarshalFail(value, strings.Join(key, "."), err)
	}

	if i != len(bytes) {
		return NewErrUnmarshalFail(value, strings.Join(key, "."),
			ErrInvalidHexLength)
	}

	return nil
}

func ValueUint64(value *fastjson.Value, key ...string) (uint64, error) {
	v := value.Get(key...)
	if v == nil {
		return 0, NewErrUnmarshalFail(value, strings.Join(key, "."),
			errors.New("missing"))
	}

	u, err := v.Uint64()
	if err != nil {
		return 0, NewErrUnmarshalFail(value, strings.Join(key, "."),
			errors.New("invalid uint64"))
	}

	return u, nil
}

func ValueBase64(value *fastjson.Value, key ...string) ([]byte, error) {
	v := value.GetStringBytes(key...)
	b := make([]byte, base64.StdEncoding.DecodedLen(len(v)))
	_, err := base64.StdEncoding.Decode(b, v)
	return b, err
}
