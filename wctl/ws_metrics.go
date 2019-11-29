package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/valyala/fastjson"
)

func (c *Client) PollMetrics() (func(), error) {
	return c.pollWS(RouteWSMetrics, func(v *fastjson.Value) error {
		var met wavelet.Metrics
		if err := met.UnmarshalValue(v); err != nil {
			return err
		}

		for v := range c.handlers {
			if f, ok := v.(func(*wavelet.Metrics)); ok {
				f(&met)
			}
		}

		return nil
	})
}
