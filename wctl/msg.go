package wctl

import "github.com/valyala/fastjson"

type MsgResponse struct {
	Message string `json:"msg"`
}

func (r *MsgResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	r.Message = string(v.GetStringBytes("msg"))

	return nil
}
