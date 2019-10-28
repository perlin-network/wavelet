package api

import (
	"encoding/base64"

	"github.com/perlin-network/wavelet"
	"github.com/valyala/fastjson"
)

type TxRequest struct {
	Sender    wavelet.AccountID `json:"sender"`
	Nonce     uint64            `json:"nonce"`
	Block     uint64            `json:"block"`
	Tag       uint8             `json:"tag"`
	Payload   []byte            `json:"payload"`
	Signature wavelet.Signature `json:"signature"`
}

var _ JSONObject = (*TxRequest)(nil)

func (s *TxRequest) bind(parser *fastjson.Parser, body []byte) error {
	v, err := parser.ParseBytes(body)
	if err != nil {
		return err
	}

	return s.UnmarshalValue(v)
}

func (s *TxRequest) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	arenaSets(arena, o,
		"sender", s.Sender,
		"nonce", s.Nonce,
		"block", s.Block,
		"tag", s.Tag,
		"payload", base64.StdEncoding.EncodeToString(s.Payload),
		"signature", s.Signature,
	)

	return o.MarshalTo(nil), nil
}

func (s *TxRequest) UnmarshalValue(v *fastjson.Value) error {
	// TODO error handling
	valueHex(v, s.Sender, "sender")
	valueHex(v, s.Signature, "signature")

	s.Nonce = v.GetUint64("nonce")
	s.Block = v.GetUint64("block")
	s.Tag = uint8(v.GetUint("tag"))

	pl, _ := valueBase64(v, "payload")
	s.Payload = pl

	return nil
}
