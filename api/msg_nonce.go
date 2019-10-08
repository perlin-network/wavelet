package api

import (
	"strconv"

	"github.com/valyala/fastjson"
)

type nonceResponse struct {
	Nonce uint64 `json:"nonce"`
	Block uint64 `json:"block"`
}

func (s *nonceResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("nonce", arena.NewNumberString(strconv.FormatUint(s.Nonce, 10)))
	o.Set("block", arena.NewNumberString(strconv.FormatUint(s.Block, 10)))
	return o.MarshalTo(nil), nil
}
