package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

func (c *Client) pollConsensus() (func(), error) {
	return c.pollWS(RouteWSConsensus, func(v *fastjson.Value) error {
		switch ev := log.ValueString(v, "event"); ev {
		case "finalized":
			var cf wavelet.ConsensusFinalized
			if err := cf.UnmarshalValue(v); err != nil {
				return err
			}

			for v := range c.handlers {
				if f, ok := v.(func(*wavelet.ConsensusFinalized)); ok {
					f(&cf)
				}
			}
		default:
			c.error(errInvalidEvent(v, ev))
		}

		return nil
	})
}
