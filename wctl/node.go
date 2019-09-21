package wctl

import (
	"encoding/hex"

	"github.com/valyala/fastjson"
)

func (c *Client) Connect(address [32]byte) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena
	o := (&fastjson.Arena{}).NewObject()
	o.Set("address", arena.NewString(hex.EncodeToString(address[:])))

	j := jsonRaw(o.MarshalTo(nil))

	var resp MsgResponse
	if err := c.RequestJSON(RouteConnect, ReqGet, j, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) Disconnect(address [32]byte) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena
	o := (&fastjson.Arena{}).NewObject()
	o.Set("address", arena.NewString(hex.EncodeToString(address[:])))

	j := jsonRaw(o.MarshalTo(nil))

	var resp MsgResponse
	if err := c.RequestJSON(RouteDisconnect, ReqGet, j, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) Restart(hard bool) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena
	o := (&fastjson.Arena{}).NewObject()

	if hard {
		o.Set("hard", arena.NewTrue())
	} else {
		o.Set("hard", arena.NewFalse())
	}

	j := jsonRaw(o.MarshalTo(nil))

	var resp MsgResponse
	if err := c.RequestJSON(RouteRestart, ReqGet, j, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
