package wctl

import (
	"github.com/valyala/fastjson"
)

func (c *Client) Connect(address string) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena

	o := (&fastjson.Arena{}).NewObject()
	o.Set("address", arena.NewString(address))

	j := jsonRaw(o.MarshalTo(nil))

	var resp MsgResponse
	if err := c.RequestJSON(RouteConnect, ReqPost, j, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) Disconnect(address string) (*MsgResponse, error) {
	// TODO: proper response struct
	var arena fastjson.Arena

	o := (&fastjson.Arena{}).NewObject()
	o.Set("address", arena.NewString(address))

	j := jsonRaw(o.MarshalTo(nil))

	var resp MsgResponse
	if err := c.RequestJSON(RouteDisconnect, ReqPost, j, &resp); err != nil {
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
	if err := c.RequestJSON(RouteRestart, ReqPost, j, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
