package wctl

import (
	"encoding/hex"

	"github.com/valyala/fastjson"
)

type Nonce struct {
	Nonce uint64 `json:"nonce"`
	Block uint64 `json:"block"`
}

func (n *Nonce) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	n.Nonce = v.GetUint64("nonce")
	n.Block = v.GetUint64("block")
	return nil
}

func (c *Client) GetAccountNonce(account [32]byte) (*Nonce, error) {
	path := RouteNonce + "/" + hex.EncodeToString(account[:])

	var res Nonce
	err := c.RequestJSON(path, ReqGet, nil, &res)
	return &res, err
}

func (c *Client) GetSelfNonce() (*Nonce, error) {
	return c.GetAccountNonce(c.PublicKey)
}
