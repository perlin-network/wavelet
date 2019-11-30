package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"

	"github.com/valyala/fastjson"
)

// PollTransactions calls the callback for each WS event received. On error, the
// callback may be called twice.
func (c *Client) PollTransactions() (func(), error) {
	return c.pollWS(RouteWSTransactions, func(v *fastjson.Value) error {
		var (
			tx = &wavelet.Transaction{}
			ev = log.ValueString(v, "event")
		)

		switch ev {
		case "applied", "rejected":
			// noop
		default:
			return errInvalidEvent(v, ev)
		}

		if err := tx.UnmarshalValue(v); err != nil {
			return err
		}

		switch ev {
		case "applied":
		}

		for v := range c.handlers {
			switch ev {
			case "applied":
				if f, ok := v.(func(*wavelet.TxApplied)); ok {
					f(&wavelet.TxApplied{tx})
				}
			case "rejected":
				if f, ok := v.(func(*wavelet.TxRejected)); ok {
					f(&wavelet.TxRejected{tx})
				}
			}
		}

		return nil
	})
}
