package api

import (
	"encoding/base64"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
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

var _ log.JSONObject = (*TxRequest)(nil)

func (s *TxRequest) bind(parser *fastjson.Parser, body []byte) error {
	v, err := parser.ParseBytes(body)
	if err != nil {
		return err
	}

	return s.UnmarshalValue(v)
}

func (s *TxRequest) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"sender", s.Sender,
		"nonce", s.Nonce,
		"block", s.Block,
		"tag", s.Tag,
		"payload", base64.StdEncoding.EncodeToString(s.Payload),
		"signature", s.Signature)
}

func (s *TxRequest) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueBatch(v,
		"sender", s.Sender,
		"signature", s.Signature,
		"nonce", &s.Nonce,
		"block", &s.Block,
		"tag", &s.Tag,
		"payload", func(b []byte) error {
			b, err := base64.StdEncoding.DecodeString(string(b))
			if err != nil {
				return err
			}

			s.Payload = b
			return nil
		},
	)
}

// MarshalEvent doesn't return the full contents of Payload, but only its
// length.
func (s *TxRequest) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("sender", s.Sender[:])
	ev.Hex("signature", s.Signature[:])

	ev.Uint64("nonce", s.Nonce)
	ev.Uint64("block", s.Block)
	ev.Uint8("tag", s.Tag)

	ev.Int("payload_len", len(s.Payload))

	ev.Msg("Transaction request")
}