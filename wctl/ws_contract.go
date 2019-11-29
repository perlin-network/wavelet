package wctl

import (
	"github.com/valyala/fastjson"
)

func (c *Client) PollContracts() (func(), error) {
	return c.pollWSArray(RouteWSContracts, func(v *fastjson.Value) error {
		// No events as far as I'm aware of
		return nil
	})
}
