package log

import (
	"github.com/valyala/fastjson"
)

type DebugEvent struct {
	Message string `json:"message"`
}

var _ UnmarshalableValue = (*DebugEvent)(nil)

func (d *DebugEvent) UnmarshalValue(v *fastjson.Value) error {
	d.Message = string(v.GetStringBytes("message"))
	return nil
}
