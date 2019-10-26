package api

import "github.com/valyala/fastjson"

func valueString(value *fastjson.Value, key ...string) string {
	return string(value.GetStringBytes(key...))
}
