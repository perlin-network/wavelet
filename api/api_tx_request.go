package api

import (
	"encoding/hex"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

type TxRequest struct {
	Sender    string `json:"sender"`
	Nonce     uint64 `json:"nonce"`
	Block     uint64 `json:"block"`
	Tag       byte   `json:"tag"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`

	sender    edwards25519.PublicKey
	payload   []byte
	signature edwards25519.Signature
}

func (s *TxRequest) bind(parser *fastjson.Parser, body []byte) error {
	if err := fastjson.ValidateBytes(body); err != nil {
		return errors.Wrap(err, "invalid json")
	}

	v, err := parser.ParseBytes(body)
	if err != nil {
		return err
	}

	senderVal := v.Get("sender")
	if senderVal == nil {
		return errors.New("missing sender")
	}
	sender, err := senderVal.StringBytes()
	if err != nil {
		return errors.Wrap(err, "invalid sender")
	}

	nonceVal := v.Get("nonce")
	if nonceVal == nil {
		return errors.New("missing nonce")
	}
	nonce, err := nonceVal.Uint64()
	if err != nil {
		return errors.Wrap(err, "invalid nonce")
	}

	blockVal := v.Get("block")
	if blockVal == nil {
		return errors.New("missing block height")
	}
	block, err := blockVal.Uint64()
	if err != nil {
		return errors.Wrap(err, "invalid block height")
	}

	tagVal := v.Get("tag")
	if tagVal == nil {
		return errors.New("missing tag")
	}
	tag, err := tagVal.Uint()
	if err != nil {
		return errors.Wrap(err, "invalid tag")
	}

	payloadVal := v.Get("payload")
	if payloadVal == nil {
		return errors.New("missing payload")
	}
	payload, err := payloadVal.StringBytes()
	if err != nil {
		return errors.Wrap(err, "invalid payload")
	}

	signatureVal := v.Get("signature")
	if signatureVal == nil {
		return errors.New("missing signature")
	}
	signature, err := signatureVal.StringBytes()
	if err != nil {
		return errors.Wrap(err, "invalid signature")
	}

	s.Sender = string(sender)
	s.Nonce = nonce
	s.Block = block
	s.Tag = byte(tag)
	s.Payload = string(payload)
	s.Signature = string(signature)

	senderBuf, err := hex.DecodeString(s.Sender)
	if err != nil {
		return errors.Wrap(err, "sender public key provided is not hex-formatted")
	}

	if len(senderBuf) != wavelet.SizeAccountID {
		return errors.Errorf("sender public key must be size %d", wavelet.SizeAccountID)
	}

	copy(s.sender[:], senderBuf)

	if sys.Tag(s.Tag) > sys.TagBatch {
		return errors.New("unknown transaction tag specified")
	}

	s.payload, err = hex.DecodeString(s.Payload)
	if err != nil {
		return errors.Wrap(err, "payload provided is not hex-formatted")
	}

	signatureBuf, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "signature provided is not hex-formatted")
	}

	if len(signatureBuf) != wavelet.SizeSignature {
		return errors.Errorf("signature must be size %d", wavelet.SizeSignature)
	}

	copy(s.signature[:], signatureBuf)

	return nil
}
