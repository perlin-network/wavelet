package wctl

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

func (c *Client) Connect(address string) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena
	o := (&fastjson.Arena{}).NewObject()
	o.Set("address", arena.NewString(address))

	var resp MsgResponse
	if err := c.RequestJSON(RouteConnect, ReqPost, log.ValueAsJSON(o), &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) Disconnect(address string) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena
	o := (&fastjson.Arena{}).NewObject()
	o.Set("address", arena.NewString(address))

	var resp MsgResponse
	if err := c.RequestJSON(RouteDisconnect, ReqPost, log.ValueAsJSON(o), &resp); err != nil {
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

	var resp MsgResponse
	if err := c.RequestJSON(RouteRestart, ReqPost, log.ValueAsJSON(o), &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
