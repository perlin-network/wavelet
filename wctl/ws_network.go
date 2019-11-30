package wctl

import (
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/log"
	"github.com/valyala/fastjson"
)

func (c *Client) PollNetwork() (func(), error) {
	return c.pollWS(RouteWSNetwork, func(v *fastjson.Value) error {
		var ev log.Loggable

		switch event := log.ValueString(v, "event"); event {
		case "joined":
			ev = &api.NetworkJoined{}
		case "left":
			ev = &api.NetworkLeft{}
		default:
			return errInvalidEvent(v, event)
		}

		if err := ev.UnmarshalValue(v); err != nil {
			return err
		}

		for _, v := range c.handlers {
			if f, ok := v.(func(log.MarshalableEvent)); ok {
				f(ev)
				continue
			}

			switch ev := ev.(type) {
			case *api.NetworkJoined:
				if f, ok := v.(func(*api.NetworkJoined)); ok {
					f(ev)
				}
			case *api.NetworkLeft:
				if f, ok := v.(func(*api.NetworkLeft)); ok {
					f(ev)
				}
			}
		}

		return nil
	})
}
